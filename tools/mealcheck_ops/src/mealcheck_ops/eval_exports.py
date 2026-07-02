"""Compare MealCheck portable eval JSONL exports."""

from __future__ import annotations

import json
from collections import Counter
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable


SCHEMA_VERSION = "0.1"
SUPPORTED_EVAL_TYPES = {"normalization", "checker"}

NORMALIZATION_METRICS = [
    "mismatch_count",
    "expected_source_items",
    "source_items_matched",
    "source_item_preservation_rate",
    "local_model_min_row_match_rate",
    "local_model_mean_row_match_rate",
    "local_model_provider_failures",
    "local_model_decode_failures",
    "local_model_repair_count",
]

CHECKER_METRICS = [
    "mismatch_count",
    "food_items",
    "resolved_items",
    "exact_resolved_items",
    "estimated_items",
    "decomposed_items",
    "unresolved_items",
    "resolved_rate",
]

LIST_FIELDS = {
    "checker": ["unresolved_foods", "unresolved_units"],
    "normalization": ["failure_categories"],
}


class CompareError(ValueError):
    """Raised for incompatible or invalid comparison inputs."""


@dataclass(frozen=True)
class ExportData:
    path: Path
    rows: dict[tuple[str, str, str], dict[str, Any]]
    eval_type: str
    dataset_id: str
    mode: str


def compare_exports(baseline_path: Path, current_path: Path) -> dict[str, Any]:
    baseline = load_export(baseline_path, "baseline")
    current = load_export(current_path, "current")
    validate_compatible(baseline, current)

    baseline_keys = set(baseline.rows)
    current_keys = set(current.rows)
    shared_keys = sorted(baseline_keys & current_keys, key=case_sort_key)
    added_keys = sorted(current_keys - baseline_keys, key=case_sort_key)
    removed_keys = sorted(baseline_keys - current_keys, key=case_sort_key)

    regressions: list[dict[str, Any]] = []
    fixes: list[dict[str, Any]] = []
    still_failing: list[dict[str, Any]] = []
    unchanged_passing = 0
    changed_metric_rows: list[dict[str, Any]] = []

    for key in shared_keys:
        before = baseline.rows[key]
        after = current.rows[key]
        case_compare = compare_case(before, after, baseline.eval_type)
        before_passed = bool(before.get("passed"))
        after_passed = bool(after.get("passed"))
        if before_passed and not after_passed:
            regressions.append(case_compare)
        elif not before_passed and after_passed:
            fixes.append(case_compare)
        elif not before_passed and not after_passed:
            still_failing.append(case_compare)
        else:
            unchanged_passing += 1
        if case_compare["metric_deltas"] or case_compare["list_deltas"]:
            changed_metric_rows.append(case_compare)

    added_cases = [case_snapshot(current.rows[key]) for key in added_keys]
    removed_cases = [case_snapshot(baseline.rows[key]) for key in removed_keys]
    return {
        "schema_version": SCHEMA_VERSION,
        "eval_type": baseline.eval_type,
        "dataset_id": baseline.dataset_id,
        "mode": baseline.mode,
        "baseline_row_count": len(baseline.rows),
        "current_row_count": len(current.rows),
        "shared_case_count": len(shared_keys),
        "added_case_count": len(added_cases),
        "removed_case_count": len(removed_cases),
        "regression_count": len(regressions),
        "fix_count": len(fixes),
        "still_failing_count": len(still_failing),
        "unchanged_passing_count": unchanged_passing,
        "changed_metric_count": len(changed_metric_rows),
        "metric_summary": metric_summary(
            baseline.eval_type,
            [compare_case(baseline.rows[key], current.rows[key], baseline.eval_type) for key in shared_keys],
        ),
        "added_cases": added_cases,
        "removed_cases": removed_cases,
        "regressions": regressions,
        "fixes": fixes,
        "still_failing": still_failing,
        "changed_metric_rows": changed_metric_rows,
    }


def load_export(path: Path, label: str) -> ExportData:
    rows: dict[tuple[str, str, str], dict[str, Any]] = {}
    eval_types: set[str] = set()
    dataset_ids: set[str] = set()
    modes: set[str] = set()
    for line_number, row in read_jsonl(path):
        eval_type = require_string(row, "eval_type", path, line_number)
        dataset_id = require_string(row, "dataset_id", path, line_number)
        case_id = require_string(row, "case_id", path, line_number)
        if eval_type not in SUPPORTED_EVAL_TYPES:
            raise CompareError(f"{label} line {line_number} has unsupported eval_type {eval_type!r}")
        eval_types.add(eval_type)
        dataset_ids.add(dataset_id)
        mode = str(row.get("mode", "") or "")
        if mode:
            modes.add(mode)
        key = (eval_type, dataset_id, case_id)
        if key in rows:
            raise CompareError(f"{label} contains duplicate row key {key}")
        rows[key] = row
    if not rows:
        raise CompareError(f"{label} export {path} contains no rows")
    if len(eval_types) != 1:
        raise CompareError(f"{label} contains multiple eval_type values: {sorted(eval_types)}")
    if len(dataset_ids) != 1:
        raise CompareError(f"{label} contains multiple dataset_id values: {sorted(dataset_ids)}")
    if len(modes) > 1:
        raise CompareError(f"{label} contains multiple mode values: {sorted(modes)}")
    return ExportData(
        path=path,
        rows=rows,
        eval_type=next(iter(eval_types)),
        dataset_id=next(iter(dataset_ids)),
        mode=next(iter(modes)) if modes else "",
    )


def read_jsonl(path: Path) -> Iterable[tuple[int, dict[str, Any]]]:
    try:
        with path.open("r", encoding="utf-8") as handle:
            for line_number, line in enumerate(handle, start=1):
                stripped = line.strip()
                if not stripped or stripped.startswith("#"):
                    continue
                try:
                    row = json.loads(stripped)
                except json.JSONDecodeError as err:
                    raise CompareError(f"{path} line {line_number} is not valid JSON: {err}") from err
                if not isinstance(row, dict):
                    raise CompareError(f"{path} line {line_number} must be a JSON object")
                yield line_number, row
    except FileNotFoundError as err:
        raise CompareError(f"input file not found: {path}") from err


def require_string(row: dict[str, Any], field: str, path: Path, line_number: int) -> str:
    value = row.get(field)
    if not isinstance(value, str) or not value.strip():
        raise CompareError(f"{path} line {line_number} missing string field {field!r}")
    return value


def validate_compatible(baseline: ExportData, current: ExportData) -> None:
    if baseline.eval_type != current.eval_type:
        raise CompareError(
            f"eval_type mismatch: baseline={baseline.eval_type!r} current={current.eval_type!r}"
        )
    if baseline.dataset_id != current.dataset_id:
        raise CompareError(
            f"dataset_id mismatch: baseline={baseline.dataset_id!r} current={current.dataset_id!r}"
        )
    if baseline.mode and current.mode and baseline.mode != current.mode:
        raise CompareError(f"mode mismatch: baseline={baseline.mode!r} current={current.mode!r}")


def compare_case(before: dict[str, Any], after: dict[str, Any], eval_type: str) -> dict[str, Any]:
    metric_deltas: dict[str, dict[str, Any]] = {}
    for metric in metrics_for_eval(eval_type):
        if before.get(metric) != after.get(metric):
            metric_deltas[metric] = {
                "baseline": before.get(metric),
                "current": after.get(metric),
                "delta": numeric_delta(before.get(metric), after.get(metric)),
            }
    list_deltas = list_field_deltas(before, after, eval_type)
    return {
        "case_id": str(before.get("case_id", "")),
        "baseline_passed": bool(before.get("passed")),
        "current_passed": bool(after.get("passed")),
        "baseline_snapshot": case_snapshot(before),
        "current_snapshot": case_snapshot(after),
        "metric_deltas": metric_deltas,
        "list_deltas": list_deltas,
    }


def metrics_for_eval(eval_type: str) -> list[str]:
    if eval_type == "normalization":
        return NORMALIZATION_METRICS
    return CHECKER_METRICS


def numeric_delta(before: Any, after: Any) -> int | float | None:
    if isinstance(before, bool) or isinstance(after, bool):
        return None
    if isinstance(before, (int, float)) and isinstance(after, (int, float)):
        delta = after - before
        if isinstance(before, int) and isinstance(after, int):
            return int(delta)
        return round(float(delta), 6)
    return None


def list_field_deltas(before: dict[str, Any], after: dict[str, Any], eval_type: str) -> dict[str, dict[str, list[str]]]:
    deltas: dict[str, dict[str, list[str]]] = {}
    for field in LIST_FIELDS.get(eval_type, []):
        before_values = normalized_list(before.get(field))
        after_values = normalized_list(after.get(field))
        added = sorted(set(after_values) - set(before_values))
        removed = sorted(set(before_values) - set(after_values))
        if added or removed:
            deltas[field] = {"added": added, "removed": removed}
    return deltas


def normalized_list(value: Any) -> list[str]:
    if value is None:
        return []
    if isinstance(value, list):
        return sorted(str(item) for item in value)
    if isinstance(value, str):
        if not value:
            return []
        return sorted(part for part in value.split("|") if part)
    return [str(value)]


def case_snapshot(row: dict[str, Any]) -> dict[str, Any]:
    fields = [
        "case_id",
        "case_type",
        "category",
        "source_dataset",
        "gate",
        "tags",
        "passed",
        "mismatch_count",
        "failure_categories",
        "decision",
        "expected_decision",
        "source_item_preservation_rate",
        "resolved_rate",
        "unresolved_foods",
        "unresolved_units",
    ]
    return {field: row.get(field) for field in fields if field in row}


def metric_summary(eval_type: str, comparisons: list[dict[str, Any]]) -> dict[str, Any]:
    metric_totals: Counter[str] = Counter()
    changed_counts: Counter[str] = Counter()
    list_adds: dict[str, Counter[str]] = {field: Counter() for field in LIST_FIELDS.get(eval_type, [])}
    list_removes: dict[str, Counter[str]] = {field: Counter() for field in LIST_FIELDS.get(eval_type, [])}
    for comparison in comparisons:
        for metric, delta in comparison["metric_deltas"].items():
            changed_counts[metric] += 1
            value = delta.get("delta")
            if isinstance(value, (int, float)):
                metric_totals[metric] += value
        for field, delta in comparison["list_deltas"].items():
            list_adds[field].update(delta.get("added", []))
            list_removes[field].update(delta.get("removed", []))
    summary: dict[str, Any] = {
        "metric_changed_counts": sorted_count_objects(changed_counts),
        "numeric_delta_totals": sorted_counter_values(metric_totals),
    }
    if eval_type == "checker":
        summary["unresolved_foods_added"] = sorted_count_objects(list_adds["unresolved_foods"])
        summary["unresolved_foods_removed"] = sorted_count_objects(list_removes["unresolved_foods"])
        summary["unresolved_units_added"] = sorted_count_objects(list_adds["unresolved_units"])
        summary["unresolved_units_removed"] = sorted_count_objects(list_removes["unresolved_units"])
    if eval_type == "normalization":
        summary["failure_categories_added"] = sorted_count_objects(list_adds["failure_categories"])
        summary["failure_categories_removed"] = sorted_count_objects(list_removes["failure_categories"])
    return summary


def sorted_count_objects(counter: Counter[str]) -> list[dict[str, Any]]:
    return [{"value": value, "count": count} for value, count in sorted(counter.items(), key=lambda item: (-item[1], item[0]))]


def sorted_counter_values(counter: Counter[str]) -> list[dict[str, Any]]:
    rows = []
    for value, total in sorted(counter.items()):
        if isinstance(total, float):
            total = round(total, 6)
        rows.append({"metric": value, "delta": total})
    return rows


def case_sort_key(key: tuple[str, str, str]) -> str:
    return key[2]


def render_markdown(result: dict[str, Any]) -> str:
    lines = [
        "# Eval Export Compare",
        "",
        f"- eval_type: `{result['eval_type']}`",
        f"- dataset_id: `{result['dataset_id']}`",
    ]
    if result.get("mode"):
        lines.append(f"- mode: `{result['mode']}`")
    lines.extend(
        [
            "",
            "## Summary",
            "",
            "| Metric | Count |",
            "| --- | ---: |",
        ]
    )
    for field in [
        "baseline_row_count",
        "current_row_count",
        "shared_case_count",
        "added_case_count",
        "removed_case_count",
        "regression_count",
        "fix_count",
        "still_failing_count",
        "unchanged_passing_count",
        "changed_metric_count",
    ]:
        lines.append(f"| {field} | {result[field]} |")
    lines.append("")
    lines.extend(markdown_case_section("Regressions", result["regressions"]))
    lines.extend(markdown_case_section("Fixes", result["fixes"]))
    lines.extend(markdown_snapshot_section("Added Cases", result["added_cases"]))
    lines.extend(markdown_snapshot_section("Removed Cases", result["removed_cases"]))
    lines.extend(markdown_case_section("Changed Metrics", result["changed_metric_rows"]))
    lines.extend(markdown_metric_summary(result))
    return "\n".join(lines).rstrip() + "\n"


def markdown_case_section(title: str, rows: list[dict[str, Any]]) -> list[str]:
    lines = [f"## {title}", ""]
    if not rows:
        return lines + ["None.", ""]
    lines.extend(["| Case | Baseline | Current | Changed Fields |", "| --- | --- | --- | --- |"])
    for row in rows:
        changed = sorted(list(row["metric_deltas"].keys()) + list(row["list_deltas"].keys()))
        lines.append(
            f"| `{row['case_id']}` | {bool_word(row['baseline_passed'])} | {bool_word(row['current_passed'])} | {', '.join(changed) or '-'} |"
        )
    return lines + [""]


def markdown_snapshot_section(title: str, rows: list[dict[str, Any]]) -> list[str]:
    lines = [f"## {title}", ""]
    if not rows:
        return lines + ["None.", ""]
    lines.extend(["| Case | Passed | Notes |", "| --- | --- | --- |"])
    for row in rows:
        notes = []
        if "case_type" in row:
            notes.append(str(row["case_type"]))
        if "category" in row:
            notes.append(str(row["category"]))
        lines.append(f"| `{row.get('case_id', '')}` | {bool_word(bool(row.get('passed')))} | {', '.join(notes) or '-'} |")
    return lines + [""]


def markdown_metric_summary(result: dict[str, Any]) -> list[str]:
    summary = result["metric_summary"]
    lines = ["## Metric Summary", ""]
    metric_counts = summary.get("metric_changed_counts", [])
    if metric_counts:
        lines.extend(["| Metric | Changed Cases |", "| --- | ---: |"])
        for row in metric_counts:
            lines.append(f"| {row['value']} | {row['count']} |")
        lines.append("")
    delta_totals = summary.get("numeric_delta_totals", [])
    if delta_totals:
        lines.extend(["| Metric | Total Delta |", "| --- | ---: |"])
        for row in delta_totals:
            lines.append(f"| {row['metric']} | {row['delta']} |")
        lines.append("")
    return lines


def bool_word(value: bool) -> str:
    return "pass" if value else "fail"

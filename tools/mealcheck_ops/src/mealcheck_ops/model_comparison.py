"""Compare MealCheck local-model regimen runs across model candidates."""

from __future__ import annotations

import json
from datetime import UTC, datetime
from pathlib import Path
from typing import Any


SCHEMA_VERSION = "0.1"
REQUIRED_STAGE_ROLES = {"baseline", "candidate"}

QUALITY_METRICS = [
    "min_local_model_row_match_rate",
    "min_local_model_food_accuracy",
    "min_local_model_quantity_accuracy",
    "min_local_model_unit_accuracy",
]

FAILURE_METRICS = [
    "command_failures",
    "repeats_with_mismatches",
    "total_provider_failures",
    "total_decode_failures",
]

TRADEOFF_METRICS = [
    "max_duration_seconds",
    "total_source_repairs",
    "total_repair_cases",
]

RUN_METRICS = [
    "repeats_requested",
    "repeats_completed",
    *FAILURE_METRICS,
    *QUALITY_METRICS,
    *TRADEOFF_METRICS,
]

DEFAULT_INCLUSION_CRITERIA = [
    "GGUF file is already available locally or on the target server; no model download is part of the comparison.",
    "Model runs through the same llama.cpp OpenAI-compatible endpoint used by MealCheck.",
    "Model uses the same MealCheck prompt, compact JSON contract, parser, and deterministic reconciliation path.",
    "Model fits the one-day local-model contract, current source-item cap, and documented timeout envelope.",
    "Primary candidate changes model scale while keeping model family and quantization comparable to production.",
    "Run artifacts record model path or id, SHA when available, llama.cpp build, service settings, quality, failures, repairs, and timing.",
]


class ModelComparisonError(ValueError):
    """Raised for invalid model-comparison matrix or regimen artifacts."""


def compare_model_runs(matrix_path: Path) -> dict[str, Any]:
    matrix = read_json_object(matrix_path)
    base_dir = matrix_path.expanduser().resolve().parent
    runs_config = matrix.get("runs")
    if not isinstance(runs_config, list) or not runs_config:
        raise ModelComparisonError(f"{matrix_path} must contain a non-empty runs array")

    runs = [load_run(config, base_dir, index) for index, config in enumerate(runs_config, start=1)]
    stage_comparisons = compare_stages(runs)
    return {
        "schema_version": SCHEMA_VERSION,
        "generated_at": datetime.now(UTC).isoformat().replace("+00:00", "Z"),
        "comparison_id": string_or_default(matrix.get("comparison_id"), matrix_path.stem),
        "objective": string_or_default(
            matrix.get("objective"),
            "Compare MealCheck local-model quality, failures, repair behavior, and latency.",
        ),
        "matrix_path": str(matrix_path.expanduser().resolve()),
        "inclusion_criteria": string_list(matrix.get("inclusion_criteria")) or DEFAULT_INCLUSION_CRITERIA,
        "runs": runs,
        "stage_comparisons": stage_comparisons,
        "overall_recommendation": overall_recommendation(stage_comparisons),
    }


def load_run(config: Any, base_dir: Path, index: int) -> dict[str, Any]:
    if not isinstance(config, dict):
        raise ModelComparisonError(f"runs[{index}] must be a JSON object")
    stage = required_string(config, "stage", f"runs[{index}]")
    role = required_string(config, "role", f"runs[{index}]")
    if role not in REQUIRED_STAGE_ROLES:
        raise ModelComparisonError(f"runs[{index}].role must be one of {sorted(REQUIRED_STAGE_ROLES)}")
    label = required_string(config, "label", f"runs[{index}]")
    run_dir = resolve_required_dir(required_string(config, "run_dir", f"runs[{index}]"), base_dir)

    summary = read_json_object(run_dir / "summary.json")
    metadata = read_optional_json_object(run_dir / "metadata.json") or {}
    live_rows = read_optional_jsonl(run_dir / "live-summary.jsonl")
    live_result = read_optional_json_object(run_dir / "live-result.json") or {}
    local_model = object_field(metadata, "local_model")
    machine = object_field(metadata, "machine")

    model_path = optional_string(config.get("model_path"))
    model_file_size = optional_number(config.get("model_file_size_bytes"))
    if model_file_size is None and model_path:
        local_model_path = Path(model_path).expanduser()
        if local_model_path.is_file():
            model_file_size = local_model_path.stat().st_size

    return {
        "stage": stage,
        "role": role,
        "label": label,
        "run_dir": str(run_dir),
        "model": {
            "id": optional_string(config.get("model_id"))
            or optional_string(local_model.get("model"))
            or label,
            "path": model_path,
            "sha256": optional_string(config.get("model_sha256"))
            or optional_string(local_model.get("model_sha")),
            "file_size_bytes": model_file_size,
        },
        "service": {
            "base_url": optional_string(local_model.get("base_url")),
            "llama_build": optional_string(config.get("llama_build"))
            or optional_string(local_model.get("llama_build")),
            "max_output_tokens": optional_number(local_model.get("max_output_tokens")),
            "timeout": optional_string(local_model.get("timeout")),
            "curl_max_time_seconds": optional_number(local_model.get("curl_max_time_seconds")),
            "service_notes": optional_string(config.get("service_notes")),
            "resource_notes": optional_string(config.get("resource_notes")),
        },
        "machine": {
            "go_version": machine.get("go_version"),
            "uname": machine.get("uname"),
            "cpu": machine.get("cpu"),
            "memory_bytes": machine.get("memory_bytes"),
        },
        "gate": normalize_gate(summary.get("gate")),
        "metrics": metrics_from_summary(summary),
        "mismatch_case_ids": string_list(summary.get("mismatch_case_ids")),
        "repeat_summaries": compact_repeat_summaries(live_rows),
        "live_result": {
            "dataset_id": optional_string(live_result.get("dataset_id")),
            "mode": optional_string(live_result.get("mode")),
            "total_cases": optional_number(live_result.get("total_cases")),
            "cases_passed": optional_number(live_result.get("cases_passed")),
            "cases_with_mismatches": optional_number(live_result.get("cases_with_mismatches")),
        },
        "artifacts": {
            "metadata": str(run_dir / "metadata.json"),
            "summary": str(run_dir / "summary.json"),
            "live_summary": str(run_dir / "live-summary.jsonl"),
            "live_result": str(run_dir / "live-result.json"),
        },
    }


def compare_stages(runs: list[dict[str, Any]]) -> list[dict[str, Any]]:
    grouped: dict[str, dict[str, dict[str, Any]]] = {}
    for run in runs:
        grouped.setdefault(run["stage"], {})
        role = run["role"]
        if role in grouped[run["stage"]]:
            raise ModelComparisonError(f"stage {run['stage']!r} has duplicate {role!r} run")
        grouped[run["stage"]][role] = run

    comparisons = []
    for stage in sorted(grouped):
        missing = REQUIRED_STAGE_ROLES - set(grouped[stage])
        if missing:
            raise ModelComparisonError(f"stage {stage!r} is missing required run role(s): {sorted(missing)}")
        baseline = grouped[stage]["baseline"]
        candidate = grouped[stage]["candidate"]
        comparisons.append(compare_stage(stage, baseline, candidate))
    return comparisons


def compare_stage(stage: str, baseline: dict[str, Any], candidate: dict[str, Any]) -> dict[str, Any]:
    metric_deltas = {}
    for metric in RUN_METRICS:
        baseline_value = baseline["metrics"].get(metric)
        candidate_value = candidate["metrics"].get(metric)
        delta = numeric_delta(baseline_value, candidate_value)
        metric_deltas[metric] = {
            "baseline": baseline_value,
            "candidate": candidate_value,
            "delta": delta,
        }
        if metric == "max_duration_seconds":
            metric_deltas[metric]["ratio"] = numeric_ratio(candidate_value, baseline_value)

    quality_regressions = [
        metric for metric in QUALITY_METRICS if negative_delta(metric_deltas[metric].get("delta"))
    ]
    quality_improvements = [
        metric for metric in QUALITY_METRICS if positive_delta(metric_deltas[metric].get("delta"))
    ]
    failure_regressions = [
        metric for metric in FAILURE_METRICS if positive_delta(metric_deltas[metric].get("delta"))
    ]
    failure_improvements = [
        metric for metric in FAILURE_METRICS if negative_delta(metric_deltas[metric].get("delta"))
    ]
    repair_delta = metric_deltas["total_source_repairs"].get("delta")
    duration_delta = metric_deltas["max_duration_seconds"].get("delta")
    duration_ratio = metric_deltas["max_duration_seconds"].get("ratio")
    recommendation = stage_recommendation(
        baseline,
        candidate,
        quality_regressions,
        failure_regressions,
        repair_delta,
        duration_delta,
        duration_ratio,
    )
    return {
        "stage": stage,
        "baseline_label": baseline["label"],
        "candidate_label": candidate["label"],
        "baseline_gate_passed": bool(baseline["gate"].get("passed")),
        "candidate_gate_passed": bool(candidate["gate"].get("passed")),
        "candidate_viable": recommendation != "keep_baseline_due_to_candidate_failure",
        "recommendation": recommendation,
        "metric_deltas": metric_deltas,
        "quality_regressions": quality_regressions,
        "quality_improvements": quality_improvements,
        "failure_regressions": failure_regressions,
        "failure_improvements": failure_improvements,
        "tradeoff_notes": tradeoff_notes(
            baseline,
            candidate,
            quality_regressions,
            quality_improvements,
            failure_regressions,
            failure_improvements,
            repair_delta,
            duration_delta,
            duration_ratio,
        ),
    }


def stage_recommendation(
    baseline: dict[str, Any],
    candidate: dict[str, Any],
    quality_regressions: list[str],
    failure_regressions: list[str],
    repair_delta: int | float | None,
    duration_delta: int | float | None,
    duration_ratio: int | float | None,
) -> str:
    baseline_passed = bool(baseline["gate"].get("passed"))
    candidate_passed = bool(candidate["gate"].get("passed"))
    if not candidate_passed or quality_regressions or failure_regressions:
        return "keep_baseline_due_to_candidate_failure"
    if not baseline_passed and candidate_passed:
        return "review_candidate_improvement"
    if negative_delta(repair_delta) and (duration_ratio is None or duration_ratio <= 1.5):
        return "review_candidate_repair_improvement"
    if not negative_delta(repair_delta) and positive_delta(duration_delta):
        return "keep_baseline_quality_tie"
    return "review_tradeoffs"


def tradeoff_notes(
    baseline: dict[str, Any],
    candidate: dict[str, Any],
    quality_regressions: list[str],
    quality_improvements: list[str],
    failure_regressions: list[str],
    failure_improvements: list[str],
    repair_delta: int | float | None,
    duration_delta: int | float | None,
    duration_ratio: int | float | None,
) -> list[str]:
    notes: list[str] = []
    if bool(baseline["gate"].get("passed")) != bool(candidate["gate"].get("passed")):
        notes.append(
            f"gate changed from {bool_word(baseline['gate'].get('passed'))} to {bool_word(candidate['gate'].get('passed'))}"
        )
    if quality_regressions:
        notes.append("candidate regressed quality metrics: " + ", ".join(quality_regressions))
    if quality_improvements:
        notes.append("candidate improved quality metrics: " + ", ".join(quality_improvements))
    if failure_regressions:
        notes.append("candidate added failure counts: " + ", ".join(failure_regressions))
    if failure_improvements:
        notes.append("candidate reduced failure counts: " + ", ".join(failure_improvements))
    if repair_delta is not None:
        if repair_delta < 0:
            notes.append(f"candidate reduced source repairs by {abs_number(repair_delta)}")
        elif repair_delta > 0:
            notes.append(f"candidate added {abs_number(repair_delta)} source repairs")
        else:
            notes.append("source repair count tied")
    if duration_delta is not None:
        if duration_delta > 0:
            detail = f"candidate max duration increased by {format_number(duration_delta)}s"
        elif duration_delta < 0:
            detail = f"candidate max duration decreased by {format_number(abs(duration_delta))}s"
        else:
            detail = "max duration tied"
        if isinstance(duration_ratio, (int, float)):
            detail += f" ({format_number(duration_ratio)}x baseline)"
        notes.append(detail)
    if not notes:
        notes.append("no measured tradeoff changed")
    return notes


def overall_recommendation(stage_comparisons: list[dict[str, Any]]) -> dict[str, Any]:
    if not stage_comparisons:
        return {"status": "insufficient_data", "basis": "no complete stage comparisons"}
    basis = stage_comparisons[-1]
    recommendation = basis["recommendation"]
    if recommendation.startswith("keep_baseline"):
        status = "keep_baseline"
    elif recommendation.startswith("review_candidate"):
        status = "review_candidate"
    else:
        status = "review_tradeoffs"
    return {
        "status": status,
        "basis_stage": basis["stage"],
        "basis_recommendation": recommendation,
        "notes": basis["tradeoff_notes"],
    }


def metrics_from_summary(summary: dict[str, Any]) -> dict[str, Any]:
    return {metric: summary.get(metric) for metric in RUN_METRICS}


def normalize_gate(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        return {
            "passed": bool(value.get("passed")),
            "min_row_match_rate": optional_number(value.get("min_row_match_rate")),
        }
    return {"passed": False, "min_row_match_rate": None}


def compact_repeat_summaries(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    fields = [
        "run_index",
        "exit_code",
        "duration_seconds",
        "cases_with_mismatches",
        "local_model_row_match_rate",
        "local_model_source_repairs",
        "local_model_provider_failures",
        "local_model_decode_failures",
        "mismatch_case_ids",
    ]
    return [{field: row.get(field) for field in fields if field in row} for row in rows]


def read_json_object(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as err:
        raise ModelComparisonError(f"required JSON file not found: {path}") from err
    except json.JSONDecodeError as err:
        raise ModelComparisonError(f"{path} is not valid JSON: {err}") from err
    if not isinstance(value, dict):
        raise ModelComparisonError(f"{path} must contain a JSON object")
    return value


def read_optional_json_object(path: Path) -> dict[str, Any] | None:
    if not path.is_file():
        return None
    return read_json_object(path)


def read_optional_jsonl(path: Path) -> list[dict[str, Any]]:
    if not path.is_file():
        return []
    rows = []
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        stripped = line.strip()
        if not stripped:
            continue
        try:
            value = json.loads(stripped)
        except json.JSONDecodeError as err:
            raise ModelComparisonError(f"{path} line {line_number} is not valid JSON: {err}") from err
        if not isinstance(value, dict):
            raise ModelComparisonError(f"{path} line {line_number} must contain a JSON object")
        rows.append(value)
    return rows


def resolve_required_dir(value: str, base_dir: Path) -> Path:
    path = Path(value).expanduser()
    if not path.is_absolute():
        path = base_dir / path
    resolved = path.resolve()
    if not resolved.is_dir():
        raise ModelComparisonError(f"run_dir is not a directory: {value}")
    return resolved


def required_string(config: dict[str, Any], field: str, context: str) -> str:
    value = optional_string(config.get(field))
    if value is None:
        raise ModelComparisonError(f"{context}.{field} is required")
    return value


def optional_string(value: Any) -> str | None:
    if isinstance(value, str) and value.strip():
        return value.strip()
    return None


def string_or_default(value: Any, default: str) -> str:
    return optional_string(value) or default


def string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [item.strip() for item in value if isinstance(item, str) and item.strip()]


def optional_number(value: Any) -> int | float | None:
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return value
    return None


def object_field(value: dict[str, Any], field: str) -> dict[str, Any]:
    child = value.get(field)
    return child if isinstance(child, dict) else {}


def numeric_delta(before: Any, after: Any) -> int | float | None:
    if isinstance(before, bool) or isinstance(after, bool):
        return None
    if isinstance(before, (int, float)) and isinstance(after, (int, float)):
        delta = after - before
        if isinstance(before, int) and isinstance(after, int):
            return int(delta)
        return round(float(delta), 6)
    return None


def numeric_ratio(numerator: Any, denominator: Any) -> float | None:
    if isinstance(numerator, bool) or isinstance(denominator, bool):
        return None
    if not isinstance(numerator, (int, float)) or not isinstance(denominator, (int, float)):
        return None
    if denominator == 0:
        return None
    return round(float(numerator) / float(denominator), 6)


def negative_delta(value: Any) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool) and value < 0


def positive_delta(value: Any) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool) and value > 0


def abs_number(value: int | float) -> str:
    return format_number(abs(value))


def bool_word(value: Any) -> str:
    return "true" if bool(value) else "false"


def format_number(value: Any) -> str:
    if isinstance(value, bool):
        return str(value).lower()
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        return f"{value:.3f}".rstrip("0").rstrip(".")
    if value is None:
        return ""
    return str(value)


def render_model_comparison_markdown(result: dict[str, Any]) -> str:
    lines = [
        "# Local Model Comparison",
        "",
        f"- comparison_id: `{escape_markdown_cell(str(result['comparison_id']))}`",
        f"- generated_at: `{escape_markdown_cell(str(result['generated_at']))}`",
        f"- recommendation: `{escape_markdown_cell(str(result['overall_recommendation']['status']))}`",
        f"- basis_stage: `{escape_markdown_cell(str(result['overall_recommendation'].get('basis_stage', '')))}`",
        "",
        "## Runs",
        "",
        "| Stage | Role | Label | Gate | Repeats | Min Row | Food | Quantity | Unit | Max Duration | Repairs | Provider Failures | Decode Failures |",
        "| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
    ]
    for run in result["runs"]:
        metrics = run["metrics"]
        lines.append(
            "| {stage} | {role} | `{label}` | {gate} | {repeats} | {row} | {food} | {quantity} | {unit} | {duration} | {repairs} | {provider} | {decode} |".format(
                stage=escape_markdown_cell(str(run["stage"])),
                role=escape_markdown_cell(str(run["role"])),
                label=escape_markdown_cell(str(run["label"])),
                gate=bool_word(run["gate"].get("passed")),
                repeats=format_number(metrics.get("repeats_completed")),
                row=format_number(metrics.get("min_local_model_row_match_rate")),
                food=format_number(metrics.get("min_local_model_food_accuracy")),
                quantity=format_number(metrics.get("min_local_model_quantity_accuracy")),
                unit=format_number(metrics.get("min_local_model_unit_accuracy")),
                duration=format_number(metrics.get("max_duration_seconds")),
                repairs=format_number(metrics.get("total_source_repairs")),
                provider=format_number(metrics.get("total_provider_failures")),
                decode=format_number(metrics.get("total_decode_failures")),
            )
        )

    lines.extend(
        [
            "",
            "## Stage Comparisons",
            "",
            "| Stage | Candidate | Recommendation | Duration Ratio | Repair Delta | Notes |",
            "| --- | --- | --- | ---: | ---: | --- |",
        ]
    )
    for comparison in result["stage_comparisons"]:
        duration = comparison["metric_deltas"]["max_duration_seconds"].get("ratio")
        repairs = comparison["metric_deltas"]["total_source_repairs"].get("delta")
        lines.append(
            "| {stage} | `{candidate}` | `{recommendation}` | {duration} | {repairs} | {notes} |".format(
                stage=escape_markdown_cell(str(comparison["stage"])),
                candidate=escape_markdown_cell(str(comparison["candidate_label"])),
                recommendation=escape_markdown_cell(str(comparison["recommendation"])),
                duration=format_number(duration),
                repairs=format_number(repairs),
                notes=escape_markdown_cell("; ".join(comparison["tradeoff_notes"])),
            )
        )

    lines.extend(["", "## Model Artifacts", "", "| Stage | Role | Model Path | SHA256 | Size | Run Dir |", "| --- | --- | --- | --- | ---: | --- |"])
    for run in result["runs"]:
        model = run["model"]
        lines.append(
            "| {stage} | {role} | `{path}` | `{sha}` | {size} | `{run_dir}` |".format(
                stage=escape_markdown_cell(str(run["stage"])),
                role=escape_markdown_cell(str(run["role"])),
                path=escape_markdown_cell(str(model.get("path") or "")),
                sha=escape_markdown_cell(str(model.get("sha256") or "")),
                size=format_number(model.get("file_size_bytes")),
                run_dir=escape_markdown_cell(str(run["run_dir"])),
            )
        )

    lines.extend(["", "## Inclusion Criteria", ""])
    for criterion in result["inclusion_criteria"]:
        lines.append(f"- {criterion}")
    lines.append("")
    return "\n".join(lines).rstrip() + "\n"


def escape_markdown_cell(value: str) -> str:
    return value.replace("|", "\\|").replace("\n", " ")

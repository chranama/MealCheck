"""Summarize MealCheck run artifacts into an operator review queue."""

from __future__ import annotations

import json
import re
from collections import Counter
from datetime import UTC, datetime
from pathlib import Path
from typing import Any


SCHEMA_VERSION = "0.1"

EVIDENCE_FILES = {
    "manifest.json",
    "decision.json",
    "report.json",
    "normalized-plan.json",
    "review/normalized-plan-review.json",
    "optional/local-model-chunks.json",
    "debug/normalization-failure.json",
}


class ArtifactSummaryError(ValueError):
    """Raised for unreadable or invalid artifact summary inputs."""


def summarize_run_artifacts(artifact_root: Path) -> dict[str, Any]:
    root = artifact_root.expanduser()
    if not root.exists():
        raise ArtifactSummaryError(f"artifact root not found: {artifact_root}")
    resolved_root = root.resolve()
    runs = [summarize_run(run_dir) for run_dir in discover_run_dirs(resolved_root)]

    status_counts = Counter(run["status"] for run in runs)
    decision_counts = Counter(str(run["decision"]) for run in runs if run.get("decision"))
    issue_counts = Counter()
    review_queue: list[dict[str, Any]] = []
    for run in runs:
        for key, value in run["issue_counts"].items():
            issue_counts[key] += value
        review_queue.extend(run["review_queue"])

    review_queue.sort(key=review_queue_sort_key)
    return {
        "schema_version": SCHEMA_VERSION,
        "generated_at": datetime.now(UTC).isoformat().replace("+00:00", "Z"),
        "artifact_root": str(resolved_root),
        "run_count": len(runs),
        "completed_count": status_counts["completed"],
        "failed_count": status_counts["failed"],
        "awaiting_review_count": status_counts["awaiting_review"],
        "unknown_count": status_counts["unknown"],
        "status_counts": dict(sorted(status_counts.items())),
        "decision_counts": dict(sorted(decision_counts.items())),
        "issue_counts": dict(sorted(issue_counts.items())),
        "review_queue": review_queue,
        "runs": runs,
    }


def discover_run_dirs(root: Path) -> list[Path]:
    if root.is_file():
        return [run_dir_for_evidence(root)]
    if is_run_dir(root):
        return [root]

    run_dirs: set[Path] = set()
    for path in root.rglob("*"):
        if not path.is_file():
            continue
        relative = path.relative_to(root).as_posix()
        if evidence_suffix(relative):
            run_dirs.add(run_dir_for_evidence(path))
    return sorted(run_dirs, key=lambda path: path.as_posix())


def is_run_dir(path: Path) -> bool:
    return any((path / evidence).is_file() for evidence in EVIDENCE_FILES)


def evidence_suffix(relative_path: str) -> bool:
    return any(relative_path == evidence or relative_path.endswith("/" + evidence) for evidence in EVIDENCE_FILES)


def run_dir_for_evidence(path: Path) -> Path:
    cleaned = path
    parent_name = cleaned.parent.name
    if parent_name in {"optional", "debug", "review"}:
        return cleaned.parent.parent
    return cleaned.parent


def summarize_run(run_dir: Path) -> dict[str, Any]:
    manifest = read_optional_json(run_dir / "manifest.json")
    decision = read_optional_json(run_dir / "decision.json")
    report = read_optional_json(run_dir / "report.json")
    normalized_plan = read_optional_json(run_dir / "normalized-plan.json")
    review = read_optional_json(run_dir / "review" / "normalized-plan-review.json")
    chunks = read_optional_json(run_dir / "optional" / "local-model-chunks.json")
    failure = read_optional_json(run_dir / "debug" / "normalization-failure.json")

    issue_counts: Counter[str] = Counter()
    review_queue: list[dict[str, Any]] = []
    run_id = run_id_for(run_dir, manifest, decision, report, review, failure, chunks)
    status = run_status(decision, review, failure, chunks)
    decision_value = string_field(decision, "decision") or string_field(report, "decision")
    trust_signals = object_field(review, "trust_signals")
    chunk_summary = summarize_chunks(chunks, failure)
    plan_item_count = normalized_plan_item_count(normalized_plan)
    missing = missing_artifacts(run_dir, manifest)

    for artifact_path in missing:
        issue_counts["missing_artifacts"] += 1
        review_queue.append(
            queue_entry(
                run_id=run_id,
                artifact_dir=run_dir,
                severity="review",
                reason="missing_artifact",
                detail=f"manifest-listed artifact is missing: {artifact_path}",
                artifact_path=artifact_path,
            )
        )

    rows = list_field(review, "rows")
    for row in rows:
        if not isinstance(row, dict) or review_row_resolved(row):
            continue
        issue_counts["unresolved_items"] += 1
        review_queue.append(
            queue_entry(
                run_id=run_id,
                artifact_dir=run_dir,
                severity="review",
                reason="unresolved_item",
                detail=unresolved_detail(row),
                day=int_field(row, "day"),
                meal_code=string_field(row, "meal_code"),
                meal_label=string_field(row, "meal_label"),
                source_item_id=int_field(row, "source_item_id"),
                source_text=string_field(row, "source_text"),
                source_parse_status=string_field(row, "source_parse_status"),
                normalized_food=string_field(row, "normalized_food"),
                quantity=number_field(row, "quantity"),
                unit=string_field(row, "unit"),
                quantity_text=string_field(row, "quantity_text"),
                unresolved_reason=string_field(row, "unresolved_reason"),
                decision=decision_value,
            )
        )

    source_count = int_field(trust_signals, "source_item_count")
    normalized_count = int_field(trust_signals, "normalized_row_count")
    if source_count is not None and normalized_count is not None and source_count != normalized_count:
        issue_counts["source_row_mismatches"] += 1
        review_queue.append(
            queue_entry(
                run_id=run_id,
                artifact_dir=run_dir,
                severity="review",
                reason="source_row_count_mismatch",
                detail=f"{source_count} source items but {normalized_count} normalized rows",
                source_item_count=source_count,
                normalized_row_count=normalized_count,
                decision=decision_value,
            )
        )

    repair_count = first_int(
        int_field(trust_signals, "repair_count"),
        chunk_summary["repair_count"],
    )
    if repair_count > 0:
        issue_counts["repair_count"] += repair_count
        review_queue.append(
            queue_entry(
                run_id=run_id,
                artifact_dir=run_dir,
                severity="review",
                reason="normalization_repairs",
                detail=f"{repair_count} deterministic source-preserving repairs",
                repair_count=repair_count,
                decision=decision_value,
            )
        )

    failed_chunk_count = first_int(
        int_field(trust_signals, "failed_chunk_count"),
        chunk_summary["failed_chunk_count"],
    )
    if failed_chunk_count > 0:
        issue_counts["failed_chunks"] += failed_chunk_count

    if chunk_summary["decode_failure_count"] > 0:
        issue_counts["decode_failures"] += chunk_summary["decode_failure_count"]
    review_queue.extend(chunk_summary["review_queue"](run_id, run_dir, decision_value))

    if failure:
        issue_counts["normalization_failures"] += 1
        review_queue.append(
            queue_entry(
                run_id=run_id,
                artifact_dir=run_dir,
                severity="failure",
                reason="normalization_failure",
                detail=first_string(
                    string_field(failure, "final_error"),
                    string_field(failure, "repair_error"),
                    string_field(failure, "initial_error"),
                    string_field(object_field(failure, "local_model_extraction"), "error"),
                    "normalization failed",
                ),
                failure_stage=string_field(object_field(failure, "local_model_extraction"), "failure_stage"),
                decision=decision_value,
            )
        )

    run_issues = sorted(issue_counts.items())
    return {
        "run_id": run_id,
        "artifact_dir": str(run_dir),
        "status": status,
        "decision": decision_value,
        "case_id": first_string(
            string_field(manifest, "case_id"),
            string_field(decision, "case_id"),
            string_field(report, "case_id"),
        ),
        "manifest_mode": string_field(manifest, "mode"),
        "mealcheck_version": string_field(object_field(manifest, "mealcheck"), "version"),
        "source_item_count": first_int(source_count, int_field(chunks, "source_item_count")),
        "normalized_row_count": first_int(normalized_count, len(rows) if rows else None, plan_item_count),
        "normalized_plan_item_count": plan_item_count,
        "unresolved_item_count": issue_counts["unresolved_items"],
        "repair_count": repair_count,
        "failed_chunk_count": failed_chunk_count,
        "chunk_count": first_int(int_field(chunks, "chunk_count"), chunk_summary["chunk_count"]),
        "decode_failure_count": chunk_summary["decode_failure_count"],
        "missing_artifacts": missing,
        "issue_count": sum(value for _, value in run_issues),
        "issue_counts": dict(run_issues),
        "review_queue": review_queue,
    }


def normalized_plan_item_count(plan: dict[str, Any] | None) -> int:
    count = 0
    for day in list_field(plan, "days"):
        if not isinstance(day, dict):
            continue
        for meal in list_field(day, "meals"):
            if isinstance(meal, dict):
                count += len(list_field(meal, "items"))
    return count


def summarize_chunks(chunks: dict[str, Any] | None, failure: dict[str, Any] | None) -> dict[str, Any]:
    extraction = chunks or object_field(failure, "local_model_extraction") or {}
    chunk_rows = list_field(extraction, "chunks")
    repair_count = 0
    failed_chunk_count = 0
    decode_failure_count = 0
    chunk_issues: list[dict[str, Any]] = []
    for chunk in chunk_rows:
        if not isinstance(chunk, dict):
            continue
        repair_count += int_field(object_field(chunk, "reconciliation"), "repair_count") or 0
        failure_stage = string_field(chunk, "failure_stage")
        error = string_field(chunk, "error")
        if failure_stage or error:
            failed_chunk_count += 1
            reason = "decode_failure" if failure_stage == "decode" else "failed_chunk"
            if reason == "decode_failure":
                decode_failure_count += 1
            chunk_issues.append(
                {
                    "severity": "failure",
                    "reason": reason,
                    "detail": first_string(error, f"chunk failed at {failure_stage}", "chunk failed"),
                    "chunk_index": int_field(chunk, "index"),
                    "day": int_field(chunk, "day"),
                    "meal_code": string_field(chunk, "meal_code"),
                    "meal_label": string_field(chunk, "meal_label"),
                    "failure_stage": failure_stage,
                    "source_item_count": len(list_field(chunk, "source_item_ids")),
                }
            )
    if string_field(extraction, "failure_stage") == "decode" and decode_failure_count == 0:
        decode_failure_count = 1

    def queue_entries(run_id: str, artifact_dir: Path, decision: str | None) -> list[dict[str, Any]]:
        return [
            queue_entry(
                run_id=run_id,
                artifact_dir=artifact_dir,
                decision=decision,
                **issue,
            )
            for issue in chunk_issues
        ]

    return {
        "chunk_count": len(chunk_rows),
        "repair_count": repair_count,
        "failed_chunk_count": failed_chunk_count,
        "decode_failure_count": decode_failure_count,
        "review_queue": queue_entries,
    }


def missing_artifacts(run_dir: Path, manifest: dict[str, Any] | None) -> list[str]:
    if manifest is None:
        return ["manifest.json"]
    missing: list[str] = []
    for artifact_path in string_list_field(manifest, "artifacts"):
        if not (run_dir / artifact_path).is_file():
            missing.append(artifact_path)
    return sorted(missing)


def queue_entry(
    *,
    run_id: str,
    artifact_dir: Path,
    severity: str,
    reason: str,
    detail: str,
    **fields: Any,
) -> dict[str, Any]:
    entry: dict[str, Any] = {
        "run_id": run_id,
        "severity": severity,
        "reason": reason,
        "detail": detail,
        "artifact_dir": str(artifact_dir),
    }
    for key, value in fields.items():
        if value is not None and value != "":
            entry[key] = value
    return entry


def render_artifact_markdown(summary: dict[str, Any]) -> str:
    lines = [
        "# Run Artifact Summary",
        "",
        f"- Artifact root: `{summary['artifact_root']}`",
        f"- Runs: {summary['run_count']}",
        f"- Completed: {summary['completed_count']}",
        f"- Awaiting review: {summary['awaiting_review_count']}",
        f"- Failed: {summary['failed_count']}",
        f"- Review queue items: {len(summary['review_queue'])}",
        "",
    ]
    if summary["decision_counts"]:
        lines.extend(["## Decisions", "", "| Decision | Count |", "|---|---:|"])
        for decision, count in summary["decision_counts"].items():
            lines.append(f"| {escape_markdown_cell(decision)} | {count} |")
        lines.append("")
    if summary["issue_counts"]:
        lines.extend(["## Issues", "", "| Issue | Count |", "|---|---:|"])
        for issue, count in summary["issue_counts"].items():
            lines.append(f"| `{escape_markdown_cell(issue)}` | {count} |")
        lines.append("")

    lines.extend(["## Review Queue", ""])
    if not summary["review_queue"]:
        lines.append("No review queue items.")
    else:
        lines.extend(["| Severity | Reason | Run | Detail |", "|---|---|---|---|"])
        for item in summary["review_queue"]:
            lines.append(
                "| {severity} | `{reason}` | `{run}` | {detail} |".format(
                    severity=escape_markdown_cell(str(item["severity"])),
                    reason=escape_markdown_cell(str(item["reason"])),
                    run=escape_markdown_cell(str(item["run_id"])),
                    detail=escape_markdown_cell(str(item["detail"])),
                )
            )
    lines.append("")
    return "\n".join(lines)


def read_optional_json(path: Path) -> dict[str, Any] | None:
    if not path.is_file():
        return None
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as err:
        raise ArtifactSummaryError(f"{path} is not valid JSON: {err}") from err
    if not isinstance(value, dict):
        raise ArtifactSummaryError(f"{path} must contain a JSON object")
    return value


def run_status(
    decision: dict[str, Any] | None,
    review: dict[str, Any] | None,
    failure: dict[str, Any] | None,
    chunks: dict[str, Any] | None,
) -> str:
    if failure is not None:
        return "failed"
    if decision is not None:
        return "completed"
    if review is not None:
        return "awaiting_review"
    if chunks and (string_field(chunks, "failure_stage") or string_field(chunks, "error")):
        return "failed"
    return "unknown"


def run_id_for(
    run_dir: Path,
    manifest: dict[str, Any] | None,
    decision: dict[str, Any] | None,
    report: dict[str, Any] | None,
    review: dict[str, Any] | None,
    failure: dict[str, Any] | None,
    chunks: dict[str, Any] | None,
) -> str:
    return first_string(
        string_field(review, "run_id"),
        string_field(failure, "run_id"),
        string_field(manifest, "run_id"),
        string_field(manifest, "case_id"),
        string_field(decision, "case_id"),
        string_field(report, "case_id"),
        trim_local_model_prefix(string_field(chunks, "plan_id")),
        run_dir.name,
    )


def review_row_resolved(row: dict[str, Any]) -> bool:
    return bool(row.get("resolved")) and not string_field(row, "unresolved_reason")


def unresolved_detail(row: dict[str, Any]) -> str:
    source = string_field(row, "source_text")
    normalized = string_field(row, "normalized_food")
    reason = string_field(row, "unresolved_reason")
    parts = []
    if source:
        parts.append(source)
    if normalized and normalized != source:
        parts.append(f"normalized as {normalized}")
    if reason:
        parts.append(reason)
    return "; ".join(parts) or "unresolved normalized row"


def review_queue_sort_key(item: dict[str, Any]) -> tuple[int, str, str, int]:
    severity_rank = {"failure": 0, "review": 1, "info": 2}
    return (
        severity_rank.get(str(item.get("severity")), 9),
        str(item.get("run_id", "")),
        str(item.get("reason", "")),
        int(item.get("source_item_id") or item.get("chunk_index") or 0),
    )


def object_field(value: dict[str, Any] | None, field: str) -> dict[str, Any] | None:
    if not isinstance(value, dict):
        return None
    child = value.get(field)
    return child if isinstance(child, dict) else None


def list_field(value: dict[str, Any] | None, field: str) -> list[Any]:
    if not isinstance(value, dict):
        return []
    child = value.get(field)
    return child if isinstance(child, list) else []


def string_list_field(value: dict[str, Any] | None, field: str) -> list[str]:
    return [str(item) for item in list_field(value, field) if isinstance(item, str) and item.strip()]


def string_field(value: dict[str, Any] | None, field: str) -> str | None:
    if not isinstance(value, dict):
        return None
    child = value.get(field)
    if isinstance(child, str) and child.strip():
        return child.strip()
    return None


def int_field(value: dict[str, Any] | None, field: str) -> int | None:
    if not isinstance(value, dict):
        return None
    child = value.get(field)
    if isinstance(child, bool):
        return None
    if isinstance(child, int):
        return child
    return None


def number_field(value: dict[str, Any] | None, field: str) -> int | float | None:
    if not isinstance(value, dict):
        return None
    child = value.get(field)
    if isinstance(child, bool):
        return None
    if isinstance(child, (int, float)):
        return child
    return None


def first_string(*values: str | None) -> str:
    for value in values:
        if value:
            return value
    return ""


def first_int(*values: int | None) -> int:
    for value in values:
        if value is not None:
            return value
    return 0


def trim_local_model_prefix(value: str | None) -> str | None:
    if value is None:
        return None
    return re.sub(r"^local-model-", "", value)


def escape_markdown_cell(value: str) -> str:
    return value.replace("|", "\\|").replace("\n", " ")

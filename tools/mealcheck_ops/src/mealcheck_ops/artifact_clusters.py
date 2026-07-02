"""Cluster MealCheck run-artifact review queues into priority items."""

from __future__ import annotations

import re
from typing import Any


REPAIR_HEAVY_THRESHOLD = 5
MAX_CLUSTER_EXAMPLES = 5

SEVERITY_WEIGHTS = {
    "normalization_failure": 5,
    "failure_stage": 4,
    "unresolved_food": 3,
    "unresolved_unit": 3,
    "repair_heavy_chunk": 2,
    "timing_outlier": 2,
    "source_phrase": 1,
}

SUGGESTED_ACTIONS = {
    "unresolved_food": "review resolver catalog coverage or unresolved recovery copy",
    "unresolved_unit": "review unit normalization, quantity parsing, or unsupported-unit guidance",
    "source_phrase": "review source parsing and local-model normalization examples",
    "failure_stage": "inspect local-model chunk artifacts and decode or provider failure handling",
    "repair_heavy_chunk": "inspect source-measurement repairs and prompt/reconciliation behavior",
    "timing_outlier": "inspect local-model latency, chunk size, and provider availability",
}


def build_clusters(review_queue: list[dict[str, Any]]) -> list[dict[str, Any]]:
    clusters: dict[tuple[str, str], dict[str, Any]] = {}
    for item in review_queue:
        for spec in cluster_specs_for_review_item(item):
            cluster_key = (spec["cluster_type"], spec["key"])
            cluster = clusters.setdefault(
                cluster_key,
                {
                    "cluster_type": spec["cluster_type"],
                    "key": spec["key"],
                    "severity_weight": 0,
                    "issue_count": 0,
                    "_run_ids": set(),
                    "_source_texts": [],
                    "_details": [],
                    "_meal_codes": [],
                    "_unresolved_reasons": [],
                    "_failure_stages": [],
                    "_units": [],
                },
            )
            cluster["severity_weight"] = max(cluster["severity_weight"], spec["severity_weight"])
            cluster["issue_count"] += spec["issue_count"]
            add_set_value(cluster["_run_ids"], string_from_item(item, "run_id"))
            add_example(cluster["_source_texts"], string_from_item(item, "source_text"))
            add_example(cluster["_details"], string_from_item(item, "detail"))
            add_example(
                cluster["_meal_codes"],
                first_string(string_from_item(item, "meal_code"), string_from_item(item, "meal_label")),
            )
            add_example(cluster["_unresolved_reasons"], string_from_item(item, "unresolved_reason"))
            add_example(cluster["_failure_stages"], string_from_item(item, "failure_stage"))
            add_example(
                cluster["_units"],
                first_string(string_from_item(item, "unit"), string_from_item(item, "quantity_text")),
            )

    result = []
    for cluster in clusters.values():
        run_ids = sorted(cluster["_run_ids"])
        issue_count = int(cluster["issue_count"])
        severity_weight = int(cluster["severity_weight"])
        result.append(
            {
                "cluster_type": cluster["cluster_type"],
                "key": cluster["key"],
                "priority_score": len(run_ids) * issue_count * severity_weight,
                "severity_weight": severity_weight,
                "run_count": len(run_ids),
                "issue_count": issue_count,
                "example_run_ids": run_ids[:MAX_CLUSTER_EXAMPLES],
                "example_source_texts": cluster["_source_texts"],
                "example_details": cluster["_details"],
                "meal_codes": sorted(cluster["_meal_codes"]),
                "unresolved_reasons": sorted(cluster["_unresolved_reasons"]),
                "failure_stages": sorted(cluster["_failure_stages"]),
                "units": sorted(cluster["_units"]),
                "suggested_next_action": SUGGESTED_ACTIONS.get(
                    cluster["cluster_type"],
                    "review the queued artifacts",
                ),
            }
        )
    return sorted(result, key=lambda cluster: (cluster["cluster_type"], cluster["key"]))


def cluster_specs_for_review_item(item: dict[str, Any]) -> list[dict[str, Any]]:
    reason = string_from_item(item, "reason")
    specs: list[dict[str, Any]] = []
    if reason in {"unresolved_item", "checker_unresolved_item"}:
        food = first_string(
            string_from_item(item, "normalized_food"),
            string_from_item(item, "food"),
            string_from_item(item, "source_text"),
        )
        if food:
            specs.append(cluster_spec("unresolved_food", food, SEVERITY_WEIGHTS["unresolved_food"]))
        unit = first_string(string_from_item(item, "unit"), string_from_item(item, "quantity_text"))
        if unit:
            specs.append(cluster_spec("unresolved_unit", unit, SEVERITY_WEIGHTS["unresolved_unit"]))
        source_text = string_from_item(item, "source_text")
        if source_text:
            specs.append(cluster_spec("source_phrase", source_text, SEVERITY_WEIGHTS["source_phrase"]))
    if reason in {"decode_failure", "failed_chunk", "normalization_failure"}:
        failure_stage = first_string(string_from_item(item, "failure_stage"), reason)
        weight = (
            SEVERITY_WEIGHTS["normalization_failure"]
            if reason == "normalization_failure"
            else SEVERITY_WEIGHTS["failure_stage"]
        )
        specs.append(
            cluster_spec(
                "failure_stage",
                f"{failure_stage}:{error_family(string_from_item(item, 'detail'))}",
                weight,
            )
        )
    if reason == "repair_heavy_chunk":
        meal = first_string(
            string_from_item(item, "meal_code"),
            string_from_item(item, "meal_label"),
            "unknown_meal",
        )
        repair_count = int_value(item.get("repair_count")) or 1
        specs.append(
            cluster_spec(
                "repair_heavy_chunk",
                f"{normalize_cluster_key(meal)}:{repair_count_bucket(repair_count)}",
                SEVERITY_WEIGHTS["repair_heavy_chunk"],
                issue_count=repair_count,
            )
        )
    if reason == "timing_outlier":
        timing_stage = first_string(string_from_item(item, "timing_stage"), "unknown_stage")
        timing_bucket_value = first_string(
            string_from_item(item, "timing_bucket"),
            timing_bucket(int_value(item.get("timing_ms")) or 0),
        )
        specs.append(
            cluster_spec(
                "timing_outlier",
                f"{normalize_cluster_key(timing_stage)}:{timing_bucket_value}",
                SEVERITY_WEIGHTS["timing_outlier"],
            )
        )
    return specs


def cluster_spec(cluster_type: str, key: str, severity_weight: int, *, issue_count: int = 1) -> dict[str, Any]:
    return {
        "cluster_type": cluster_type,
        "key": normalize_cluster_key(key),
        "severity_weight": severity_weight,
        "issue_count": max(issue_count, 1),
    }


def priority_sort_key(cluster: dict[str, Any]) -> tuple[int, int, int, str, str]:
    return (
        -int(cluster["priority_score"]),
        -int(cluster["run_count"]),
        -int(cluster["issue_count"]),
        str(cluster["cluster_type"]),
        str(cluster["key"]),
    )


def add_example(values: list[str], value: str) -> None:
    if not value or value in values or len(values) >= MAX_CLUSTER_EXAMPLES:
        return
    values.append(value)


def add_set_value(values: set[str], value: str) -> None:
    if value:
        values.add(value)


def string_from_item(item: dict[str, Any], field: str) -> str:
    value = item.get(field)
    if isinstance(value, str) and value.strip():
        return value.strip()
    return ""


def first_string(*values: str | None) -> str:
    for value in values:
        if value:
            return value
    return ""


def int_value(value: Any) -> int | None:
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value
    return None


def normalize_cluster_key(value: str) -> str:
    normalized = re.sub(r"\s+", " ", value.strip().lower())
    normalized = normalized.strip(" .,:;")
    return normalized or "unknown"


def error_family(value: str) -> str:
    lower = normalize_cluster_key(value)
    if "deadline exceeded" in lower or "timed out" in lower or "timeout" in lower:
        return "timeout"
    if "decode" in lower and ("invalid" in lower or "compact" in lower or "json" in lower):
        return "decode_invalid_output"
    if "provider" in lower:
        return "provider_error"
    compact = re.sub(r"[^a-z0-9]+", "_", lower).strip("_")
    return compact[:64] or "unknown_error"


def repair_count_bucket(repair_count: int) -> str:
    if repair_count >= 20:
        return "repairs_20_plus"
    if repair_count >= 10:
        return "repairs_10_to_19"
    if repair_count >= REPAIR_HEAVY_THRESHOLD:
        return f"repairs_{REPAIR_HEAVY_THRESHOLD}_to_9"
    return "repairs_below_threshold"


def timing_bucket(timing_ms: int) -> str:
    if timing_ms >= 120_000:
        return "120s_plus"
    if timing_ms >= 60_000:
        return "60s_to_119s"
    if timing_ms >= 45_000:
        return "45s_to_59s"
    if timing_ms >= 30_000:
        return "30s_to_44s"
    return "below_threshold"

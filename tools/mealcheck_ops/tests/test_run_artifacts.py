from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


PACKAGE_SRC = Path(__file__).resolve().parents[1] / "src"
REPO_ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(PACKAGE_SRC))

from mealcheck_ops.run_artifacts import summarize_run_artifacts  # noqa: E402


class RunArtifactSummaryTest(unittest.TestCase):
    def test_summarizes_completed_review_and_failed_debug_artifacts(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write_completed_review_run(root / "run_completed")
            write_failed_debug_run(root / "run_failed")

            summary = summarize_run_artifacts(root)

            self.assertEqual(summary["run_count"], 2)
            self.assertEqual(summary["completed_count"], 1)
            self.assertEqual(summary["failed_count"], 1)
            self.assertEqual(summary["decision_counts"], {"warn": 1})
            self.assertEqual(summary["issue_counts"]["unresolved_items"], 1)
            self.assertEqual(summary["issue_counts"]["repair_count"], 3)
            self.assertEqual(summary["issue_counts"]["failed_chunks"], 1)
            self.assertEqual(summary["issue_counts"]["decode_failures"], 1)
            self.assertEqual(summary["issue_counts"]["normalization_failures"], 1)
            self.assertEqual(summary["issue_counts"]["source_row_mismatches"], 1)
            self.assertEqual(summary["issue_counts"]["missing_artifacts"], 2)

            reasons = {item["reason"] for item in summary["review_queue"]}
            self.assertEqual(
                reasons,
                {
                    "decode_failure",
                    "missing_artifact",
                    "normalization_failure",
                    "normalization_repairs",
                    "source_row_count_mismatch",
                    "unresolved_item",
                },
            )
            unresolved = next(item for item in summary["review_queue"] if item["reason"] == "unresolved_item")
            self.assertEqual(unresolved["run_id"], "run_completed")
            self.assertEqual(unresolved["source_item_id"], 2)
            self.assertEqual(unresolved["source_text"], "1 cup almond milk")
            self.assertEqual(unresolved["normalized_food"], "almond milk")
            self.assertEqual(unresolved["unresolved_reason"], "food_not_found")

            failed = next(run for run in summary["runs"] if run["run_id"] == "run_failed")
            self.assertEqual(failed["status"], "failed")
            self.assertEqual(failed["decode_failure_count"], 1)

    def test_single_evidence_file_summarizes_owning_run_directory(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            run_dir = Path(tmp) / "run_completed"
            write_completed_review_run(run_dir)

            summary = summarize_run_artifacts(run_dir / "optional" / "local-model-chunks.json")

            self.assertEqual(summary["run_count"], 1)
            self.assertEqual(summary["runs"][0]["run_id"], "run_completed")

    def test_clusters_and_prioritizes_repeated_cross_run_issues(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write_clustered_review_run(
                root / "run_a",
                run_id="run_a",
                source_text="1 cup almond milk",
                normalized_food="almond milk",
                unit="cup",
                repair_count=6,
                provider_request_ms=35_000,
                total_ms=50_000,
            )
            write_clustered_review_run(
                root / "run_b",
                run_id="run_b",
                source_text="1 cup almond milk",
                normalized_food="almond milk",
                unit="cup",
            )
            write_checker_unresolved_run(root / "run_checker")

            summary = summarize_run_artifacts(root)

            clusters = {(cluster["cluster_type"], cluster["key"]): cluster for cluster in summary["clusters"]}
            almond = clusters[("unresolved_food", "almond milk")]
            self.assertEqual(almond["run_count"], 2)
            self.assertEqual(almond["issue_count"], 2)
            self.assertEqual(almond["priority_score"], 12)
            self.assertEqual(almond["example_run_ids"], ["run_a", "run_b"])

            unit = clusters[("unresolved_unit", "cup")]
            self.assertEqual(unit["run_count"], 2)
            self.assertEqual(unit["issue_count"], 2)

            repair = clusters[("repair_heavy_chunk", "b:repairs_5_to_9")]
            self.assertEqual(repair["issue_count"], 6)
            self.assertEqual(repair["priority_score"], 12)

            self.assertIn(("timing_outlier", "provider_request_ms:30s_to_44s"), clusters)
            self.assertIn(("timing_outlier", "total_ms:45s_to_59s"), clusters)
            self.assertIn(("unresolved_food", "seasoning blend"), clusters)
            self.assertIn(("unresolved_unit", "some"), clusters)
            self.assertEqual(summary["priority_queue"][0]["priority_score"], 12)

    def test_cli_writes_json_and_markdown(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write_completed_review_run(root / "run_completed")
            out = root / "summary.json"
            markdown = root / "summary.md"

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "summarize-run-artifacts.py"),
                    "--artifact-root",
                    str(root),
                    "--out",
                    str(out),
                    "--markdown",
                    str(markdown),
                ],
                check=False,
                text=True,
                capture_output=True,
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(json.loads(out.read_text(encoding="utf-8"))["run_count"], 1)
            self.assertIn("# Run Artifact Summary", markdown.read_text(encoding="utf-8"))
            self.assertIn("## Priority Queue", markdown.read_text(encoding="utf-8"))
            self.assertIn("## Clusters", markdown.read_text(encoding="utf-8"))
            self.assertIn("`unresolved_item`", markdown.read_text(encoding="utf-8"))


def write_completed_review_run(run_dir: Path) -> None:
    write_json(
        run_dir / "manifest.json",
        {
            "schema_version": "0.1",
            "case_id": "case-completed",
            "mode": "hosted",
            "mealcheck": {"version": "abc123"},
            "artifacts": [
                "decision.json",
                "report.json",
                "normalized-plan.json",
                "review/normalized-plan-review.json",
                "optional/local-model-chunks.json",
                "missing.json",
            ],
        },
    )
    write_json(run_dir / "decision.json", {"case_id": "case-completed", "decision": "warn"})
    write_json(run_dir / "report.json", {"case_id": "case-completed", "decision": "warn"})
    write_json(run_dir / "normalized-plan.json", {"schema_version": "0.1"})
    write_json(
        run_dir / "review" / "normalized-plan-review.json",
        {
            "schema_version": "0.1",
            "run_id": "run_completed",
            "status": "awaiting_confirmation",
            "trust_signals": {
                "source_item_count": 2,
                "normalized_row_count": 1,
                "unresolved_item_count": 1,
                "repair_count": 3,
                "failed_chunk_count": 0,
            },
            "rows": [
                {
                    "day": 1,
                    "meal_code": "b",
                    "meal_label": "Breakfast",
                    "source_item_id": 1,
                    "source_text": "1 cup oatmeal",
                    "source_parse_status": "parsed",
                    "normalized_food": "cooked oatmeal",
                    "resolved": True,
                    "quantity": 1,
                    "unit": "cup",
                },
                {
                    "day": 1,
                    "meal_code": "b",
                    "meal_label": "Breakfast",
                    "source_item_id": 2,
                    "source_text": "1 cup almond milk",
                    "source_parse_status": "parsed",
                    "normalized_food": "almond milk",
                    "resolved": False,
                    "quantity": 1,
                    "unit": "cup",
                    "unresolved_reason": "food_not_found",
                },
            ],
        },
    )
    write_json(
        run_dir / "optional" / "local-model-chunks.json",
        {
            "schema_version": "0.1",
            "plan_id": "local-model-run_completed",
            "source_item_count": 2,
            "chunk_count": 1,
            "chunks": [
                {
                    "index": 0,
                    "day": 1,
                    "meal_code": "b",
                    "meal_label": "Breakfast",
                    "source_item_ids": [1, 2],
                    "decoded_rows": [{"source_item_id": 1}, {"source_item_id": 2}],
                    "reconciliation": {"repair_count": 3},
                }
            ],
        },
    )


def write_clustered_review_run(
    run_dir: Path,
    *,
    run_id: str,
    source_text: str,
    normalized_food: str,
    unit: str,
    repair_count: int = 0,
    provider_request_ms: int = 0,
    total_ms: int = 0,
) -> None:
    write_json(
        run_dir / "manifest.json",
        {
            "schema_version": "0.1",
            "case_id": run_id,
            "mode": "hosted",
            "mealcheck": {"version": "abc123"},
            "artifacts": [
                "decision.json",
                "report.json",
                "normalized-plan.json",
                "review/normalized-plan-review.json",
                "optional/local-model-chunks.json",
            ],
        },
    )
    write_json(run_dir / "decision.json", {"case_id": run_id, "decision": "warn"})
    write_json(run_dir / "report.json", {"case_id": run_id, "decision": "warn"})
    write_json(run_dir / "normalized-plan.json", {"days": []})
    write_json(
        run_dir / "review" / "normalized-plan-review.json",
        {
            "schema_version": "0.1",
            "run_id": run_id,
            "status": "awaiting_confirmation",
            "trust_signals": {
                "source_item_count": 1,
                "normalized_row_count": 1,
                "unresolved_item_count": 1,
                "repair_count": repair_count,
                "failed_chunk_count": 0,
            },
            "rows": [
                {
                    "day": 1,
                    "meal_code": "b",
                    "meal_label": "Breakfast",
                    "source_item_id": 1,
                    "source_text": source_text,
                    "source_parse_status": "parsed",
                    "normalized_food": normalized_food,
                    "resolved": False,
                    "quantity": 1,
                    "unit": unit,
                    "unresolved_reason": "food_not_found",
                }
            ],
        },
    )
    write_json(
        run_dir / "optional" / "local-model-chunks.json",
        {
            "schema_version": "0.1",
            "plan_id": f"local-model-{run_id}",
            "source_item_count": 1,
            "chunk_count": 1,
            "chunks": [
                {
                    "index": 0,
                    "day": 1,
                    "meal_code": "b",
                    "meal_label": "Breakfast",
                    "source_item_ids": [1],
                    "decoded_rows": [{"source_item_id": 1}],
                    "reconciliation": {"repair_count": repair_count},
                    "stage_timings": {
                        "provider_request_ms": provider_request_ms,
                        "total_ms": total_ms,
                    },
                }
            ],
        },
    )


def write_checker_unresolved_run(run_dir: Path) -> None:
    write_json(
        run_dir / "manifest.json",
        {
            "schema_version": "0.1",
            "case_id": "run_checker",
            "mode": "validate",
            "mealcheck": {"version": "abc123"},
            "artifacts": ["decision.json", "report.json", "normalized-plan.json", "unresolved-foods.json"],
        },
    )
    write_json(run_dir / "decision.json", {"case_id": "run_checker", "decision": "block"})
    write_json(run_dir / "report.json", {"case_id": "run_checker", "decision": "block"})
    write_json(run_dir / "normalized-plan.json", {"days": []})
    write_json(
        run_dir / "unresolved-foods.json",
        [
            {
                "day": 1,
                "meal": "lunch",
                "food": "seasoning blend",
                "quantity_text": "some",
                "unresolved_reason": "vague_quantity",
            }
        ],
    )


def write_failed_debug_run(run_dir: Path) -> None:
    write_json(
        run_dir / "debug" / "normalization-failure.json",
        {
            "schema_version": "0.1",
            "run_id": "run_failed",
            "final_error": "could not decode compact output",
            "local_model_extraction": {
                "schema_version": "0.1",
                "plan_id": "local-model-run_failed",
                "failure_stage": "decode",
                "error": "decode failed",
                "chunks": [
                    {
                        "index": 0,
                        "day": 1,
                        "meal_code": "l",
                        "source_item_ids": [1],
                        "failure_stage": "decode",
                        "error": "invalid token",
                    }
                ],
            },
        },
    )


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, sort_keys=True), encoding="utf-8")

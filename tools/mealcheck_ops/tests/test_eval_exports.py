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

from mealcheck_ops.eval_exports import CompareError, compare_exports  # noqa: E402


class CompareEvalExportsTest(unittest.TestCase):
    def test_cli_reports_regressions_fixes_added_removed_and_metric_delta(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            baseline = root / "baseline.jsonl"
            current = root / "current.jsonl"
            out = root / "compare.json"
            markdown = root / "compare.md"
            write_jsonl(
                baseline,
                [
                    normalization_row("case_a", passed=True),
                    normalization_row("case_b", passed=True),
                    normalization_row("case_c", passed=False, mismatch_count=1, failure_categories=["adapter_item_count_failed"]),
                    normalization_row("case_d", passed=True),
                    normalization_row("case_e", passed=True, matched=2, rate=0.6667),
                ],
            )
            write_jsonl(
                current,
                [
                    normalization_row("case_a", passed=True),
                    normalization_row("case_b", passed=False, mismatch_count=1, failure_categories=["source_inventory_count_failed"]),
                    normalization_row("case_c", passed=True),
                    normalization_row("case_e", passed=True, matched=3, rate=1.0),
                    normalization_row("case_f", passed=True),
                ],
            )

            completed = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / "scripts" / "compare-eval-exports.py"),
                    "--baseline",
                    str(baseline),
                    "--current",
                    str(current),
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

            result = json.loads(out.read_text(encoding="utf-8"))
            self.assertEqual(result["eval_type"], "normalization")
            self.assertEqual(result["dataset_id"], "p0-normalization-v1")
            self.assertEqual(result["regression_count"], 1)
            self.assertEqual(result["fix_count"], 1)
            self.assertEqual(result["added_case_count"], 1)
            self.assertEqual(result["removed_case_count"], 1)
            self.assertEqual(result["changed_metric_count"], 3)
            self.assertEqual(result["regressions"][0]["case_id"], "case_b")
            self.assertEqual(result["fixes"][0]["case_id"], "case_c")
            self.assertEqual(result["added_cases"][0]["case_id"], "case_f")
            self.assertEqual(result["removed_cases"][0]["case_id"], "case_d")
            self.assertIn("# Eval Export Compare", markdown.read_text(encoding="utf-8"))
            self.assertIn("`case_b`", markdown.read_text(encoding="utf-8"))

    def test_checker_compare_summarizes_unresolved_food_and_unit_deltas(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            baseline = root / "baseline.jsonl"
            current = root / "current.jsonl"
            write_jsonl(
                baseline,
                [
                    checker_row(
                        "balanced_common-001",
                        resolved_rate=0.5,
                        resolved_items=1,
                        unresolved_items=1,
                        unresolved_foods=["almond milk"],
                        unresolved_units=["cup"],
                    )
                ],
            )
            write_jsonl(
                current,
                [
                    checker_row(
                        "balanced_common-001",
                        resolved_rate=0.75,
                        resolved_items=3,
                        unresolved_items=1,
                        unresolved_foods=["soy milk"],
                        unresolved_units=["tbsp"],
                    )
                ],
            )

            result = compare_exports(baseline, current)
            self.assertEqual(result["eval_type"], "checker")
            self.assertEqual(result["changed_metric_count"], 1)
            metric_summary = result["metric_summary"]
            self.assertEqual(metric_summary["unresolved_foods_added"], [{"value": "soy milk", "count": 1}])
            self.assertEqual(metric_summary["unresolved_foods_removed"], [{"value": "almond milk", "count": 1}])
            self.assertEqual(metric_summary["unresolved_units_added"], [{"value": "tbsp", "count": 1}])
            self.assertEqual(metric_summary["unresolved_units_removed"], [{"value": "cup", "count": 1}])
            deltas = result["changed_metric_rows"][0]["metric_deltas"]
            self.assertEqual(deltas["resolved_rate"]["delta"], 0.25)
            self.assertEqual(deltas["resolved_items"]["delta"], 2)

    def test_rejects_incompatible_eval_type_or_dataset(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            baseline = root / "baseline.jsonl"
            current = root / "current.jsonl"
            write_jsonl(baseline, [normalization_row("case_a")])
            write_jsonl(current, [checker_row("case_a")])
            with self.assertRaisesRegex(CompareError, "eval_type mismatch"):
                compare_exports(baseline, current)

            write_jsonl(current, [normalization_row("case_a", dataset_id="other-dataset")])
            with self.assertRaisesRegex(CompareError, "dataset_id mismatch"):
                compare_exports(baseline, current)

    def test_rejects_mixed_eval_type_input(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            baseline = root / "baseline.jsonl"
            current = root / "current.jsonl"
            write_jsonl(baseline, [normalization_row("case_a"), checker_row("case_b")])
            write_jsonl(current, [normalization_row("case_a")])
            with self.assertRaisesRegex(CompareError, "multiple eval_type"):
                compare_exports(baseline, current)


def normalization_row(
    case_id: str,
    *,
    dataset_id: str = "p0-normalization-v1",
    passed: bool = True,
    matched: int = 3,
    rate: float = 1.0,
    mismatch_count: int = 0,
    failure_categories: list[str] | None = None,
) -> dict[str, object]:
    return {
        "eval_type": "normalization",
        "dataset_id": dataset_id,
        "mode": "deterministic",
        "case_id": case_id,
        "case_type": "success",
        "gate": "strict",
        "source_dataset": "fixture",
        "tags": ["test"],
        "passed": passed,
        "mismatch_count": mismatch_count,
        "failure_categories": failure_categories or [],
        "expected_source_items": 3,
        "source_items_matched": matched,
        "source_item_preservation_rate": rate,
        "adapter_valid": True,
        "local_model_repeats": 0,
        "local_model_min_row_match_rate": 0,
        "local_model_mean_row_match_rate": 0,
        "local_model_provider_failures": 0,
        "local_model_decode_failures": 0,
        "local_model_repair_count": 0,
    }


def checker_row(
    case_id: str,
    *,
    dataset_id: str = "fndds-grounded-meal-plans-v1",
    passed: bool = True,
    resolved_rate: float = 1.0,
    resolved_items: int = 4,
    unresolved_items: int = 0,
    unresolved_foods: list[str] | None = None,
    unresolved_units: list[str] | None = None,
) -> dict[str, object]:
    return {
        "eval_type": "checker",
        "dataset_id": dataset_id,
        "catalog_id": "catalog-v1",
        "case_id": case_id,
        "category": "test",
        "tags": ["test"],
        "decision": "pass",
        "expected_decision": "pass",
        "passed": passed,
        "mismatch_count": 0,
        "food_items": 4,
        "resolved_items": resolved_items,
        "exact_resolved_items": resolved_items,
        "estimated_items": 0,
        "decomposed_items": 0,
        "unresolved_items": unresolved_items,
        "resolved_rate": resolved_rate,
        "unresolved_foods": unresolved_foods or [],
        "unresolved_units": unresolved_units or [],
    }


def write_jsonl(path: Path, rows: list[dict[str, object]]) -> None:
    path.write_text("".join(json.dumps(row, sort_keys=True) + "\n" for row in rows), encoding="utf-8")

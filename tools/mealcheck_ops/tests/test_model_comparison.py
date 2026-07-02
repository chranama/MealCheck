from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


PACKAGE_SRC = Path(__file__).resolve().parents[1] / "src"
sys.path.insert(0, str(PACKAGE_SRC))

from mealcheck_ops.model_comparison import (  # noqa: E402
    ModelComparisonError,
    compare_model_runs,
    render_model_comparison_markdown,
)


class ModelComparisonTest(unittest.TestCase):
    def test_compares_baseline_and_candidate_regimen_runs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            baseline_model = root / "Qwen3-0.6B-Q4_K_M.gguf"
            candidate_model = root / "Qwen3-1.7B-Q4_K_M.gguf"
            baseline_model.write_bytes(b"baseline")
            candidate_model.write_bytes(b"candidate-model")
            write_regimen_run(root / "baseline", "Qwen3-0.6B-Q4_K_M", duration=120, repairs=4)
            write_regimen_run(root / "candidate", "Qwen3-1.7B-Q4_K_M", duration=180, repairs=0)
            matrix = root / "matrix.json"
            write_json(
                matrix,
                {
                    "comparison_id": "qwen3-size-local",
                    "objective": "Compare production model with larger candidate.",
                    "runs": [
                        {
                            "stage": "local",
                            "role": "baseline",
                            "label": "Qwen3-0.6B-Q4_K_M",
                            "run_dir": "baseline",
                            "model_path": str(baseline_model),
                        },
                        {
                            "stage": "local",
                            "role": "candidate",
                            "label": "Qwen3-1.7B-Q4_K_M",
                            "run_dir": "candidate",
                            "model_path": str(candidate_model),
                            "resource_notes": "local CPU-only test",
                        },
                    ],
                },
            )

            result = compare_model_runs(matrix)

            self.assertEqual(result["comparison_id"], "qwen3-size-local")
            self.assertEqual(result["overall_recommendation"]["status"], "review_candidate")
            comparison = result["stage_comparisons"][0]
            self.assertEqual(comparison["stage"], "local")
            self.assertEqual(comparison["recommendation"], "review_candidate_repair_improvement")
            self.assertEqual(comparison["metric_deltas"]["max_duration_seconds"]["delta"], 60)
            self.assertEqual(comparison["metric_deltas"]["max_duration_seconds"]["ratio"], 1.5)
            self.assertEqual(comparison["metric_deltas"]["total_source_repairs"]["delta"], -4)
            self.assertEqual(result["runs"][0]["model"]["file_size_bytes"], len(b"baseline"))
            self.assertIn("candidate reduced source repairs by 4", comparison["tradeoff_notes"])

            markdown = render_model_comparison_markdown(result)
            self.assertIn("# Local Model Comparison", markdown)
            self.assertIn("`Qwen3-1.7B-Q4_K_M`", markdown)
            self.assertIn("review_candidate_repair_improvement", markdown)

    def test_candidate_failure_keeps_baseline(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write_regimen_run(root / "baseline", "Qwen3-0.6B-Q4_K_M", duration=120, repairs=0)
            write_regimen_run(
                root / "candidate",
                "Qwen3-1.7B-Q4_K_M",
                duration=90,
                repairs=0,
                gate_passed=False,
                decode_failures=1,
                row_match_rate=0.75,
            )
            matrix = root / "matrix.json"
            write_json(
                matrix,
                {
                    "runs": [
                        {
                            "stage": "local",
                            "role": "baseline",
                            "label": "Qwen3-0.6B-Q4_K_M",
                            "run_dir": str(root / "baseline"),
                        },
                        {
                            "stage": "local",
                            "role": "candidate",
                            "label": "Qwen3-1.7B-Q4_K_M",
                            "run_dir": str(root / "candidate"),
                        },
                    ]
                },
            )

            result = compare_model_runs(matrix)

            comparison = result["stage_comparisons"][0]
            self.assertFalse(comparison["candidate_gate_passed"])
            self.assertFalse(comparison["candidate_viable"])
            self.assertEqual(comparison["recommendation"], "keep_baseline_due_to_candidate_failure")
            self.assertEqual(result["overall_recommendation"]["status"], "keep_baseline")

    def test_rejects_stage_without_baseline_and_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write_regimen_run(root / "candidate", "Qwen3-1.7B-Q4_K_M", duration=90, repairs=0)
            matrix = root / "matrix.json"
            write_json(
                matrix,
                {
                    "runs": [
                        {
                            "stage": "local",
                            "role": "candidate",
                            "label": "Qwen3-1.7B-Q4_K_M",
                            "run_dir": "candidate",
                        }
                    ]
                },
            )

            with self.assertRaisesRegex(ModelComparisonError, "missing required run role"):
                compare_model_runs(matrix)

    def test_cli_writes_json_and_markdown(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write_regimen_run(root / "baseline", "Qwen3-0.6B-Q4_K_M", duration=120, repairs=0)
            write_regimen_run(root / "candidate", "Qwen3-1.7B-Q4_K_M", duration=150, repairs=0)
            matrix = root / "matrix.json"
            out = root / "comparison.json"
            markdown = root / "comparison.md"
            write_json(
                matrix,
                {
                    "runs": [
                        {
                            "stage": "local",
                            "role": "baseline",
                            "label": "Qwen3-0.6B-Q4_K_M",
                            "run_dir": "baseline",
                        },
                        {
                            "stage": "local",
                            "role": "candidate",
                            "label": "Qwen3-1.7B-Q4_K_M",
                            "run_dir": "candidate",
                        },
                    ]
                },
            )

            completed = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "mealcheck_ops",
                    "compare-model-runs",
                    "--matrix",
                    str(matrix),
                    "--out",
                    str(out),
                    "--markdown",
                    str(markdown),
                ],
                check=False,
                text=True,
                capture_output=True,
                env={**os.environ, "PYTHONPATH": str(PACKAGE_SRC)},
            )

            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(json.loads(out.read_text(encoding="utf-8"))["stage_comparisons"][0]["stage"], "local")
            self.assertIn("# Local Model Comparison", markdown.read_text(encoding="utf-8"))


def write_regimen_run(
    run_dir: Path,
    label: str,
    *,
    duration: int,
    repairs: int,
    gate_passed: bool = True,
    decode_failures: int = 0,
    provider_failures: int = 0,
    row_match_rate: float = 1.0,
) -> None:
    run_dir.mkdir(parents=True, exist_ok=True)
    write_json(
        run_dir / "metadata.json",
        {
            "schema_version": "0.1",
            "regimen": "p0-live-local-model",
            "local_model": {
                "base_url": "http://127.0.0.1:11435/v1",
                "model": f"{label}.gguf",
                "model_sha": f"sha-{label}",
                "llama_build": "llama-test-build",
                "max_output_tokens": 1536,
                "timeout": "240s",
                "curl_max_time_seconds": 20,
            },
            "machine": {
                "go_version": "go version go1.24 test",
                "uname": "Darwin test",
                "cpu": "test cpu",
                "memory_bytes": "17179869184",
            },
        },
    )
    summary = {
        "schema_version": "0.1",
        "regimen": "p0-live-local-model",
        "repeats_requested": 3,
        "repeats_completed": 3,
        "command_failures": 0 if gate_passed else 1,
        "repeats_with_mismatches": 0 if gate_passed else 1,
        "min_local_model_row_match_rate": row_match_rate,
        "min_local_model_food_accuracy": row_match_rate,
        "min_local_model_quantity_accuracy": row_match_rate,
        "min_local_model_unit_accuracy": row_match_rate,
        "max_duration_seconds": duration,
        "total_source_repairs": repairs,
        "total_repair_cases": 1 if repairs else 0,
        "total_provider_failures": provider_failures,
        "total_decode_failures": decode_failures,
        "mismatch_case_ids": [] if gate_passed else ["case_failed"],
        "gate": {"min_row_match_rate": 1, "passed": gate_passed},
    }
    write_json(run_dir / "summary.json", summary)
    write_json(
        run_dir / "live-result.json",
        {
            "dataset_id": "p0-normalization-v1",
            "mode": "local-llama",
            "total_cases": 14,
            "cases_passed": 14 if gate_passed else 13,
            "cases_with_mismatches": summary["repeats_with_mismatches"],
        },
    )
    (run_dir / "live-summary.jsonl").write_text(
        json.dumps(
            {
                "run_index": 1,
                "exit_code": 0 if gate_passed else 1,
                "duration_seconds": duration,
                "cases_with_mismatches": summary["repeats_with_mismatches"],
                "local_model_row_match_rate": row_match_rate,
                "local_model_source_repairs": repairs,
                "local_model_provider_failures": provider_failures,
                "local_model_decode_failures": decode_failures,
                "mismatch_case_ids": summary["mismatch_case_ids"],
            }
        )
        + "\n",
        encoding="utf-8",
    )


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")

# Local Model Comparison

- comparison_id: `qwen3-0.6b-vs-qwen3-1.7b-local-20260702`
- generated_at: `2026-07-02T21:30:39.233027Z`
- recommendation: `keep_baseline`
- basis_stage: `local`

## Runs

| Stage | Role | Label | Gate | Repeats | Min Row | Food | Quantity | Unit | Max Duration | Repairs | Provider Failures | Decode Failures |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| local | baseline | `Qwen3-0.6B-Q4_K_M` | true | 3 | 1 | 1 | 1 | 1 | 45 | 81 | 0 | 0 |
| local | candidate | `Qwen3-1.7B-Q4_K_M` | false | 3 | 0.859 | 0.859 | 0.859 | 0.859 | 99 | 128 | 0 | 1 |

## Stage Comparisons

| Stage | Candidate | Recommendation | Duration Ratio | Repair Delta | Notes |
| --- | --- | --- | ---: | ---: | --- |
| local | `Qwen3-1.7B-Q4_K_M` | `keep_baseline_due_to_candidate_failure` | 2.2 | 47 | gate changed from true to false; candidate regressed quality metrics: min_local_model_row_match_rate, min_local_model_food_accuracy, min_local_model_quantity_accuracy, min_local_model_unit_accuracy; candidate added failure counts: repeats_with_mismatches, total_decode_failures; candidate added 47 source repairs; candidate max duration increased by 54s (2.2x baseline) |

## Model Artifacts

| Stage | Role | Model Path | SHA256 | Size | Run Dir |
| --- | --- | --- | --- | ---: | --- |
| local | baseline | `/Users/chranama/infra/models/gguf/Qwen3-0.6B-Q4_K_M.gguf` | `18ea1f301079bba6391ab6d455c0c8565fd5a3214075eb2cd9daf351dedc719b` | 484220160 | `/Users/chranama/career/MealCheck/.mealcheck-local-model/model-comparison/20260702-local-0.6b` |
| local | candidate | `/Users/chranama/infra/models/gguf/Qwen3-1.7B-Q4_K_M.gguf` | `2cf0badd555da6ffb89ff4105dd9abfd8f331808ee271d01071eefdc4a45b8c3` | 1282439424 | `/Users/chranama/career/MealCheck/.mealcheck-local-model/model-comparison/20260702-local-1.7b` |

## Inclusion Criteria

- GGUF file is already available locally or on the target server; no model download is part of the comparison.
- Model runs through the same llama.cpp OpenAI-compatible endpoint used by MealCheck.
- Model uses the same MealCheck prompt, compact JSON contract, parser, and deterministic reconciliation path.
- Model fits the one-day local-model contract, current source-item cap, and documented timeout envelope.
- Primary candidate changes model scale while keeping model family and quantization comparable to production.
- Run artifacts record model path or id, SHA when available, llama.cpp build, service settings, quality, failures, repairs, and timing.

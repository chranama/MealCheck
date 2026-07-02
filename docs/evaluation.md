# MealCheck Evaluation

MealCheck evaluates two distinct tasks. Keeping them separate matters because a
failure in the first task prevents the second task from running, and a failure
in the second task does not necessarily mean the model normalized the user's
input incorrectly.

Task 1 is P0 meal-plan normalization. It asks whether an in-bound pasted meal
plan can be turned into canonical MealCheck structure:

```text
natural meal-plan text
  -> deterministic meal chunks with numbered source items
  -> per-meal compact row JSON
  -> canonical MealCheck plan JSON
  -> deterministic checker load
```

P0 measures source item preservation, day and meal assignment, quantity/unit
parsing, schema validity, and whether unsupported or vague inputs fail at the
right stage with useful guidance. P0 does not judge nutrient totals, final
report decision, or resolver catalog coverage except as downstream smoke
checks.

Task 2 is P1 food and unit resolution. It asks whether a canonical MealCheck
plan can resolve ordinary foods and measured portions to nutrient evidence
without guessing. P1 measures local catalog coverage, FNDDS fallback coverage,
unresolved reason quality, expected-outcome mismatches, and the conservative
boundary between exact, estimated, decomposed, and unresolved foods.

## P0 Meal-Plan Normalization

P0 is the highest-priority user-facing evaluation because normalization is the
gateway to every hosted live report. Recent live failures have come from hidden
day or meal-count assumptions, unsupported-unit handling, natural phrasing, and
local-model representation mismatch. A reasonable in-bound meal plan should
normalize successfully before the deterministic verifier has any chance to be
useful.

### P0 Goals And Metric Translation

P0's product goal is to define and harden the acceptable pasted-meal-plan input
boundary described in `docs/product.md` and
`docs/meal-plan-input-robustness.md`. A user should be able to paste a concise
ingredient-level meal plan that follows the public guidance and get a reliable
normalization result. Inputs outside that boundary should fail before queueing
or preserve unresolved fields with clear guidance.

P0 translates that goal into normalization metrics, not nutrition metrics:

- acceptable-input coverage -> `case_success_rate` and
  `normalization_success_rate`
- the hosted default example works -> `preloaded_example_success`
- no hidden day or meal-count override -> `day_assignment_accuracy`,
  `meal_assignment_accuracy`, and per-tag pass rates for one-day, two-day,
  three-day, and snack-inclusive cases
- no source food disappears or duplicates -> `source_item_preservation_rate`,
  `omitted_source_item_count`, and `duplicate_source_item_count`
- quantities and units survive normalization -> `quantity_accuracy`,
  `unit_accuracy`, and `unsupported_unit_false_failure_count`
- food phrases remain usable for the resolver -> `food_phrase_accuracy`
- local-model output can enter MealCheck -> `schema_validity_rate`,
  `adapter_valid_cases`, provider failure count, and compact-output decode
  failure count
- vague or unsupported inputs fail at the right boundary ->
  qualification-failure pass rate and `post_queue_normalization_failure_count`
- live model runs are operationally usable -> repeat stability,
  `normalization_latency_ms_p50`, and `normalization_latency_ms_p95`

The current seed implementation reports a subset of these directly. Some
accuracy metrics are currently represented as row-level mismatch messages and
aggregate row-match rates rather than as separate first-class fields.

The current checked-in P0 corpus is small and hand-authored:

- `examples/meal-plan-input-robustness/manifest.json`: acceptable pasted meal
  plans with expected day counts, meal-code coverage, item counts, and tags.
- `examples/meal-plan-input-robustness/failure-manifest.json`: qualification
  failures that should be refused before model normalization.
- `data/evaluation/p0-normalization/manifest.json`: generated P0 evaluation
  manifest for the current reviewed seed corpus.
- `data/evaluation/p0-normalization/cases-v1.jsonl`: strict one-day success
  cases with expected source items and compact-row fields.
- `data/evaluation/p0-normalization/multiday-exploratory-cases-v1.jsonl`:
  broader multi-day success cases retained for local/self-hosted comparison,
  not the hosted strict gate.
- `data/evaluation/p0-normalization/failure-cases-v1.jsonl`: qualification
  failure cases with expected failure status.
- `scripts/test-meal-plan-input-robustness.sh`: local-model smoke harness for
  compatible acceptable-input cases.
- `mealcheck eval-normalization`: P0 runner for deterministic source inventory,
  compact-row adapter, qualification-failure, tag-summary, failure-summary, and
  opt-in local-model baseline metrics. Use `-export-jsonl` or `-export-csv`
  to write flat per-case rows for cross-commit comparison.

The checked-in seed can be extended with public ingredient-parsing datasets as
source material for generated MealCheck normalization cases:

- NYT Ingredient Phrase Tagger:
  `https://github.com/nytimes/ingredient-phrase-tagger`
- TASTEset:
  `https://arxiv.org/abs/2204.07775`

NYT Ingredient Phrase Tagger is the best first source for item-level gold data.
Its labeled ingredient phrases map closely to MealCheck's compact rows:
quantity, unit, food name, and comment. It does not contain meal-plan structure,
so MealCheck should wrap selected ingredient lines in generated day and meal
contexts.

TASTEset is the best second source for harder natural language around food
entities. Its recipe NER labels include food products, quantities, units,
processes, and qualities. It is useful for prep adjectives, multi-token food
boundaries, and recipe-like language. It should be transformed into MealCheck
meal-plan snippets only after filtering out cases that require recipe
decomposition or unsupported quantity inference.

Both datasets should be treated as source data for evaluation generation, not
as direct training data for MealCheck. Do not train or fine-tune the local
model in this slice, expand the resolver catalog from this dataset, treat recipe
instructions as valid P0 success cases, make fuzzy food matching part of the P0
score, or check in raw third-party source datasets until license and size
handling are reviewed.

Optional NYT and TASTEset generation support lives in `tools/mealcheck_data`.
Raw third-party source files stay local, generated external outputs remain
exploratory until manual review, and active promotion decisions belong in
[Current Priorities](current-priorities.md).

### P0 Case Format

Generated P0 cases should live under:

```text
data/evaluation/p0-normalization/
```

Use JSONL for generated cases so larger datasets stay diffable and streamable.
Each success case should include the raw prompt text, expected source item
inventory, expected compact rows, and tags:

```json
{
  "schema_version": "0.1",
  "id": "nyt_supported_unit_000001",
  "source_dataset": "nyt_ingredient_phrase_tagger",
  "source_ref": {
    "row_id": "12345"
  },
  "input_text": "Day 1 breakfast:\n- 1 cup cooked oatmeal\n- 1/2 cup blueberries",
  "expected": {
    "days": [1],
    "source_items": [
      {
        "source_item_id": 1,
        "day": 1,
        "meal_code": "b",
        "source_text": "1 cup cooked oatmeal",
        "food": "cooked oatmeal",
        "quantity": 1,
        "unit": "cup"
      }
    ]
  },
  "tags": [
    "success",
    "supported_unit",
    "fraction",
    "generated_day_context"
  ]
}
```

Failure cases should use the same envelope, but their expectation should name
the intended failure stage and reason:

```json
{
  "schema_version": "0.1",
  "id": "nyt_unsupported_unit_000001",
  "source_dataset": "nyt_ingredient_phrase_tagger",
  "input_text": "Day 1 breakfast:\n- 1 handful almonds",
  "expected_failure": {
    "stage": "source_inventory",
    "reason": "unsupported_unit"
  },
  "tags": ["failure", "unsupported_unit"]
}
```

### P0 Dataset Generation

The generator command is:

```text
PYTHONPATH=tools/mealcheck_data/src python3 -m mealcheck_data generate-p0-normalization-evaluation
```

The generator reads optional external source files from environment variables:

```text
MEALCHECK_NYT_INGREDIENTS_CSV=/tmp/mealcheck-p0/nyt-ingredients-snapshot-2015.csv
MEALCHECK_TASTESET_CSV=/tmp/mealcheck-p0/TASTEset.csv
```

The generator writes deterministic artifacts:

```text
data/evaluation/p0-normalization/source-manifest.json
data/evaluation/p0-normalization/manifest.json
data/evaluation/p0-normalization/cases-v1.jsonl
data/evaluation/p0-normalization/failure-cases-v1.jsonl
```

When local NYT or TASTEset source CSVs are provided, the generator also writes
separate exploratory files:

```text
data/evaluation/p0-normalization/nyt-cases-v1.jsonl
data/evaluation/p0-normalization/nyt-failure-cases-v1.jsonl
data/evaluation/p0-normalization/nyt-quarantine-v1.jsonl
data/evaluation/p0-normalization/tasteset-cases-v1.jsonl
data/evaluation/p0-normalization/tasteset-failure-cases-v1.jsonl
data/evaluation/p0-normalization/tasteset-quarantine-v1.jsonl
```

Probe local source files before generation:

```bash
PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data generate-p0-normalization-evaluation \
  --probe-sources \
  --nyt-csv "$MEALCHECK_NYT_INGREDIENTS_CSV" \
  --tasteset-csv "$MEALCHECK_TASTESET_CSV"
```

Generation rules:

1. Keep only rows with a parseable numeric quantity, unit, and food/product
   span for success cases.
2. Map units only when the conversion is already supported by MealCheck's
   normalization boundary: `g`, `oz`, `cup`, `tbsp`, `tsp`, `slice`, and
   `serving`.
3. Exclude ranges, optional amounts, "to taste", "as needed", and vague size
   terms from success cases.
4. Preserve preparation and quality words when they are part of the food phrase
   a user would naturally paste, such as `cooked oatmeal` or `chopped carrots`.
5. Put unsupported but common units into the failure set instead of silently
   dropping them.
6. Stratify selected cases by unit, fraction style, decimal style, food-name
   length, prep adjective, comment text, and source dataset.
7. Use a fixed random seed and stable sorted IDs so generation is reproducible.

The source datasets contain ingredient lines, not full MealCheck plans. The
generator should wrap selected item phrases into deterministic meal-plan
contexts:

- one-day canonical bullets
- one-day inline sentences
- one-day paragraph meal spans
- numbered list items
- one-day reverse measurements, such as `chicken, 100 g`
- two-day clear `Day N` sections
- one-day snack-inclusive plans
- one-day snack-inclusive paragraphs
- compact multi-day text near the public input style
- natural rewrites with `with`, `plus`, commas, and `of` after the unit

The wrapper templates may be synthetic because the target is the MealCheck
normalization contract, not recipe authenticity. The item-level gold comes from
public annotation; the day and meal structure is generated so MealCheck-specific
grouping can be tested.

### P0 Evaluation Runner

The P0 runner should operate in three tiers.

Tier 1: deterministic source-inventory tests.

- Run in ordinary CI.
- Use MealCheck's deterministic source item inventory logic.
- Verify expected item count, source item order, day, meal code, source text,
  quantity token, and unit token.
- Does not require llama.cpp.

Tier 2: adapter contract tests.

- Feed expected full compact rows into the existing adapter.
- Verify canonical plan JSON loads and preserves day, meal, quantity, unit, and
  food values.
- Does not require llama.cpp.

Tier 3: local-model normalization eval.

- Run the same per-meal chunked local-model workflow used by hosted live runs.
- Each model call sees one meal text span and only that meal's source items.

- Run manually or in a scheduled environment with the MacBook model server.
- Send `input_text` through the same per-meal compact extraction path used by
  live runs.
- Compare normalized canonical plan JSON against expected rows.
- Record provider, decode, and row-match failure classes.
- Support repeats per case because the model path can be nondeterministic.

The deterministic implementation uses this command:

```bash
go run ./cmd/mealcheck eval-normalization \
  -gate strict \
  -out /tmp/mealcheck-p0-normalization.json
```

Run optional external generated cases separately:

```bash
go run ./cmd/mealcheck eval-normalization \
  -gate exploratory \
  -source-dataset nyt_ingredient_phrase_tagger \
  -out /tmp/mealcheck-p0-nyt-normalization.json
```

The opt-in local-model implementation uses this command:

```bash
MEALCHECK_LOCAL_MODEL_NAME="$MODEL_NAME" \
go run ./cmd/mealcheck eval-normalization \
  -mode local-llama \
  -gate strict \
  -local-model-repeats 3 \
  -out /tmp/mealcheck-p0-local-model.json
```

### P0 Metrics And Gates

Report P0 metrics separately for deterministic and local-model tiers:

- `case_success_rate`
- `source_item_preservation_rate`
- `omitted_source_item_count`
- `duplicate_source_item_count`
- `day_assignment_accuracy`
- `meal_assignment_accuracy`
- `quantity_accuracy`
- `unit_accuracy`
- `food_phrase_accuracy`
- `schema_validity_rate`
- `unsupported_unit_false_failure_count`
- `post_queue_normalization_failure_count`
- `normalization_latency_ms_p50`
- `normalization_latency_ms_p95`
- `repair_attempt_rate`

For model-tier runs, also emit a ranked failure inventory:

- `source_inventory_failed`
- `model_json_invalid`
- `source_item_omitted`
- `source_item_duplicated`
- `wrong_day`
- `wrong_meal`
- `wrong_quantity`
- `wrong_unit`
- `wrong_food_phrase`
- `adapter_validation_failed`
- `checker_load_failed`

Required P0 release gates:

- The current hand-curated robustness corpus passes.
- The preloaded hosted example passes locally and through the deployed path.
- Deterministic source-inventory tests pass for the generated P0 success set.
- Failure cases fail before model queueing or preserve unresolved quantities
  with specific, user-facing guidance.

Track but do not immediately release-block on:

- large generated NYT local-model success rate
- large generated TASTEset local-model success rate
- latency percentiles over the generated corpus
- per-tag model weaknesses

This split prevents a large generated dataset from blocking small urgent fixes
while still making normalization reliability measurable.

Current strict deterministic P0 seed result:

- 9 one-day success cases pass deterministic source-inventory and adapter
  checks.
- 5 qualification-failure cases pass expected-status checks.
- 85 of 85 expected source items are preserved, for a
  `source_item_preservation_rate` of 1.0.
- The seed result has zero mismatches.

Exploratory multi-day P0 cases:

- 3 success cases remain available under the `exploratory` gate.
- They cover 63 expected source items.
- They are useful for local/self-hosted comparison, but they are outside the
  hosted one-day local-model contract and must not determine the default P0
  live gate.

Serving MacBook live local-model result before the strict one-day gate split:

- Run date: 2026-06-30.
- MealCheck commit: `094e8073b76a8564abe0d9bbf6ec992e661e87a6`.
- Model: `Qwen3-0.6B-Q4_K_M.gguf`.
- Model SHA-256:
  `18ea1f301079bba6391ab6d455c0c8565fd5a3214075eb2cd9daf351dedc719b`.
- llama.cpp commit: `7c082bc417bbe53210a83df4ba5b49e18ce6193c`.
- Output directory:
  `/Users/chranama-server/MealCheck-data/p0-runs/p0-live-local-model-20260630-094e807`.
- Deterministic baseline passed: 14 of 14 cases, zero mismatches, 139 of 139
  source items preserved.
- The live gate failed because the old strict gate still included
  `robustness_three_day_compact`, which is outside the current hosted one-day
  contract.
- Repeat 1 passed all 11 success cases. Repeats 2 and 3 each had one compact
  decode failure on `robustness_three_day_compact`.
- Aggregate: 3 of 3 repeats completed, 0 provider failures, 2 decode failures,
  min row/food/quantity/unit accuracy 0.8058, max duration 676 seconds.

Most recent serving MacBook strict one-day live local-model result:

- Run date: 2026-07-01.
- MealCheck commit: `22f942679c30aa1d59a250159418cde337597e10`.
- Model: `Qwen3-0.6B-Q4_K_M.gguf`.
- Model SHA-256:
  `18ea1f301079bba6391ab6d455c0c8565fd5a3214075eb2cd9daf351dedc719b`.
- llama.cpp commit: `7c082bc417bbe53210a83df4ba5b49e18ce6193c`.
- Output directory:
  `/Users/chranama-server/MealCheck-data/p0-runs/p0-live-local-model-20260701-122257-`.
- Deterministic baseline passed: 14 of 14 strict cases, zero mismatches, 85 of
  85 source items preserved.
- Live gate passed: 3 of 3 repeats completed, zero mismatches, zero provider
  failures, zero compact-output decode failures.
- Live local-model scoring covered 27 accepted one-day runs and 255 expected
  source rows across repeats.
- Minimum row, food, quantity, and unit accuracy were each 1.0.
- Source-grounded reconciliation made 126 deterministic field repairs across
  the three repeats and 26 repair-case observations.
- Maximum run duration was 272 seconds.
- A previous run at commit `d509577d7045558c3575739a710781c0ba0195f0`
  exposed a stochastic reverse-measurement omission in one breakfast chunk. The
  current passing run followed source-first prompt hardening.

Reverse-measurement deployed artifact inspection:

- Run date: 2026-07-01.
- Deployed run: `run_d7e150060a2632e0911d1867`.
- Collected artifact directory:
  `/Users/chranama-server/MealCheck-data/p0-runs/reverse-target-20260701-123326-22f9426`.
- The chunk artifact showed the refreshed source-first prompt:
  "Authoritative source items" before "Context-only meal text".
- The run completed with 3 meal chunks, 9 source items, exact source-ID
  preservation, and 7.775 seconds of recorded local-model extraction time.
- Breakfast and lunch raw compact outputs matched without repairs. Dinner
  preserved all three source IDs but emitted unresolved comma food placeholders;
  deterministic reconciliation repaired salmon, sweet potato, and olive oil from
  the source measurements with 11 field repairs.
- This is acceptable for the current strict gate because the SLM preserved row
  identity and the deterministic harness repaired fields grounded in source
  text. It remains a watch item if similar repair-heavy outputs become frequent
  or turn into omissions.

Previous P0 live local-model seed result before per-meal chunking:

- 3 of 3 repeats completed against the production `Qwen3-0.6B-Q4_K_M` model
  on the prototyping laptop.
- The strict live gate passed with `MEALCHECK_P0_MIN_ROW_MATCH_RATE=1`.
- Minimum local-model row match rate was 1.0.
- Minimum local-model food, quantity, and unit accuracy were each 1.0.
- Provider failures and compact-output decode failures were both zero.
- Source-grounded reconciliation made 306 field repairs across the three
  repeats, so the seed now passes through deterministic repair rather than
  because the model emits perfect rows unaided.
- The current per-meal chunked local-model contract needs a fresh live regimen
  run before the live result is treated as production evidence.

The live local-model improvement loop is tracked in
[Current Priorities](current-priorities.md) and milestone history is recorded in
[Implementation Plan](implementation-plan.md).

### P0 Live Local-Model Regimen

The P0 live local-model regimen evaluates MealCheck's P0 normalization task
against a live llama.cpp-compatible model endpoint on a prototyping laptop or
server. It is intended for fast prompt, schema, parser, and model-server
iteration. It is not a substitute for final acceptance on the serving MacBook.

Goal:

Measure whether the live local model can turn acceptable pasted meal-plan text
into per-meal compact MealCheck rows without losing source items, changing
quantities, changing units, or producing invalid compact JSON. Day and meal
assignment are determined by the source inventory before the model call; the
model returns only source id, food phrase, quantity, and unit for each meal
chunk.

The current checked-in P0 corpus is still a seed corpus:

- 9 strict one-day success cases
- 3 exploratory multi-day success cases
- 5 qualification failure cases
- 85 strict expected source items
- 63 exploratory expected source items
- supported units only: `g`, `oz`, `cup`, `tbsp`, `tsp`, `slice`, `serving`

The default live-model regimen uses the strict gate, so it measures extraction
quality on the hosted one-day success cases. Failure cases are still checked
through deterministic qualification. Use `mealcheck eval-normalization -gate
exploratory` when intentionally comparing broader local/self-hosted inputs.

Preconditions:

- Start one local model behind an OpenAI-compatible `/v1` endpoint. The default
  endpoint is `http://127.0.0.1:11435/v1`.
- The script expects `curl`, `git`, `go`, `jq`, a running
  llama.cpp-compatible server, and a clean enough MealCheck worktree to make the
  run interpretable.
- The script does not start llama.cpp and does not download models.

Fast iteration run:

```bash
MEALCHECK_P0_REPEATS=1 \
MEALCHECK_P0_ALLOW_MISMATCH=1 \
scripts/run-p0-local-model-regimen.sh
```

This mode is for finding obvious failure classes. It should not be treated as a
pass/fail gate.

Baseline run:

```bash
MEALCHECK_P0_REPEATS=3 \
MEALCHECK_P0_LOCAL_MODEL_BASE_URL=http://127.0.0.1:11435/v1 \
MEALCHECK_P0_LOCAL_MODEL_NAME="$MODEL_NAME" \
scripts/run-p0-local-model-regimen.sh
```

If `MEALCHECK_P0_LOCAL_MODEL_NAME` is omitted, the script uses the first model
id from `/v1/models`.

The baseline gate requires:

- deterministic P0 eval passes first
- every live repeat exits successfully
- zero provider failures
- zero compact-output decode failures
- zero case mismatches
- `local_model_row_match_rate` is at least `MEALCHECK_P0_MIN_ROW_MATCH_RATE`,
  which defaults to `1`

Useful optional knobs:

- `MEALCHECK_P0_OUTPUT_DIR`: result directory
- `MEALCHECK_P0_MAX_OUTPUT_TOKENS`: local-model output cap, default `1536`
- `MEALCHECK_P0_LOCAL_MODEL_TIMEOUT`: model request timeout, default `240s`
- `MEALCHECK_P0_CURL_MAX_TIME_SECONDS`: endpoint health-check timeout, default
  `20`
- `MEALCHECK_P0_REQUIRE_CLEAN_WORKTREE=1`: require no uncommitted changes

For release-candidate confidence on the prototyping laptop, run five repeats:

```bash
MEALCHECK_P0_REPEATS=5 \
scripts/run-p0-local-model-regimen.sh
```

Captured artifacts:

Each run writes a timestamped directory under `/tmp` unless
`MEALCHECK_P0_OUTPUT_DIR` is set. The directory contains:

- `metadata.json`: git commit, branch, dirty status, model endpoint, model id,
  optional model SHA/build labels, Go version, OS, CPU, memory, repeat count,
  and gate threshold
- `models-response.json`: raw `/v1/models` response
- `git-status.txt`: short worktree status
- `deterministic-result.json`: offline P0 result
- `live-result.json`: repeat-aware local-model result from
  `mealcheck eval-normalization -mode local-llama -local-model-repeats N`
- `live-summary.jsonl`: one compact result row per repeat
- `summary.json`: aggregate gate result
- stdout/stderr files for deterministic and live runs

Set optional model labels when available:

```bash
MEALCHECK_P0_MODEL_SHA=<gguf-sha256> \
MEALCHECK_P0_LLAMA_BUILD=<llama.cpp-version-or-commit> \
scripts/run-p0-local-model-regimen.sh
```

Those labels make laptop-to-server comparison easier.

Reading results:

```bash
jq . /tmp/mealcheck-p0-local-model-*/summary.json
```

The highest-signal fields are:

- `gate.passed`
- `repeats_with_mismatches`
- `min_local_model_row_match_rate`
- `total_provider_failures`
- `total_decode_failures`
- `mismatch_case_ids`
- `max_duration_seconds`

Then inspect mismatched run files:

```bash
jq '.mismatches' /tmp/mealcheck-p0-local-model-*/live-result.json
```

Interpret failures by class:

- provider failures: model endpoint, timeout, server crash, or request
  incompatibility
- decode failures: model did not return valid compact MealCheck JSON
- row mismatches: model returned parseable rows but changed item count, day,
  meal, food phrase, quantity, or unit
- deterministic failures: source inventory or adapter logic changed before the
  model was tested

Regression-risk discipline:

The prototyping laptop is useful for fast iteration because it has better
hardware, but it can hide production failures. Treat laptop results as a
development baseline only.

Before promoting a prompt, schema, model, llama.cpp build, or extraction logic
change, repeat the same P0 regimen on the serving MacBook with:

- same MealCheck commit
- same GGUF file and SHA
- same llama.cpp build
- same endpoint configuration
- same context size, max output tokens, timeout, thread/GPU settings, and prompt
  cache settings where applicable

The serving MacBook remains authoritative for latency, capacity, timeout, and
queue-risk decisions.

### P0 Buildout Plan

Build P0 evaluation in slices that each produce a runnable artifact. The first
goal is not model fine-tuning; it is a measured loop that can say exactly which
normalization subtask failed and whether the failure is deterministic, prompt
related, or local-model related.

Slice 1: define the checked-in P0 evaluation contract. Current status:
implemented for the reviewed seed corpus.

- Add `data/evaluation/p0-normalization/source-manifest.json` with source
  dataset names, expected local source paths, source URLs, license notes, and
  generation commands.
- Add an initial `data/evaluation/p0-normalization/manifest.json` that records
  schema version, dataset version, case counts, tag vocabulary, supported units,
  and release-gate status.
- Keep raw public datasets out of the repository unless size and license review
  explicitly allow check-in.
- Add `go run ./cmd/mealcheck fixture-check` coverage for manifest shape and
  case-file consistency once generated artifacts exist.

Slice 2: implement deterministic P0 case loading and scoring. Current status:
implemented as `mealcheck eval-normalization`.

- Add a normalization-eval loader for JSONL success and failure cases.
- Score source item inventory without calling the local model.
- Score expected compact rows through the adapter without calling the local
  model.
- Emit a result JSON with pass/fail counts, per-tag metrics, and ranked failure
  reasons.
- This slice should be CI-safe and should not depend on llama.cpp.

Slice 3: generate a small reviewed NYT Ingredient Phrase Tagger subset. Current
status: generator support is present; checked-in NYT-derived cases are deferred
until source/license review.

- Read the NYT CSV from `MEALCHECK_NYT_INGREDIENTS_CSV`.
- Keep rows with parsed food name, numeric quantity, and supported MealCheck
  unit.
- Normalize units into MealCheck's supported vocabulary.
- Generate one-day and two-day meal-plan wrappers around selected ingredient
  rows.
- Start with a small checked-in sample, such as 100 success cases and 25 failure
  cases, so diffs remain reviewable.
- Quarantine rows with ranges, alternatives, optional ingredients, vague
  quantities, or comments that make the expected row debatable.

Slice 4: add TASTEset for harder span-boundary coverage. Current status:
generator support is present; checked-in TASTEset-derived cases are deferred
until source/license review.

- Read TASTEset from `MEALCHECK_TASTESET_CSV` or
  `MEALCHECK_TASTESET_DIR`.
- Reconstruct ingredient lines from span annotations.
- Keep lines with quantity, unit, and food spans for success cases when the unit
  is supported.
- Preserve useful `PROCESS` and `PHYSICAL_QUALITY` spans in the food phrase
  when they match MealCheck's public input boundary.
- Use unsupported units, missing quantities, ranges, and recipe-like lines as
  failure cases.
- Keep TASTEset separate in tags and metrics so it does not mask simpler NYT
  regressions.

Slice 5: add structure-focused cases from NHANES/WWEIA. Current status:
planned.

- Reuse the existing NHANES/WWEIA source-download convention already used by P1.
- Generate synthetic pasted text from real recall day, eating occasion, food
  description, and gram-weight rows.
- Use this layer to evaluate day/meal grouping and compact-row generation, not
  food-resolution quality.
- Preserve participant/day/eating-occasion source refs so failures can be
  traced.
- Keep a small reviewed checked-in sample first; larger generated coverage can
  remain an optional local artifact.

Slice 6: add failure-class datasets. Current status: qualification failures from
the reviewed seed corpus are implemented; generated unsupported-unit and
vague-quantity cases are planned.

- Derive `unsupported_unit`, `vague_quantity`, `range_quantity`, and
  `quantity_missing` cases from NYT and TASTEset source rows.
- Derive `recipe_or_menu_needs_decomposition` cases from recipe-like public
  corpora only after license review.
- Optionally use a generic out-of-scope intent dataset such as CLINC150 for
  `not_meal_plan` hard negatives, but keep those cases secondary because they
  are not food-domain examples.
- Every failure case should name the expected public failure category and the
  stage where it should fail.

Slice 7: add the local-model P0 runner. Current status: implemented as the
opt-in `mealcheck eval-normalization -mode local-llama` path.

- Add `go run ./cmd/mealcheck eval-normalization` as the stable command.
- Run deterministic tiers by default.
- Add an explicit `-mode local-llama` flag for local-model runs so accidental
  CI execution does not call llama.cpp.
- Score local-model provider failures, compact-output decode failures, and
  canonical row mismatches separately from deterministic source-inventory
  failures.
- Support repeat runs per success case with `-local-model-repeats`, aggregate
  per-repeat metrics, and report unstable cases separately from consistently
  wrong-output failures.
- Future: record compact output, canonical plan JSON, normalization events,
  failure details, and stage timings under a result directory.

Slice 8: connect evaluation to the engineering loop. Current status: initial
result summary is documented; running and summarizing a live local-model
baseline plus generated external-source summaries remains planned.

- Add a short result summary to `docs/evaluation.md` after the first baseline
  run.
- Promote stable high-signal generated cases into
  `examples/meal-plan-input-robustness/` when they represent public input
  formats that should always work.
- Keep generated dataset failures grouped by subtask:
  day/meal detection, item segmentation, quantity extraction, unit extraction,
  food phrase extraction, compact-row generation, and failure classification.
- Use the failure inventory to choose between deterministic parser changes,
  prompt changes, retrieval examples, constrained decoding, or model-training
  experiments.

The first useful milestone is a deterministic P0 eval that can run without the
model and prove that generated cases, source item inventory, and adapter
contracts are coherent. The second useful milestone is a local-model baseline
that reports where the current hosted path fails. Fine-tuning or other
weight-changing ML work should wait until those baselines show stable,
high-frequency model failures that cannot be handled more simply.

## P1 Food And Unit Resolution

P1 uses evaluation datasets to expand the local nutrient catalog from measured
resolver gaps and reviewed source data rather than from intuition. There are
two checked-in P1 layers:

1. an FNDDS-grounded synthetic regression layer for targeted workflow behavior
2. a WWEIA/NHANES dietary recall layer for real reported intake patterns

The local catalog is still intentionally bounded. It is not trying to replace
FoodData Central. Its job is fast, deterministic coverage for common foods,
CI-safe demos, and a clear basis for deciding which long-tail foods should
remain unresolved, resolve through the conservative FNDDS fallback, or move to
a future API-backed lookup path.

### P1 Goals And Metric Translation

P1's product goal is to make the normalized MealCheck plan verifiable against
source-backed nutrient evidence without pretending that ambiguous foods are
known. P1 starts after P0 has already produced canonical MealCheck JSON.

P1 translates that goal into resolver and evidence metrics:

- common-food coverage improves -> `resolved_item_rate` overall, by dataset,
  and by category/tag
- resolver behavior does not regress -> expected-outcome mismatch count, which
  should remain zero for strict gates
- the local catalog is useful but bounded -> local-catalog resolved count,
  unresolved count, and unresolved frequency inventory
- fallback expansion is conservative -> FNDDS fallback coverage rate,
  auto-match count, blocked/quarantined count, and review-required count
- approximate nutrition is visible -> estimated resolution count,
  decomposed resolution count, and `estimated_or_decomposed_foods` warnings
- ambiguous, vague, unsupported, branded, or unsafe foods remain visible ->
  unresolved reason counts and unresolved reason quality
- unit handling is source-backed -> supported conversion count,
  missing-conversion count, and unsupported-unit unresolved count
- unresolved foods do not silently affect totals -> unresolved item count,
  excluded unresolved item count, and de minimis policy warning count

P1 does not judge whether the local model extracted the right meal-plan rows.
If a canonical plan contains the wrong food, quantity, unit, day, or meal
because normalization failed, that is a P0 failure even if the resolver handles
the resulting structured item correctly.

Current P1 result snapshot:

| Evaluation layer | Resolver configuration | Resolved items | Resolver rate | Expected mismatches | Notes |
|---|---|---:|---:|---:|---|
| FNDDS-grounded synthetic meal plans | original 17-food catalog | 296 / 900 | 32.89% | not a current gate | MVP baseline showing the original fixture catalog limit |
| FNDDS-grounded synthetic meal plans | expanded 159-food catalog | 885 / 900 | 98.33% | 0 | strict P1 regression gate; 15 unresolved items are intentional |
| WWEIA/NHANES real recalls | expanded 159-food catalog | 550 / 815 | 67.48% | 0 | real-recall no-fallback coverage; exposes bounded-catalog gaps |
| WWEIA/NHANES real recalls | expanded catalog plus FNDDS fallback | 774 / 815 | 94.97% | not compared | coverage run: 690 exact, 45 estimated, and 39 decomposed resolutions |

The strictest P1 pass/fail signal is not maximum coverage by itself. It is high
reviewed coverage with zero expected-outcome mismatches and visible unresolved,
estimated, or decomposed items when the system cannot make an exact
source-backed match.

## P1 Source Data

FNDDS source:

- USDA FNDDS 2021-2023 At A Glance, Foods and Beverages workbook
- USDA FNDDS 2021-2023 At A Glance, Nutrient Values workbook
- USDA FNDDS 2021-2023 At A Glance, Portions and Weights workbook

WWEIA/NHANES source:

- NHANES August 2021-August 2023 `DR1IFF_L.xpt`, Dietary Interview -
  Individual Foods, First Day
- NHANES August 2021-August 2023 `DR2IFF_L.xpt`, Dietary Interview -
  Individual Foods, Second Day
- NHANES August 2021-August 2023 `DEMO_L.xpt`, Demographics, used for adult
  age filtering

Source pages:

- `https://www.ars.usda.gov/northeast-area/beltsville-md-bhnrc/beltsville-human-nutrition-research-center/food-surveys-research-group/docs/fndds-download-databases/`
- `https://wwwn.cdc.gov/nchs/nhanes/search/datapage.aspx?Component=Dietary`
- `https://fdc.nal.usda.gov/download-datasets/`

FNDDS is appropriate for the local catalog because it is based on foods and
beverages reported in WWEIA/NHANES and includes both nutrient values and
familiar household portions. WWEIA/NHANES is appropriate for the real-recall
evaluation layer because it contains public, deidentified 24-hour dietary
interview rows with participant/day, eating occasion, food code, grams, and
nutrient values.

The source workbooks and XPT files are not checked into the repository.
Regeneration expects them at:

```bash
/tmp/mealcheck-fndds-2021-2023/foods-and-beverages.xlsx
/tmp/mealcheck-fndds-2021-2023/nutrient-values.xlsx
/tmp/mealcheck-fndds-2021-2023/portions-and-weights.xlsx
/tmp/mealcheck-nhanes-2021-2023/DR1IFF_L.xpt
/tmp/mealcheck-nhanes-2021-2023/DR2IFF_L.xpt
/tmp/mealcheck-nhanes-2021-2023/DEMO_L.xpt
```

## P1 Artifacts

- `data/nutrients/fixture-catalog-v1.json`: 159-food reviewed local catalog
  generated from FNDDS source rows.
- `data/evaluation/fndds-grounded-meal-plans-v1.json`: 100 structured
  meal-plan cases with source-like text, normalized plan JSON, settings, tags,
  source refs, and expected outcomes.
- `data/evaluation/wweia-nhanes-real-recalls-v1.json`: 100 real-recall
  evaluation cases generated from adult reliable WWEIA/NHANES dietary
  interview rows. Cases preserve reported gram weights and eating occasions.
- `data/evaluation/results/baseline-17-foods.json`: evaluation of the 100-case
  FNDDS-grounded dataset against the original 17-food local catalog.
- `data/evaluation/results/fndds-grounded-catalog-v1.json`: evaluation of the
  FNDDS-grounded dataset against the expanded FNDDS-grounded catalog.
- `data/evaluation/results/wweia-nhanes-real-recalls-v1.json`: evaluation of
  the WWEIA/NHANES real-recall dataset against the expanded local catalog.
- `data/evaluation/results/wweia-nhanes-real-recalls-with-fndds-fallback-v1.json`:
  coverage run for the same real-recall dataset with the optional FNDDS SQLite
  fallback enabled.
- `mealcheck eval-checker`: deterministic runner for case coverage, unresolved food
  frequency, category summaries, and expected-outcome mismatches.
- `tools/mealcheck_data`: reproducible generator package for the expanded
  fixture catalog, FNDDS-grounded evaluation dataset, WWEIA/NHANES real-recall
  evaluation dataset, and FNDDS reference import.

## P1 Dataset Mix

The FNDDS-grounded dataset contains:

- 20 balanced common-food plans
- 15 vegetarian plans
- 10 vegan plans
- 10 high-sodium plans
- 10 high-added-sugar plans
- 10 allergen-risk plans
- 10 low-protein plans
- 8 long-tail unresolved plans
- 7 vague-quantity plans

Each case contains exactly one day and three meals. This keeps individual cases
inspectable while still creating a 900-item resolver workload.

The WWEIA/NHANES real-recall dataset contains:

- 40 fully resolved real eating-occasion cases
- 30 high-coverage full adult recall days
- 20 high-sodium full adult recall days
- 10 low-protein full adult recall days

The real-recall layer uses adult reliable recalls only. It intentionally mixes
resolved real eating occasions with full-day recall cases that expose catalog
gaps. It should not be described as a meal-plan dataset. It is a structured
dietary recall dataset transformed into MealCheck evaluation cases.

## P1 Current Results

The original 17-food catalog resolves 296 of 900 items, a 32.89% resolver rate,
against the FNDDS-grounded dataset. This baseline intentionally shows the limit
of the MVP fixture catalog.

The expanded FNDDS-grounded catalog resolves 885 of 900 items, a 98.33%
resolver rate, with zero expected-outcome mismatches. The 15 unresolved items
are intentional:

- 8 long-tail foods that should stay unresolved until a FoodData Central
  fallback exists
- 7 vague quantities that should block until the user provides a quantity/unit

Against the WWEIA/NHANES real-recall dataset, the expanded catalog resolves
550 of 815 items, a 67.48% resolver rate, with zero expected-outcome
mismatches. This lower rate is expected: real dietary recalls include mixed
dishes, alcohol, branded-like snack variants, condiments, and specific
prepared-food forms that the bounded local catalog does not yet cover. The
latest reviewed batch added source-backed local coverage for tap water, plain
carbonated water, instant coffee, apple juice, white sugar, toasted white bread,
frozen cooked green peas, and white rice with no added fat. The top remaining
gap candidates now include wine, pasta with tomato-based sauce and meat, white
rolls, beer, cheese NFS, flavored liquid coffee creamer, saltine crackers, ham
sandwiches, pepper-type soft drinks, and flavored Greek yogurt.

With the optional FNDDS SQLite fallback enabled, the same WWEIA/NHANES
real-recall dataset resolves 774 of 815 items, a 94.97% resolver rate. The run
contains 690 exact resolutions, 45 estimated approximation resolutions, and 39
decomposed resolutions. This is a coverage run, not a strict
expected-outcome regression, because the checked-in WWEIA expected unresolved
counts describe the no-fallback catalog mode. The fallback removes common
source-backed gaps such as white rolls, sandwich vegetable components, simple
source-coded beverages, raw fruit, nuts, and selected source-coded NFS rows
while leaving ambiguous composed, restaurant/product-style, review-required,
and unsupported-unit rows unresolved. The reviewed local catalog now handles
tap water, 100% apple juice, instant coffee, granulated sugar, no-added-fat
vegetables and rice, and plain carbonated water before fallback.

## P1 Catalog Expansion Policy

Expansion rules:

1. Add foods in reviewed batches driven by evaluation coverage and unresolved
   frequency.
2. Keep matching conservative: exact normalized names plus preprocessed,
   auto-approved match keys.
3. Keep units per food explicit and source-backed; unsupported units remain
   unresolved.
4. Mark ambiguous, branded, vague, and long-tail foods unresolved instead of
   guessing.
5. Store source references on every generated food.
6. Keep source workbooks out of CI; check in deterministic generated JSON.

Runtime lookup order:

1. Use the reviewed local catalog first.
2. If no reviewed catalog match exists, pass the item through the fallback
   resolver gate. The gate only allows quantified foods with units supported by
   the fallback surface and names that appear specific enough for automatic
   lookup.
3. The gate blocks broad one-word foods, mixed dishes that need ingredient
   decomposition, restaurant or branded foods, unclear preparation, non-food
   text, and unsupported fallback units.
4. If a blocked source-backed FNDDS food code or broad natural food name has a
   curated or generated approximation proxy, and no configured constraint makes
   proxy use unsafe, resolve it as `estimated`. Generated proxies are derived
   from conservative FNDDS categories such as raw fruit, simple beverages, nuts,
   milk, simple proteins, legumes, and simple toppings.
5. If a blocked composed food has a curated exact decomposition template or a
   source-code-backed family decomposition rule, and it has a deterministic
   gram or ounce quantity, split the item across FNDDS component foods and
   resolve it as `decomposed`.
6. If the gate allows lookup, optionally check the FNDDS SQLite fallback.
7. If allowed fallback lookup misses, broad proxies and decomposition rules may
   still resolve the item when their safety gates match.
8. The fallback only resolves preprocessed FNDDS match keys whose
   `resolver_status` is `auto` and whose match is unique.
9. The fallback supports gram units plus source-backed food-specific unit
   conversions derived from FNDDS Portions and Weights.
10. Explicit `unknown_food` items with quantities may retry the fallback.
   Explicit vague quantities, unsupported units, and other unresolved reasons
   remain unresolved.
11. Quarantined or review-required FNDDS rows are never automatic exact resolver hits
   unless a specific generated match key is explicitly marked `auto`; broad keys
   that map to multiple foods fail closed.

Estimated and decomposed items remain in `resolved-foods.json` with
`resolution_method`, confidence, proxy, reason, or component metadata. They also
produce an `estimated_or_decomposed_foods` warning so the report does not
silently treat approximate nutrition as an exact food match.

Approximation proxies are concepts, not literal `NFS` text rules. WWEIA/NHANES
eval rows can reach a proxy through `source_food_code`, which preserves the
reported FNDDS food code behind descriptions such as `Cheese, NFS`. Real manual
usage reaches the same proxy only through reviewed natural keys such as
`cheese`, `butter`, or `pickles`; specific foods still attempt exact resolution
first, while unreviewed ambiguous phrases remain blocked.

Unresolved verification policy:

- By default, any unresolved food or quantity remains in `unresolved-foods.json`
  and blocks `quantities_resolvable`.
- When `settings.verification_constraints.unresolved_policy.de_minimis_enabled`
  is explicitly enabled, MealCheck may exclude tiny unresolved mass items from
  nutrition totals only when the item has deterministic `g`, `gram`, `grams`,
  `oz`, `ounce`, or `ounces` quantity, has `unknown_food` or
  `missing_conversion:<unit>` as its unresolved reason, and stays within
  per-item, per-day, and per-day-count caps.
- De minimis exclusion is disabled when allergy or excluded-food constraints
  are configured.
- Excluded unresolved items are written to `excluded-unresolved-foods.json` and
  make `quantities_resolvable` warn rather than pass. They are never counted in
  nutrition totals.

FNDDS At A Glance does not publish added-sugar grams. MealCheck therefore uses
a documented proxy in the generated fixture: naturally sweet foods and plain
dairy receive `added_sugar_g = 0`, while explicitly sweetened categories such
as soda, candy, cookies, syrup, sweetened drinks, and higher-sugar cereal use
FNDDS total sugar as a conservative added-sugar proxy. This is adequate for
resolver and workflow evaluation, but it should be replaced by FPED or another
reviewed added-sugar source before making broader nutrition claims.

The SQLite fallback uses FNDDS total sugar as a conservative `added_sugar_g`
proxy for all fallback rows because FNDDS At A Glance does not publish
added-sugar grams.

## P1 FNDDS Reference Database

MealCheck also keeps a full local FNDDS 2021-2023 reference layer. This is not
the reviewed resolver catalog. Its job is to preserve all source rows and
precompute which foods are plausible future resolver candidates.

Artifacts:

- `data/reference/fndds/source-manifest.json`: source URLs, expected raw file
  paths, and generation command.
- `data/reference/fndds-2021-2023/foods.jsonl`: all imported FNDDS food rows
  with descriptions, category metadata, nutrients, portions, source refs, and
  candidate classification.
- `data/reference/fndds-2021-2023/nutrients.jsonl`: normalized nutrient rows.
- `data/reference/fndds-2021-2023/portions.jsonl`: normalized portion rows.
- `data/reference/fndds-2021-2023/resolver-candidates.jsonl`: eligible
  source-backed foods for future review.
- `data/reference/fndds-2021-2023/resolver-match-keys.jsonl`: canonical and
  alias match keys with resolver status, confidence, and block reason.
- `data/reference/fndds-2021-2023/unit-conversions.jsonl`: source-backed
  food-specific gram conversions derived from FNDDS portions.
- `data/reference/fndds-2021-2023/quarantined-foods.jsonl`: ambiguous,
  composed, restaurant/product-style, or unclear-preparation foods.
- `data/reference/fndds-2021-2023/review-required-foods.jsonl`: source rows
  that need manual handling before candidate use.
- `data/reference/fndds-2021-2023/approximation-proxies.json`: curated
  broad and source-code-backed generic-food proxies that can resolve as
  estimated nutrition.
- `data/reference/fndds-2021-2023/decomposition-templates.json`: curated
  composed-food templates with component fractions and FNDDS food codes.
- `data/reference/fndds-2021-2023/decomposition-rules.json`: curated
  family-level decomposition rules matched by FNDDS source food code first and
  narrow text terms second. Soup and stew coverage is intentionally limited to
  rules with explicit main components such as lentil soup, vegetable soup, beef
  soup, and beef stew with pasta.
- `data/reference/fndds-2021-2023/classification-summary.json`: counts by
  status and ambiguity flag.
- `data/reference/fndds-2021-2023/fndds.sqlite`: indexed read-only runtime
  fallback database generated from the same reference rows.

Current FNDDS reference import:

- 5,432 food rows preserved
- 3,056 eligible resolver candidates
- 2,375 quarantined rows
- 6,201 resolver match keys
- 25,928 source-backed unit conversions
- 95 approximation proxies: 16 curated and 79 generated
- 95 source-code mappings into approximation proxies
- 6 curated decomposition templates
- 31 curated decomposition rules
- 33 source-code mappings into decomposition rules
- 1 review-required row

The preprocessing classifier quarantines rows with signals such as `NFS`,
not-specified descriptions, mixed dishes, sandwiches, pizza, burritos,
restaurant/product-style wording, unclear added-fat preparation, and
multi-component allergen risk. Quarantined rows are not deleted; they remain
available for frequency mining and manual review pressure.

Regenerate the FNDDS reference layer after downloading the FNDDS workbooks:

```bash
PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data import-fndds-reference
```

## Running Evaluation

Run the current hand-authored P0 robustness smoke harness:

```bash
scripts/test-meal-plan-input-robustness.sh
```

Run the deterministic P0 normalization evaluation:

```bash
go run ./cmd/mealcheck eval-normalization \
  -out /tmp/mealcheck-p0-normalization.json
```

Write portable per-case P0 rows alongside the aggregate result:

```bash
go run ./cmd/mealcheck eval-normalization \
  -out /tmp/mealcheck-p0-normalization.json \
  -export-jsonl /tmp/mealcheck-p0-normalization.rows.jsonl \
  -export-csv /tmp/mealcheck-p0-normalization.rows.csv
```

Compare two portable P0 or P1 JSONL exports across commits:

```bash
PYTHONPATH=tools/mealcheck_ops/src \
  python3 -m mealcheck_ops compare-eval-exports \
  --baseline /tmp/before.rows.jsonl \
  --current /tmp/after.rows.jsonl \
  --out /tmp/eval-compare.json \
  --markdown /tmp/eval-compare.md
```

The comparison output matches rows by `eval_type`, `dataset_id`, and `case_id`,
then reports added and removed cases, regressions, fixes, still-failing cases,
changed metrics, and eval-specific metric summaries.

Summarize completed or failed hosted run artifacts into an operator review and
priority queue:

```bash
PYTHONPATH=tools/mealcheck_ops/src \
  python3 -m mealcheck_ops summarize-run-artifacts \
  --artifact-root /tmp/mealcheck-live-artifacts \
  --out /tmp/mealcheck-run-summary.json \
  --markdown /tmp/mealcheck-run-summary.md
```

The artifact summary consumes canonical run artifacts such as `manifest.json`,
`decision.json`, `report.json`, `review/normalized-plan-review.json`,
`unresolved-foods.json`, `optional/local-model-chunks.json`, and
`debug/normalization-failure.json`. It reports run status and decision counts,
then queues unresolved normalized rows, checker unresolved foods,
source-row count mismatches, deterministic normalization repairs, failed
chunks, normalization failures, timing outliers, and manifest-listed missing
artifacts for operator review. It also emits deterministic `clusters` and a
ranked `priority_queue` over repeated unresolved foods, source phrases, units,
failure stages, timing outliers, and repair-heavy local-model chunks.

Run the Python operator-tooling tests:

```bash
python3 -m unittest discover -s tools/mealcheck_ops/tests
```

Run the opt-in local-model P0 baseline when the local llama.cpp-compatible
service is available:

```bash
MEALCHECK_P0_REPEATS=3 \
MEALCHECK_P0_LOCAL_MODEL_NAME="$MODEL_NAME" \
scripts/run-p0-local-model-regimen.sh
```

The live-model regimen above captures model endpoint metadata, git metadata,
deterministic baseline results, repeated live-model results, and an aggregate
gate summary.

Regenerate the P1 catalog and dataset after downloading the FNDDS workbooks:

```bash
PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data generate-fndds-evaluation
```

Regenerate the P1 WWEIA/NHANES real-recall layer after downloading the NHANES
XPT files and the FNDDS foods workbook:

```bash
PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data generate-wweia-nhanes-evaluation
```

Run the expanded P1 catalog evaluation:

```bash
go run ./cmd/mealcheck eval-checker
```

Write a result artifact:

```bash
go run ./cmd/mealcheck eval-checker \
  -out data/evaluation/results/fndds-grounded-catalog-v1.json
```

Write portable per-case P1 rows alongside the aggregate result:

```bash
go run ./cmd/mealcheck eval-checker \
  -out /tmp/mealcheck-p1-checker.json \
  -export-jsonl /tmp/mealcheck-p1-checker.rows.jsonl \
  -export-csv /tmp/mealcheck-p1-checker.rows.csv
```

The same `mealcheck_ops compare-eval-exports` command compares P1 JSONL exports
and summarizes resolver metric deltas plus unresolved-food and unresolved-unit
changes.

Write the WWEIA/NHANES result artifact:

```bash
go run ./cmd/mealcheck eval-checker \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -out data/evaluation/results/wweia-nhanes-real-recalls-v1.json
```

Write the WWEIA/NHANES FNDDS fallback coverage artifact:

```bash
go run ./cmd/mealcheck eval-checker \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite \
  -skip-expected \
  -out data/evaluation/results/wweia-nhanes-real-recalls-with-fndds-fallback-v1.json
```

Run a comparison against another catalog:

```bash
go run ./cmd/mealcheck eval-checker \
  -catalog /path/to/catalog.json \
  -out data/evaluation/results/custom.json \
  -allow-mismatch
```

`-allow-mismatch` is useful for baseline or exploratory runs where expected
outcomes are not supposed to pass yet.

## CI Role

CI currently covers parts of both evaluation tasks.

P0 source-inventory coverage lives in hosted generation tests, the
hand-authored input robustness manifest, and the deterministic
`mealcheck eval-normalization` command. The `-mode local-llama` P0 baseline is
manual because it depends on llama.cpp availability.

`go run ./cmd/mealcheck fixture-check` verifies P1 fixture, dataset, and
reference-layer integrity:

- the expanded catalog has at least 100 foods
- catalog food IDs, names, and aliases do not collide
- allergens and food groups use reviewed labels
- every food has a positive gram conversion
- each checked-in evaluation dataset contains exactly 100 cases
- required evaluation categories are present in each dataset
- each case has expected outcomes and food items
- WWEIA/NHANES cases retain source refs and source metrics
- FNDDS reference artifacts exist and have internally consistent counts
- curated/generated approximation proxies, decomposition templates, and decomposition
  rules reference valid FNDDS food codes and have valid confidence/fraction
  metadata
- the FNDDS SQLite fallback has table counts, indexes, known statuses,
  approximation proxy rows, template rows, rule rows, and resolver examples
  consistent with the generated reference artifacts
- eligible FNDDS resolver candidates do not carry hard quarantine flags
- known ambiguous and allowlisted FNDDS examples classify as expected

The strict evaluation can be used as a release gate:

```bash
go run ./cmd/mealcheck eval-checker
```

The WWEIA/NHANES layer can also be run as a release or catalog-expansion gate:

```bash
go run ./cmd/mealcheck eval-checker \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json
```

The FNDDS fallback coverage run is useful for catalog expansion analysis but is
not a strict release gate:

```bash
go run ./cmd/mealcheck eval-checker \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite \
  -skip-expected
```

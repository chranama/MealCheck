# MealCheck Evaluation

MealCheck evaluates two distinct tasks. Keeping them separate matters because a
failure in the first task prevents the second task from running, and a failure
in the second task does not necessarily mean the model normalized the user's
input incorrectly.

Task 1 is P0 meal-plan normalization. It asks whether an in-bound pasted meal
plan can be turned into canonical MealCheck structure:

```text
natural meal-plan text
  -> numbered source item inventory
  -> compact row JSON
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
- `data/evaluation/p0-normalization/cases-v1.jsonl`: success cases with
  expected source items and compact-row fields.
- `data/evaluation/p0-normalization/failure-cases-v1.jsonl`: qualification
  failure cases with expected failure status.
- `scripts/test-meal-plan-input-robustness.sh`: local-model smoke harness for
  compatible acceptable-input cases.
- `mealcheck eval-normalization`: P0 runner for deterministic source inventory,
  compact-row adapter, qualification-failure, tag-summary, failure-summary, and
  opt-in local-model baseline metrics.

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

The concrete integration plan for these two external sources lives in
[P0 External Dataset Integration Plan](p0-external-dataset-integration-plan.md).

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

The generator script is:

```text
scripts/generate-p0-normalization-evaluation.py
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
python3 scripts/generate-p0-normalization-evaluation.py \
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
- numbered list items
- two-day clear `Day N` sections
- one-day snack-inclusive plans
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

- Feed expected compact rows into the existing adapter.
- Verify canonical plan JSON loads and preserves day, meal, quantity, unit, and
  food values.
- Does not require llama.cpp.

Tier 3: local-model normalization eval.

- Run manually or in a scheduled environment with the MacBook model server.
- Send `input_text` through the same compact local-model extraction prompt and
  adapter used by live runs.
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

Current P0 seed result:

- 8 success cases pass deterministic source-inventory and adapter checks.
- 3 qualification-failure cases pass expected-status checks.
- 120 of 120 expected source items are preserved, for a
  `source_item_preservation_rate` of 1.0.
- The seed result has zero mismatches.

Current P0 live local-model seed result:

- 3 of 3 repeats completed against the production `Qwen3-0.6B-Q4_K_M` model
  on the prototyping laptop.
- The strict live gate passed with `MEALCHECK_P0_MIN_ROW_MATCH_RATE=1`.
- Minimum local-model row match rate was 1.0.
- Minimum local-model food, quantity, and unit accuracy were each 1.0.
- Provider failures and compact-output decode failures were both zero.
- Source-grounded reconciliation made 306 field repairs across the three
  repeats, so the seed now passes through deterministic repair rather than
  because the model emits perfect rows unaided.

The live local-model improvement loop is tracked in
[Normalization Engine Improvement Plan](normalization-engine-improvement-plan.md).

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
- Future: record compact output, canonical plan JSON, normalization events,
  failure details, and stage timings under a result directory.
- Support repeat runs per case and report instability separately from ordinary
  wrong-output failures.

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
| FNDDS-grounded synthetic meal plans | expanded 151-food catalog | 885 / 900 | 98.33% | 0 | strict P1 regression gate; 15 unresolved items are intentional |
| WWEIA/NHANES real recalls | expanded 151-food catalog | 496 / 815 | 60.86% | 0 | real-recall no-fallback coverage; exposes bounded-catalog gaps |
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

- `data/nutrients/fixture-catalog-v1.json`: 151-food reviewed local catalog
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
- `mealcheck eval`: deterministic runner for case coverage, unresolved food
  frequency, category summaries, and expected-outcome mismatches.
- `scripts/generate-fndds-evaluation.py`: reproducible generator for the
  expanded fixture catalog and FNDDS-grounded evaluation dataset.
- `scripts/generate-wweia-nhanes-evaluation.py`: reproducible generator for the
  WWEIA/NHANES real-recall evaluation dataset.

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
496 of 815 items, a 60.86% resolver rate, with zero expected-outcome
mismatches. This lower rate is expected: real dietary recalls include tap
water, mixed dishes, alcohol, branded-like snack variants, condiments, and
specific prepared-food forms that the bounded local catalog does not yet cover.
The top gap candidates currently include tap water, white rolls, granulated
sugar, wine, apple juice, instant coffee, saltine crackers, flavored liquid
coffee creamer, and common mixed dishes.

With the optional FNDDS SQLite fallback enabled, the same WWEIA/NHANES
real-recall dataset resolves 774 of 815 items, a 94.97% resolver rate. The run
contains 690 exact fallback resolutions, 45 estimated approximation resolutions,
and 39 decomposed resolutions. This is a coverage run, not a strict
expected-outcome regression, because the checked-in WWEIA expected unresolved
counts describe the no-fallback catalog mode. The fallback removes common
source-backed gaps such as water, 100% juice, instant coffee, white rolls,
granulated sugar, no-added-fat vegetables and rice, sandwich vegetable
components, simple source-coded beverages, raw fruit, nuts, and selected
source-coded NFS rows while leaving ambiguous composed, restaurant/product-style,
review-required, and unsupported-unit rows unresolved.

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
python3 scripts/import-fndds-reference.py
```

## Running Evaluation

Run the current hand-authored P0 robustness smoke harness:

```bash
scripts/test-meal-plan-input-robustness.sh
```

Run the deterministic P0 normalization evaluation:

```bash
go run ./cmd/mealcheck eval-normalization \
  -dataset data/evaluation/p0-normalization/cases-v1.jsonl \
  -out /tmp/mealcheck-p0-normalization.json
```

Run the opt-in local-model P0 baseline when the local llama.cpp-compatible
service is available:

```bash
MEALCHECK_P0_REPEATS=3 \
MEALCHECK_P0_LOCAL_MODEL_NAME="$MODEL_NAME" \
scripts/run-p0-local-model-regimen.sh
```

The full live-model regimen is documented in
`docs/p0-live-model-regimen.md`. It captures model endpoint metadata, git
metadata, deterministic baseline results, repeated live-model results, and an
aggregate gate summary.

Regenerate the P1 catalog and dataset after downloading the FNDDS workbooks:

```bash
python3 scripts/generate-fndds-evaluation.py
```

Regenerate the P1 WWEIA/NHANES real-recall layer after downloading the NHANES
XPT files and the FNDDS foods workbook:

```bash
python3 scripts/generate-wweia-nhanes-evaluation.py
```

Run the expanded P1 catalog evaluation:

```bash
go run ./cmd/mealcheck eval
```

Write a result artifact:

```bash
go run ./cmd/mealcheck eval \
  -out data/evaluation/results/fndds-grounded-catalog-v1.json
```

Write the WWEIA/NHANES result artifact:

```bash
go run ./cmd/mealcheck eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -out data/evaluation/results/wweia-nhanes-real-recalls-v1.json
```

Write the WWEIA/NHANES FNDDS fallback coverage artifact:

```bash
go run ./cmd/mealcheck eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite \
  -skip-expected \
  -out data/evaluation/results/wweia-nhanes-real-recalls-with-fndds-fallback-v1.json
```

Run a comparison against another catalog:

```bash
go run ./cmd/mealcheck eval \
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
go run ./cmd/mealcheck eval
```

The WWEIA/NHANES layer can also be run as a release or catalog-expansion gate:

```bash
go run ./cmd/mealcheck eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json
```

The FNDDS fallback coverage run is useful for catalog expansion analysis but is
not a strict release gate:

```bash
go run ./cmd/mealcheck eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite \
  -skip-expected
```

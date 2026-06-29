# Normalization Engine Improvement Plan

This plan targets P0 meal-plan normalization: converting acceptable pasted
meal-plan text into canonical MealCheck rows. It does not cover P1 nutrition
resolution, FNDDS matching, approximation, or decomposition after rows already
exist.

## Current State

The local-model path already has a useful shape:

- `QualifyMealPlanText` rejects obvious non-meal-plan or too-vague text before
  model work.
- `localLlamaResolvedSourceItems` builds a deterministic source-item inventory
  with source item id, day, meal code, and source text.
- `localModelExtractionMessages` prompts the local model to return compact row
  JSON shaped as `{"i":[[source_item_id,day,meal_code,food,quantity,unit]]}`.
- `DecodeLocalLlamaCompactPlan` expands compact rows into canonical MealCheck
  plan JSON.
- `mealcheck eval-normalization` and
  `scripts/run-p0-local-model-regimen.sh` score deterministic and live local
  model behavior.

The latest live local-model run showed that the model can preserve row count,
day assignment, and meal assignment, but still changes row content. The main
failure classes were:

- quantity and unit embedded in `food` while `quantity`/`unit` are wrong
- fraction quantities parsed incorrectly
- `tbsp`/`tsp` substitutions
- preparation adjectives dropped from food phrases
- row-order or row-content swaps in compact multi-day examples

These are P0 failures because the verifier should receive the user's intended
food, quantity, unit, day, and meal without nutrition-critical drift.

## Implementation Status

Implemented:

- deterministic source-item measurement parser for integer, decimal, fraction,
  mixed-number, unit-alias, and `of` cleanup cases
- source-aware compact row decode that reconciles model output against
  deterministic source items by `source_item_id`
- canonical source-id row ordering for compact local-model rows
- hosted local-model run and qualification paths wired through source-aware
  decode
- normalization events for source-grounded repairs
- local-model prompt tightening for authoritative source items, fractions,
  `tbsp`/`tsp`, and preparation-word preservation
- P0 eval metrics for local-model day, meal, food, quantity, and unit accuracy
- P0 eval and regimen artifact fields for source repair counts

Current live seed result:

- 3 of 3 local-model P0 repeats completed
- 0 provider failures
- 0 compact-output decode failures
- 0 mismatched cases
- minimum row match rate: 1.0
- minimum food, quantity, and unit accuracy: 1.0
- total source-grounded field repairs across repeats: 306
- gate passed with `MEALCHECK_P0_MIN_ROW_MATCH_RATE=1`

Remaining planned work:

- first-class unsupported-unit qualification diagnostics
- larger reviewed NYT and TASTEset-derived P0 datasets
- broader model/runtime comparison after the reviewed seed remains stable

## Product Goal

An acceptable input under `docs/meal-plan-input-robustness.md` should normalize
successfully with:

- no hidden day-count or meal-count assumption
- every resolved source item represented exactly once
- stable day and meal assignment
- numeric quantity and supported unit preserved
- food phrase preserved well enough for downstream resolution
- unsupported or vague input rejected or preserved as unresolved before it
  becomes misleading nutrition math

## Target Architecture

Normalization should be a hybrid deterministic/model pipeline:

1. Qualification preflight decides whether text is in bounds.
2. Source inventory deterministically enumerates candidate food rows from the
   input text.
3. Deterministic measurement parser extracts quantity, unit, and candidate food
   phrase from each source item whenever possible.
4. The local model fills only the fields that still require language judgment,
   primarily food phrase cleanup and meal-code inference when context is weak.
5. Post-model reconciliation compares model rows back to the deterministic
   source inventory and repairs safe, source-grounded mismatches.
6. Strict validation either emits canonical MealCheck JSON or fails with a
   user-facing reason and operator-visible diagnostics.

The model should not be the only component responsible for exact numeric
measurement parsing. Small local models are useful for flexible language
boundaries, but deterministic code is better for exact quantities and units.

## Slice 1: Measurement Parser

Add a deterministic parser for a single source item string.

Inputs:

- source item id
- source text
- source day
- source meal code, or `infer`

Outputs:

- quantity as a positive number
- normalized unit: `g`, `oz`, `cup`, `tbsp`, `tsp`, `slice`, or `serving`
- food phrase with the leading quantity and unit removed
- parse status and failure reason

Required coverage:

- integers: `4 oz chicken breast`
- decimals: `0.5 cup blueberries`
- fractions: `1/2 cup blueberries`
- mixed numbers: `1 1/2 cups rice`
- unit aliases: grams, ounces, cups, tablespoons, teaspoons, slices, servings
- `of` cleanup: `1 cup of rice` -> `rice`
- punctuation cleanup around inline sentences

This parser should live near the current local-model source inventory code
unless it grows large enough to justify a dedicated package.

Acceptance:

- unit tests for the supported formats
- deterministic P0 eval still passes
- parser results can be emitted in debug artifacts or eval output

## Slice 2: Source-Grounded Row Reconciliation

After `DecodeLocalLlamaCompactPlan`, compare each compact row to the source
inventory by `source_item_id`.

Repair rules should be conservative:

- If the model returned the right source item id but embedded the source
  quantity/unit in the food field, replace `quantity`, `unit`, and `food` from
  the deterministic measurement parser.
- If the model changed `tbsp` to `tsp`, or another supported unit, but the
  source parser found an unambiguous unit, prefer the source parser.
- If the model changed a fraction quantity and the source parser found an
  unambiguous quantity, prefer the source parser.
- If the model dropped only leading measurement text from the food phrase,
  accept the cleaned deterministic food phrase.
- Do not repair day or meal assignment unless the source inventory supplied
  those fields unambiguously.
- Do not repair if the source item cannot be parsed deterministically.

Every repair should record a normalization event and an eval-visible reason,
for example:

- `measurement_repaired_from_source`
- `unit_repaired_from_source`
- `food_prefix_stripped_from_source`

Acceptance:

- live-model P0 mismatches for quantity/unit prefix errors are reduced without
  relaxing the gate
- no repair happens when the source parser is uncertain
- repaired output remains canonical MealCheck JSON

## Slice 3: Prompt Tightening

Revise `localModelExtractionMessages` after the deterministic repair layer is
in place. The prompt should align with the hybrid contract:

- source item ids are authoritative
- source day and meal code are authoritative when provided
- quantity and unit must come from the leading measurement in `source_text`
- food must preserve preparation adjectives present in `source_text`
- examples should cover fractions, `tbsp` versus `tsp`, `oz`, compact inline
  text, and multi-day rows

Keep examples short. The production model is small, so the prompt must stay
compact enough to avoid hurting latency or crowding the input.

Acceptance:

- one-repeat exploratory P0 live run improves or holds row-match rate
- three-repeat P0 regimen improves or holds repeat stability
- no increase in decode failures

## Slice 4: Row Alignment And Order Robustness

The current compact row schema includes `source_item_id`, which should make row
order less important. The adapter and eval should lean into that.

Work:

- make reconciliation source-id keyed before falling back to row order
- make eval report source-id mismatches separately from content mismatches
- detect duplicate or missing source ids as hard failures
- consider scoring content against source id rather than row index when all
  source ids are present and valid

Acceptance:

- row swaps are diagnosed as alignment issues, not many unrelated food,
  quantity, and unit mismatches
- duplicate/missing source ids remain hard failures
- deterministic adapter tests cover out-of-order rows

## Slice 5: Qualification Boundary And Unsupported Units

Keep the acceptable-input boundary clear. P0 success cases should use supported
units. Inputs with vague or unsupported units should not become false hard
failures after queueing.

Work:

- make qualification identify unsupported units in otherwise structured text
  before model extraction when possible
- preserve unsupported quantities as unresolved only when the product path is
  designed to continue with unresolved items
- keep unsupported-unit failure cases in the P0 eval dataset
- report unsupported-unit false failures separately from real unsupported-unit
  rejections

Acceptance:

- preloaded example and supported-unit seed cases do not fail for unit parsing
- unsupported-unit failure cases return the expected public category
- debug artifacts show whether the failure was qualification, source inventory,
  model decode, reconciliation, or final validation

## Slice 6: Evaluation Expansion

The current P0 seed corpus is useful but small. Expand only after the repair
loop has enough diagnostics to classify failures.

Work:

- add first-class metrics for quantity accuracy, unit accuracy, food phrase
  accuracy, source-id accuracy, and repair counts
- add reviewed NYT Ingredient Phrase Tagger derived cases once source handling
  is settled
- add reviewed TASTEset derived cases for harder food phrase boundaries
- keep large generated datasets optional and non-release-blocking at first

Acceptance:

- `mealcheck eval-normalization` reports the new metrics directly instead of
  only row-level mismatch strings
- `scripts/run-p0-local-model-regimen.sh` preserves those metrics in per-repeat
  and aggregate artifacts
- release gate remains strict on the reviewed seed corpus

## Slice 7: Model And Runtime Experiments

Use model/runtime changes as measured experiments, not as the first fix.

Experiments:

- compare the production `Qwen3-0.6B-Q4_K_M` model against one larger local
  candidate on the prototyping laptop
- test constrained JSON/schema settings supported by llama.cpp
- verify temperature and sampling are deterministic
- compare latency and memory pressure against the serving MacBook budget

Acceptance:

- every model experiment writes the P0 regimen artifact directory
- model SHA, llama.cpp build, settings, and endpoint are recorded
- no model is considered a production replacement until it passes the same
  regimen on the serving MacBook with acceptable latency

## Suggested Execution Order

1. Implement the deterministic measurement parser.
2. Add source-grounded row reconciliation and repair events.
3. Update P0 eval metrics to show repair counts and per-field accuracy.
4. Tighten the local-model prompt against the remaining mismatch classes.
5. Re-run the three-repeat P0 live regimen on the prototyping laptop.
6. If the seed gate still fails, inspect only the remaining mismatches before
   changing the model.
7. After the seed gate passes, expand NYT/TASTEset reviewed cases.
8. Run the same gate on the serving MacBook before treating the change as
   production-ready.

## Non-Goals

- Do not relax the P0 gate to hide quantity or unit drift.
- Do not use fuzzy FNDDS food resolution to compensate for bad P0 extraction.
- Do not fine-tune the model before deterministic parsing and reconciliation
  are exhausted.
- Do not add broad natural-language support outside the current acceptable
  input boundary without updating `docs/meal-plan-input-robustness.md` and P0
  eval cases.

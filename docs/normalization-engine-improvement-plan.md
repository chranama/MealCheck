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

## Refactor Plan: Deterministic-First LLM Assist

The next architecture step is to stop treating the local model as the default
normalization bridge from pasted text to canonical MealCheck JSON. The
deterministic source inventory and measurement parser are now strong enough to
become the primary path for supported inputs. The model should move to bounded
assist roles: fallback normalization for unresolved fragments, candidate
selection, decomposition suggestions, and user-facing explanations.

The target flow is:

```text
pasted text
  -> qualification preflight
  -> deterministic source inventory
  -> deterministic measurement parser
  -> deterministic canonical plan builder, if all source items are resolved
  -> optional LLM assist for unresolved or ambiguous chunks
  -> deterministic validation/reconciliation of assist output
  -> canonical MealCheck JSON
  -> resolver/checker/report
```

### Target Package Shape

Introduce a dedicated normalization package so hosted generation is no longer
the owner of source inventory and local-model row semantics:

```text
internal/normalization/
  types.go
  source_inventory.go
  measurement_parser.go
  deterministic_plan.go
  assist.go
  chunking.go
  validation.go
```

The package should expose a small orchestration API:

```go
type Engine struct {
    Assist Provider
    Policy Policy
}

func (e Engine) Normalize(ctx context.Context, input Request) (Result, error)
```

The result should distinguish how the plan was produced:

- `method: deterministic`
- `method: deterministic_with_llm_assist`
- `method: failed_pre_model`
- `method: failed_post_assist_validation`

It should also expose:

- source inventory rows
- parsed measurement rows
- unresolved or ambiguous rows
- assist requests and responses, when used
- normalization events
- confidence or review flags

### Slice A: Extract Current Deterministic Normalization Primitives

Move the current source inventory, measurement parsing, source item count, unit
normalization, and source-grounded reconciliation code from `internal/hosted`
into `internal/normalization`.

This should be a behavior-preserving extraction:

- keep existing function wrappers in `internal/hosted` temporarily
- keep `mealcheck eval-normalization` passing
- keep hosted local-model behavior unchanged
- add direct unit tests around the new package API

Acceptance:

- `go test ./...` passes
- P0 deterministic eval remains `11 / 11`
- local-model repeat eval output is unchanged for the current seed corpus

### Slice B: Add Deterministic Canonical Plan Builder

Add a deterministic builder that converts fully parsed source items directly
into canonical MealCheck plan JSON without calling the model.

The builder should require:

- every expected source item has a positive quantity
- every unit is in the supported unit vocabulary
- every food phrase is non-empty
- day and meal assignment are known or can be deterministically inferred under
  the accepted input boundary
- no source item id is missing or duplicated

Hosted `local_model` runs should try this path before any provider call.

Acceptance:

- preloaded hosted example normalizes without a provider call
- robustness seed cases that are fully parseable normalize deterministically
- normalization events show `deterministic_normalized` for deterministic runs
- inputs outside the deterministic boundary fail before queueing or move to the
  explicit assist policy, never silently guess

### Slice C: Introduce Assist Policy And Explicit Fallback Boundary

Add a policy layer that decides whether unresolved deterministic rows should:

- fail with user-facing clarification
- continue as unresolved rows, if the product path supports that
- be sent to bounded LLM assist

The first production-safe default should be conservative:

```text
supported explicit rows -> deterministic success
vague or unsupported quantities -> fail with guidance
natural-language rows -> optional LLM assist only behind a config flag
```

Acceptance:

- failure output names the exact source item and reason
- frontend can display deterministic failure guidance without model internals
- `post_queue_normalization_failure_count` does not rise

### Slice D: Implement Chunked LLM Assist

When assist is enabled, send compact source-item chunks rather than the full
meal-plan text.

Chunk boundaries should be source-item aware:

- first preference: one meal per chunk
- second preference: one day per chunk
- fallback: fixed-size source item groups while preserving item boundaries
- only unresolved or ambiguous rows should be sent when possible

The assist prompt should not ask the model to normalize the whole plan. It
should ask for one of a small set of actions per source item:

```json
{
  "source_item_id": 7,
  "action": "propose_row | needs_clarification | abstain",
  "food": "chicken rice soup",
  "quantity": 1,
  "unit": "serving",
  "confidence": "low",
  "message": "Please provide a measurable amount."
}
```

Validation must reject:

- missing source ids
- duplicate source ids
- unsupported units
- invented source ids
- rows for source items that deterministic policy did not allow the model to
  modify
- outputs that do not fit the strict assist schema

Acceptance:

- LLM input and output token counts are materially lower than full-plan
  normalization
- repeated assist eval can isolate unstable source items
- deterministic rows are never sent to the model unnecessarily
- merged output records which rows used assist

### Slice E: Split P0 Evaluation By Normalization Path

Update P0 eval so it no longer treats all normalization as one task.

Report separate metrics for:

- deterministic supported-input normalization
- pre-model clarification failures
- LLM assist fallback rows
- assist abstention accuracy
- assist false-accept rate
- repeat instability by source item
- latency by deterministic path versus assist path

The current local-model repeat support should become the basis for assist
repeat scoring rather than full-plan repeat scoring.

Acceptance:

- deterministic strict gate remains release-blocking
- LLM assist eval is tracked separately and can be exploratory at first
- generated NYT/TASTEset cases are tagged by the path they exercise

### Slice F: Add P1 LLM Candidate Assist After P0 Stabilizes

Do not add broad LLM food matching until deterministic normalization is the
primary path. Once P0 is stable, add an optional candidate-assist stage inside
the resolver.

The model should receive:

- user food phrase
- source item context
- top deterministic/FNDDS candidates
- category and nutrient summary fields

The model may only:

- select a provided candidate id
- return `ambiguous`
- return `no_safe_match`

It must not invent food ids or nutrient values.

Acceptance:

- P1 eval reports candidate-assist accuracy and abstention accuracy
- all selected candidate ids are validated before nutrition math
- approximate or assisted resolutions are visible in report artifacts

### Slice G: Report Explanation Layer

After deterministic math is complete, optionally use the LLM to produce
human-facing explanations:

- why normalization failed
- which rows used approximation or assist
- which foods drive a warning or block
- what user edit would improve confidence

This should be downstream of calculation. It should never compute nutrient
totals or alter decisions.

Acceptance:

- deterministic decision JSON remains the source of truth
- explanation artifacts cite the deterministic evidence they summarize
- missing or failed explanation generation does not fail the run

## Updated Execution Order

1. Extract source inventory and measurement parsing into
   `internal/normalization` without behavior changes.
2. Add the deterministic canonical plan builder.
3. Wire hosted `local_model` input to try deterministic normalization before
   provider calls.
4. Add method/confidence/review metadata to normalization artifacts.
5. Split P0 eval metrics by deterministic path, clarification failure, and LLM
   assist path.
6. Add conservative assist policy with assist disabled by default or limited to
   exploratory local runs.
7. Implement chunked source-item assist for unresolved fragments.
8. Promote stable assist cases into P0 only after repeat eval shows acceptable
   stability.
9. Add P1 candidate-assist experiments after deterministic P0 remains stable.
10. Add report explanation generation last, because it should not influence
    correctness.

Current implementation status:

- items 1-4 are implemented for hosted `local_model` runs
- `internal/normalization.Engine` is the normalization boundary for
  deterministic text normalization
- fully parsed explicit meal-plan text builds canonical MealCheck JSON without
  a provider call
- `optional/normalization-result.json` records method metadata, source
  inventory, parsed rows, unresolved pre-model rows, assist policy state, and
  provider fallback usage
- P0 eval reports deterministic canonical-plan path metrics separately from
  opt-in local-model repeat metrics, covering the deterministic portion of
  item 5
- conservative assist policy and chunking scaffolding exist, but LLM assist is
  not enabled as production behavior
- existing local-model compact decode remains as the fallback path when the
  deterministic builder cannot safely cover the input

## Non-Goals

- Do not relax the P0 gate to hide quantity or unit drift.
- Do not use fuzzy FNDDS food resolution to compensate for bad P0 extraction.
- Do not fine-tune the model before deterministic parsing and reconciliation
  are exhausted.
- Do not add broad natural-language support outside the current acceptable
  input boundary without updating `docs/meal-plan-input-robustness.md` and P0
  eval cases.
- Do not let LLM assist silently change deterministic rows that already parsed
  cleanly.
- Do not let the model invent food ids, nutrient values, source item ids, or
  quantities that are later treated as exact.

# LLM Assist Implementation Plan

MealCheck now has a deterministic-first normalization engine. The next LLM
work should be assistive, bounded, and evaluated separately from deterministic
verification. This plan covers two assist modes:

- P0 normalization assist: help convert unresolved source-item chunks into
  canonical MealCheck rows.
- P1 candidate assist: help select among already generated resolver candidates.

The shared rule is that the LLM may propose, classify, select, or abstain.
Deterministic code validates, merges, calculates, and decides.

## Current State

Implemented:

- `internal/normalization.Engine`
- deterministic source inventory
- deterministic measurement parser
- deterministic canonical plan builder
- source-item chunking
- model-backed P0 assist request/response contract
- P0 assist prompt construction, strict decoding, validation, and merge
- hosted provider adapter for schema-bound assist calls
- provider-level custom response schema support
- `optional/normalization-result.json`
- P0 deterministic path metrics in `mealcheck eval-normalization`
- exploratory P0 assist eval mode in `mealcheck eval-normalization`
- P1 candidate-assist request/response contract and validation scaffold

Not yet implemented:

- production enablement flags
- hosted production wiring for P0 assist
- P1 deterministic candidate export from unresolved resolver items
- P1 resolver merge/report integration
- P1 candidate-assist eval mode

## Shared Design Constraints

Both assist modes must follow these constraints:

- Assist is opt-in until eval evidence supports promotion.
- Deterministic success paths must never call the model.
- The model cannot invent source ids, food ids, nutrient values, quantities, or
  verification decisions.
- Every model output is schema validated before it can influence artifacts.
- Invalid model output becomes `abstain`, `needs_clarification`, or a controlled
  assist failure, not an opaque run failure.
- Assisted rows must remain visible in artifacts and reports.
- Repeat eval is required before production enablement.

## Shared Package Shape

Add a small assist abstraction that does not make `internal/normalization`
depend on `internal/hosted`:

```go
// internal/assist
type Client interface {
    Complete(ctx context.Context, request Request) (Response, error)
}

type Request struct {
    Task           string
    SchemaName     string
    ResponseSchema map[string]any
    Messages       []Message
}

type Response struct {
    RawText string
}
```

Hosted code can adapt the existing `hosted.Provider` to this interface. The
normalization and resolver packages should depend only on the assist interface,
not on hosted provider details.

Suggested files:

```text
internal/assist/client.go
internal/hosted/assist_adapter.go
internal/normalization/assist_contract.go
internal/normalization/assist_prompt.go
internal/normalization/assist_validation.go
internal/normalization/assist_merge.go
internal/checker/candidate_assist_contract.go
```

## P0 Normalization Assist

### Product Goal

P0 assist should reduce arbitrary normalization failures for in-bound meal-plan
text while preserving exact source-item accounting. It is not a general recipe
parser and should not expand the public acceptable-input boundary silently.

### Eligible Inputs

P0 assist may run only when:

- deterministic normalization failed
- deterministic source inventory found at least one source item
- unresolved items are limited to day, meal, food phrase, quantity, or supported
  unit normalization issues
- deterministic rows that parsed cleanly can be held fixed

P0 assist must not run when:

- source inventory found zero rows
- qualification says the text is not a meal plan
- the text is recipe-like and needs decomposition beyond source-item row repair
- the input asks for nutrition calculation or medical claims instead of a meal
  plan check

### P0 Assist Request

Each request should be one source-aware chunk:

```json
{
  "task": "p0_normalization_assist",
  "chunk_id": "chunk_1",
  "source_items": [
    {
      "id": 7,
      "day": 1,
      "meal_code": "l",
      "text": "one cup brown rice"
    }
  ],
  "allowed_units": ["g", "oz", "cup", "tbsp", "tsp", "slice", "serving"],
  "allowed_meal_codes": ["b", "m", "l", "a", "d", "s", "e"],
  "fixed_source_item_ids": [1, 2, 3, 4, 5, 6]
}
```

Only unresolved source items should be sent when possible. If meal/day context
is needed, include neighboring deterministic rows as read-only context, never
as editable rows.

### P0 Assist Response

The model must return one object with rows:

```json
{
  "items": [
    {
      "source_item_id": 7,
      "action": "propose_row",
      "day": 1,
      "meal_code": "l",
      "food": "brown rice",
      "quantity": 1,
      "unit": "cup",
      "confidence": "high",
      "message": ""
    }
  ]
}
```

Allowed actions:

- `propose_row`
- `needs_clarification`
- `abstain`

Allowed confidence values:

- `high`
- `medium`
- `low`

### P0 Validation

Reject or abstain on:

- invalid JSON
- unknown fields when strict schema is enabled
- missing source item id
- invented source item id
- duplicate source item id
- row for fixed deterministic source item id
- unsupported meal code
- unsupported unit
- non-positive quantity
- empty food phrase
- action/result mismatch, such as `abstain` with populated row fields

For `needs_clarification`, preserve a user-facing message but do not use it for
nutrition math.

### P0 Merge

Merge rules:

1. Start from deterministic parsed rows.
2. Add only validated `propose_row` items.
3. Preserve unresolved rows for `needs_clarification` and `abstain`.
4. Rebuild canonical `checker.Plan` deterministically from merged rows.
5. Run the same plan validation used by deterministic normalization.
6. Record method `deterministic_with_llm_assist` only if at least one accepted
   assist row is merged.

Artifacts:

- `optional/normalization-result.json`
  - `assist_used`
  - `assist_chunks`
  - `assist_requests`
  - `assist_responses`
  - `accepted_assist_rows`
  - `rejected_assist_rows`
  - `review_flags`
- `optional/llm-output.json` remains provider raw output when assist is used.

### P0 Eval

Extend `mealcheck eval-normalization` with an exploratory assist path:

```text
mealcheck eval-normalization -mode assist-local-llama -local-model-repeats 3
```

Metrics:

- `assist_eligible_cases`
- `assist_attempted_cases`
- `assist_success_cases_run`
- `assist_success_cases_pass`
- `assist_rows_attempted`
- `assist_rows_accepted`
- `assist_rows_rejected`
- `assist_abstentions`
- `assist_clarifications`
- `assist_schema_failures`
- `assist_false_accepts`
- `assist_unstable_cases`
- `assist_repeat_summary`
- `assist_case_repeat_summary`

Latency metrics remain planned; they are not emitted yet.

Promotion gate:

- deterministic strict gate remains release-blocking
- assist starts exploratory only
- zero false accepts on reviewed strict cases
- no deterministic row changed by assist
- exact source item preservation after merge
- acceptable repeat stability over at least three repeats

### P0 Implementation Slices

Completed:

1. Define P0 assist request/response structs and JSON schema.
2. Add strict decoder and validation tests.
3. Add merge logic from deterministic rows plus accepted assist rows.
4. Add `AssistProviderAdapter` around the hosted provider interface.
5. Add eval-only P0 assist mode; hosted production assist remains disabled.
6. Add raw request/response capture to `normalization-result.json`.

Remaining:

1. Run repeat eval on seed corpus and natural rewrites.
2. Add latency metrics to assist eval output.
3. Add a config flag for hosted exploratory use only after eval is stable.

## P1 Candidate Assist

### Product Goal

P1 assist should help resolve ordinary foods when deterministic matching
already produced plausible candidates but cannot safely choose one. It should
increase useful coverage without turning MealCheck into fuzzy food search.

### Eligible Inputs

P1 assist may run only when:

- P0 produced canonical MealCheck JSON
- the resolver has an unresolved item with food identity ambiguity
- deterministic candidate generation produced a bounded candidate list
- each candidate has a stable id and validated nutrient data

P1 assist must not run when:

- no candidates exist
- the unresolved reason is missing quantity, unsupported unit, or missing
  conversion
- the item is branded, medical, supplement-like, non-food, or outside resolver
  policy
- deterministic gates marked the candidate set as unsafe

### Candidate Generation

Before calling the model, deterministic code should build the candidate list:

- exact local catalog candidates
- alias candidates
- FNDDS fallback candidates whose `resolver_status` is `auto`
- safe approximate proxies, if already policy-approved
- decomposition candidates, if already policy-approved

Candidate list cap:

- default max 5 candidates
- stable deterministic ordering
- include only candidates that can support the input unit or conversion

### P1 Assist Request

```json
{
  "task": "p1_candidate_assist",
  "source_item_id": 7,
  "food": "turkey meatballs",
  "quantity": 5,
  "unit": "oz",
  "day": 2,
  "meal": "dinner",
  "source_text": "5 oz turkey meatballs",
  "candidates": [
    {
      "candidate_id": "fndds:123456",
      "name": "Meatballs, turkey",
      "aliases": ["turkey meatballs"],
      "category": "meat mixed dish",
      "supported_units": ["g", "oz"],
      "resolution_method_if_selected": "exact",
      "nutrient_summary": {
        "kcal_per_100g": 180,
        "protein_g_per_100g": 20,
        "sodium_mg_per_100g": 450
      }
    }
  ]
}
```

### P1 Assist Response

```json
{
  "action": "select_candidate",
  "candidate_id": "fndds:123456",
  "confidence": "medium",
  "reason": "The candidate name and alias match the source phrase."
}
```

Allowed actions:

- `select_candidate`
- `ambiguous`
- `no_safe_match`

### P1 Validation

Reject or abstain on:

- invalid JSON
- missing action
- selected candidate id not in the provided candidate list
- selected candidate fails deterministic resolver gate
- selected candidate lacks unit support or conversion
- selected candidate violates ambiguity, brand, or quarantine policy
- unsupported confidence value

The model must not return nutrient values. Nutrients always come from the
selected candidate record.

### P1 Merge

If a candidate is accepted:

- resolve the item through the existing deterministic resolver path
- set `resolution_method` to the candidate's deterministic method plus an
  assist marker, for example `llm_selected_exact`
- store `assist_candidate_id`, `assist_confidence`, and `assist_reason`
- include a report-visible review flag

If the model returns `ambiguous` or `no_safe_match`, keep the item unresolved
with a specific reason.

### P1 Eval

Do not use P1 assist in the strict resolver gate until exploratory eval is
stable.

Add an exploratory candidate-assist eval mode to `mealcheck eval-checker`:

```text
mealcheck eval-checker -candidate-assist local-llama -candidate-assist-repeats 3
```

Metrics:

- `candidate_assist_eligible_items`
- `candidate_assist_attempted_items`
- `candidate_assist_selected_items`
- `candidate_assist_correct_selections`
- `candidate_assist_false_selections`
- `candidate_assist_abstentions`
- `candidate_assist_ambiguous`
- `candidate_assist_no_safe_match`
- `candidate_assist_schema_failures`
- `candidate_assist_repeat_unstable_items`

Promotion gate:

- zero expected-outcome mismatches in strict P1 gate
- zero false selections on reviewed cases
- high abstention is acceptable
- accepted selections must improve coverage on common unresolved foods
- every selected id must be replayable from artifacts

### P1 Implementation Slices

Completed:

1. Add P1 assist request/response structs and JSON schema.
2. Add strict validation and abstention handling for known candidates,
   invented candidate ids, ambiguity, and no-safe-match responses.

Remaining:

1. Add deterministic candidate list export for unresolved resolver items.
2. Add exploratory `mealcheck eval-checker` candidate-assist mode.
3. Record candidate-assist artifacts in resolver output.
4. Add report labels for assisted resolution.
5. Run repeat eval on FNDDS-grounded and WWEIA/NHANES coverage corpora.
6. Consider production opt-in only after false selections remain zero.

## Rollout Order

1. Build P0 assist contract, validation, merge, and eval mode.
2. Run P0 assist repeat eval on the seed corpus and natural rewrites.
3. Enable P0 assist only for exploratory hosted runs.
4. Promote narrow P0 assist classes if false accepts are zero.
5. Build deterministic P1 candidate export.
6. Build P1 candidate-assist eval mode.
7. Run P1 repeat eval on reviewed resolver datasets.
8. Add report visibility for assisted rows and selections.
9. Consider production opt-in after both P0 and P1 exploratory metrics are
   stable.

## Non-Goals

- Do not use the LLM to compute nutrients.
- Do not use the LLM to choose foods outside a provided candidate list.
- Do not use P1 assist to compensate for P0 normalization failures.
- Do not silently expand the public acceptable-input boundary.
- Do not enable production assist without repeat eval and artifact review.

# Implementation Plan

MealCheck should start with a seeded, deterministic proof before live model calls
or hosted service code.

## MVP Definition

The MVP is a public, inspectable meal-plan verification demo with a constrained
live BYOK path.

The MVP should include:

- Cloudflare Pages frontend.
- MacBook-hosted backend through Cloudflare Tunnel.
- Seeded public demo that requires no credentials and no live model calls.
- One healthy-adult meal-plan scenario.
- Versioned guideline pack snapshot.
- Local fixture nutrient catalog sufficient for the seeded scenario.
- Strict normalized meal-plan schema.
- Three input modes: manual structured entry, profile-only generation, and
  prompt-based generation.
- Deterministic checks for structure, allergens, nutrient limits, unresolved
  foods, and baseline-versus-candidate regression.
- Human-readable report and machine-readable artifacts.
- BYOK path behind an access gate for optional generation and bounded JSON
  repair.
- One worker and explicit resource limits.

The MVP excludes:

- medical diet recommendations
- disease-specific guidance
- broad FoodData Central search as a required live dependency
- local model serving
- anonymous maintainer-paid inference
- multi-user collaboration
- account history dashboards
- grocery price optimization
- mobile app packaging

The MVP is complete when a reviewer can inspect a seeded report and understand:

- what meal plan was checked
- what constraints were declared
- which guideline pack was used
- which foods were resolved or unresolved
- which checks passed, warned, or blocked
- why the final decision was reached

## Local CLI User Story

The first user story is a reviewer pulling the MealCheck repository and running a
seeded validation locally without model API keys.

Expected future flow:

```bash
go run ./cmd/mealcheck validate \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out artifacts/latest
go run ./cmd/mealcheck decision artifacts/latest/decision.json
```

The seeded example should:

- use a fixture guideline pack
- use a fixture nutrient catalog
- include at least one baseline plan and one candidate plan
- generate a complete artifact bundle
- not require network access
- not require provider credentials

Acceptance criteria:

- fresh checkout can run the example from documented commands
- output is deterministic
- report links failures to plan evidence and source-pack references
- artifact shape matches `docs/contracts.md`
- the example is understandable without maintainer explanation

## Pre-Build Decisions

### 1. Build Order

Build in this order:

1. Contracts and seeded fixtures.
2. Guideline-pack fixture and schema.
3. Nutrient catalog fixture and resolver.
4. Deterministic checker engine.
5. Local CLI and artifact bundle.
6. Human-readable reports.
7. Static frontend with seeded artifacts.
8. Hosted API and worker.
9. BYOK profile-only generation, prompt-based generation, and bounded repair.
10. Live nutrient lookup, if still needed.

This order proves the hard part first: evidence-backed evaluation.

### 2. Implementation Language And Framework

Use Go for the first implementation.

Initial Go scope:

- checker engine
- local CLI
- hosted API
- worker
- cleanup job
- artifact writer

Use JSON Schema contracts for the external artifact shapes. The Go code can use
generated or hand-written types, but the JSON contracts remain the cross-surface
source of truth.

Python may be used later for offline preprocessing helpers if that becomes the
fastest way to prepare guideline or nutrient source data. It is not the runtime
default and should not be presented as a product differentiator.

### 3. Case Contract

Use one JSON file per case for MVP. JSONL can wait until batch runs matter.

The case should name input mode, profile, constraints, guideline pack, plan
paths or generation prompt, and expected check policy.

### 4. Meal Plan Contract

Require normalized JSON before evaluation.

The checker should not evaluate arbitrary prose directly. The three MVP input
modes are:

- manual structured entry
- profile-only LLM generation
- prompt-based LLM generation

All three modes must produce the same normalized meal-plan JSON. The normalized
plan is the auditable artifact.

### 5. Guideline Pack Contract

Create one initial pack:

`dga-2025-2030-us-adult-general-v1`

Initial scope:

- healthy adult general-use checks
- sodium limit
- added sugar limit
- saturated fat limit
- calorie target tolerance
- declared allergen exclusion
- declared food exclusion
- meal-prep safety reminders
- source citations and disclaimer text

The first pack should not cover:

- pediatric guidance
- pregnancy
- diabetes
- hypertension
- kidney disease
- allergies beyond declared ingredient exclusion checks
- sports nutrition

The selected source set and preprocessing pipeline are documented in
`docs/nutritional-guidelines.md`.

### 6. Nutrient Catalog Strategy

Start with local fixture data.

MVP fixture scope:

- the foods needed by the seeded fixture
- reviewed aliases for those foods
- per-food conversions for supported household units
- canonical gram quantities internally

The first fixture catalog has 17 foods. That is sufficient for Milestone 0
because it covers the seeded baseline, candidate, allergen, high-sodium,
food-group, unit-conversion, and unresolved-quantity paths. A broader 30 to 60
food catalog can be added later if the public demo needs a more credible manual
entry surface.

Supported MVP units:

- `g`
- `oz`
- `cup`
- `tbsp`
- `tsp`
- `serving`

Unit conversion is allowed only when the fixture defines the conversion for that
food. Missing conversions remain unresolved rather than guessed.

Food resolution should use exact matches plus reviewed aliases. Fuzzy matching is
post-MVP.

Reason:

- stable tests
- no network dependency
- no external API key
- predictable MacBook resource use
- clear seeded demo behavior

Later, add FoodData Central lookup behind a cache and rate limit. The MVP uses
fixture nutrient data so seeded demos and tests do not require network access or
a FoodData Central API key.

### 7. Deterministic Check Set

Initial checks:

- `meal_plan_schema_valid`
- `required_meals_present`
- `quantities_resolvable`
- `allergens_absent`
- `excluded_foods_absent`
- `calories_within_tolerance`
- `sodium_under_limit`
- `added_sugar_under_limit`
- `saturated_fat_under_limit`
- `protein_minimum_met`
- `food_group_coverage`
- `shopping_list_consistent`
- `prep_safety_mentions_present`
- `baseline_candidate_regression`

Decision policy:

- block when a declared allergy or forbidden food appears
- block when required structure is missing
- block when a nutrition-critical food, quantity, or unit cannot be resolved
- block when candidate newly violates a configured hard limit
- warn when optional foods or shopping-list items are unresolved but not enough
  to invalidate the whole plan
- warn when sodium exceeds 2,300 mg/day
- warn when saturated fat exceeds 10 percent of calories
- warn when a meal exceeds 10 g added sugar
- warn when calories are outside the configured target tolerance
- warn when protein is below a user-configured minimum
- warn when food-group or prep-safety checks are incomplete
- pass when no blocking violation or material regression is detected

Protein checks are `not_applicable` when no protein minimum is configured.

Nutrient thresholds are warnings by default unless the case or user marks a
threshold as hard.

### 8. Provider Scope

Support two provider modes first:

- `none`: validation of manually entered or fixture normalized plans
- `openai_compatible`: optional BYOK profile-generation, prompt-generation, and
  bounded JSON repair

No provider is needed for the seeded public demo.

The LLM should not be used as the source of truth for nutrition compliance or
missing nutrition-critical plan details.

### 9. Hosted Access Gate

Public access:

- seeded demo reports
- safe artifacts
- no live model calls

Live BYOK access:

- require invite token
- apply upload, runtime, and queue limits
- discard provider credentials after each run

Admin access:

- separate admin token or private route
- view queue state
- delete runs
- trigger cleanup

### 10. Database Schema

Initial hosted tables:

- `runs`: run metadata, status, visibility, decision, timestamps, expiry.
- `job_queue`: queued jobs, attempts, lease owner, lease expiry, error summary.
- `run_events`: append-only progress events for SSE replay.
- `artifact_files`: file metadata, paths, sizes, content types.
- `invite_tokens`: optional token hashes and status if env-var token is not enough.

Do not store model provider API keys.

Apply the privacy and retention defaults in `docs/privacy-and-safety.md`. If a
field is not needed for queueing, filtering, deletion, or operational status, do
not persist it in normalized database columns.

### 11. Resource Limits

Initial hosted defaults:

- one active run
- queue size 3
- max 20 cases per run
- max 3 days per meal plan in the public seeded demo
- max 7 days per live run
- max 10 minutes per run
- 7-day artifact retention
- no local LLM inference

This fits the reset 2019 MacBook Air because the expensive work is remote BYOK
generation or repair, and local work is bounded parsing, lookup, arithmetic,
and report generation.

### 12. Frontend Layout

Keep the frontend in the same repo under `ui/`.

The frontend should show:

- public seeded demo list
- selected report
- check summary
- daily nutrition totals
- unresolved foods
- source-pack citations
- manual structured entry form
- profile-only generation form
- prompt-based generation form
- optional create-run form for invite-token BYOK users
- backend status

The purpose of the project is not to prove frontend complexity.

### 13. API Details

Initial endpoints:

- `GET /api/health`
- `GET /api/demo-runs`
- `POST /api/runs`
- `GET /api/runs/{id}`
- `GET /api/runs/{id}/events`
- `GET /api/runs/{id}/report`
- `GET /api/runs/{id}/artifacts`
- `GET /api/runs/{id}/artifacts/{artifact_path}`
- `DELETE /api/runs/{id}`

SSE event types:

- `queued`
- `started`
- `plan_normalized`
- `food_resolved`
- `check_completed`
- `artifact_written`
- `completed`
- `failed`

Error shape:

```json
{
  "error": {
    "code": "invalid_meal_plan",
    "message": "Candidate plan is missing item quantities.",
    "request_id": "req_123",
    "details": {}
  }
}
```

## Milestone 0: Contracts And Fixtures

Status: Complete

Deliver:

- JSON schemas for case, meal plan, source registry, guideline pack, nutrient
  catalog, decision, and report.
- Source registry and preprocessing notes matching
  `docs/nutritional-guidelines.md`.
- Seeded case for healthy adult meal plan.
- Baseline and candidate plan fixtures.
- Fixture guideline pack.
- Fixture nutrient catalog.

Acceptance:

- fixtures validate against schemas
- seeded candidate includes at least one block-worthy failure
- expected report evidence can be described without implementation
- repeatable fixture validation runs through
  `go run ./cmd/mealcheck-fixture-check`

Current status:

- schemas exist in `schemas/`
- source registry and guideline pack exist in
  `data/guidelines/dga-2025-2030-us-adult-general-v1/`
- fixture nutrient catalog exists in `data/nutrients/`
- seeded baseline, candidate, case, expected decision, and expected evidence
  exist in
  `examples/seeded-3-day-peanut-allergy/`
- the fixture catalog is intentionally scoped to the seeded case for Milestone 0
- artifact filenames are fixed in `docs/contracts.md`
- a native Go fixture validator exists under `cmd/mealcheck-fixture-check`

## Milestone 1: Resolver And Checks

Status: Complete for the seeded proof case.

Deliver:

- food normalization
- fixture nutrient lookup
- unit normalization for fixture units
- daily nutrition totals
- deterministic checks
- decision aggregation

Acceptance:

- seeded case produces expected `pass`, `warn`, or `block`
- unresolved foods are visible
- LLM-supplied nutrient totals are ignored or flagged

Current status:

- checker core exists in `internal/checker/`
- seeded case loading, food normalization, exact alias matching, unit
  normalization, nutrient lookup, daily totals, and meal totals are implemented
- deterministic checks cover meal structure, unresolved quantities, allergens,
  user-excluded foods, calories, sodium, added sugar, saturated fat, protein,
  vegetable coverage, and prep-safety notes
- decision aggregation produces a `pass`, `warn`, or `block` result
- tests verify the seeded case blocks as expected and reject LLM-supplied
  nutrition totals
- serving-count and detailed food-safety numeric rules are encoded in the
  guideline pack, but remain post-seeded-case checker expansion work

## Milestone 2: CLI And Artifacts

Deliver:

- `mealcheck validate`
- `mealcheck compare`
- `mealcheck decision`
- artifact bundle
- Markdown report

Acceptance:

- seeded example runs with no network access
- artifact bundle matches contract
- CLI exit codes match decision policy

## Milestone 3: Public Seeded Demo

Deliver:

- static frontend under `ui/`
- seeded report view
- check details and source references
- backend health state

Acceptance:

- frontend can be deployed as static files
- seeded report remains inspectable if backend is offline
- no secrets or live provider calls are required

## Milestone 4: Hosted Wrapper

Deliver:

- hosted API
- one worker
- Postgres-backed run metadata and queue
- filesystem artifact storage
- cleanup job
- Cloudflare Tunnel-compatible local binding

Acceptance:

- backend can serve seeded reports
- one live BYOK run can be queued and completed
- limits are enforced in code
- artifacts expire according to retention policy

## Milestone 5: BYOK Generation And Repair

Deliver:

- invite-token access gate
- OpenAI-compatible provider support
- profile-only generate-plan flow
- prompt-based generate-plan flow
- bounded JSON repair flow
- secret redaction

Acceptance:

- public users cannot trigger maintainer-paid inference
- user keys do not appear in logs, database records, reports, or artifacts
- deterministic evaluation remains separate from LLM explanation
- repair never invents missing quantities, units, or nutrition-critical details

## First Scenario

Use a three-day plan for a healthy adult with:

- 2,000 kcal/day target
- peanut allergy
- shellfish excluded
- sodium max 2,300 mg/day
- added sugar max 10 g/meal
- saturated fat max 10 percent of calories

Seeded candidate failures:

- includes peanut sauce in one meal
- exceeds sodium on one day
- includes at least one vague quantity
- removes a required meal-prep safety note compared with baseline

## Remaining Decisions

These decisions remain after Milestone 0:

- frontend package/build tool and Cloudflare Pages settings
- database migration tool
- final runtime data and artifact paths on the MacBook server
- whether the public demo needs the nutrient catalog expanded beyond the seeded
  fixture set

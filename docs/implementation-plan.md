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
- Deterministic checks for structure, allergens, nutrient limits, unresolved
  foods, and baseline-versus-candidate regression.
- Human-readable report and machine-readable artifacts.
- BYOK path behind an access gate for optional generation or parsing.
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
uv sync
uv run mealcheck validate \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out artifacts/latest
uv run mealcheck decision artifacts/latest/decision.json
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
9. BYOK generation or parsing.
10. Live nutrient lookup, if still needed.

This order proves the hard part first: evidence-backed evaluation.

### 2. Case Contract

Use one JSON file per case for MVP. JSONL can wait until batch runs matter.

The case should name profile, constraints, guideline pack, plan paths, and
expected check policy.

### 3. Meal Plan Contract

Require normalized JSON before evaluation.

The checker should not evaluate arbitrary prose directly. Freeform meal plans
must be normalized by:

- user editing
- deterministic parser for simple formats
- optional BYOK parser LLM

The normalized plan is the auditable artifact.

### 4. Guideline Pack Contract

Create one initial pack:

`us-adult-general-v1`

Initial scope:

- healthy adult general-use checks
- sodium limit
- added sugar limit
- saturated fat limit
- calorie target tolerance
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

### 5. Nutrient Catalog Strategy

Start with local fixture data.

Reason:

- stable tests
- no network dependency
- no external API key
- predictable MacBook resource use
- clear seeded demo behavior

Later, add FoodData Central lookup behind a cache and rate limit.

### 6. Deterministic Check Set

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
- block when candidate newly violates a configured hard limit
- warn when foods are unresolved but not enough to invalidate the whole plan
- warn when food-group or prep-safety checks are incomplete
- pass when no blocking violation or material regression is detected

### 7. Provider Scope

Support two provider modes first:

- `none`: validation of supplied normalized plans
- `openai_compatible`: optional BYOK generation or parsing

No provider is needed for the seeded public demo.

The LLM should not be used as the source of truth for nutrition compliance.

### 8. Hosted Access Gate

Public access:

- seeded demo reports
- safe artifacts
- no live model calls

Live BYOK access:

- require invite token or simple account gate
- apply upload, runtime, and queue limits
- discard provider credentials after each run

Admin access:

- separate admin token or private route
- view queue state
- delete runs
- trigger cleanup

### 9. Database Schema

Initial hosted tables:

- `runs`: run metadata, status, visibility, decision, timestamps, expiry.
- `job_queue`: queued jobs, attempts, lease owner, lease expiry, error summary.
- `run_events`: append-only progress events for SSE replay.
- `artifact_files`: file metadata, paths, sizes, content types.
- `invite_tokens`: optional token hashes and status if env-var token is not enough.

Do not store model provider API keys.

Avoid storing detailed profile data longer than required for the report. If a
field is not needed after artifact creation, do not persist it in normalized
database columns.

### 10. Resource Limits

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
generation or parsing, and local work is bounded parsing, lookup, arithmetic,
and report generation.

### 11. Frontend Layout

Keep the frontend in the same repo under `ui/`.

The frontend should show:

- public seeded demo list
- selected report
- check summary
- daily nutrition totals
- unresolved foods
- source-pack citations
- optional create-run form for invite-token BYOK users
- backend status

The purpose of the project is not to prove frontend complexity.

### 12. API Details

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

Deliver:

- JSON schemas for case, meal plan, guideline pack, decision, and report.
- Seeded case for healthy adult meal plan.
- Baseline and candidate plan fixtures.
- Fixture guideline pack.
- Fixture nutrient catalog.

Acceptance:

- fixtures validate against schemas
- seeded candidate includes at least one block-worthy failure
- expected report evidence can be described without implementation

## Milestone 1: Resolver And Checks

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

## Milestone 5: BYOK Generation Or Parsing

Deliver:

- invite-token access gate
- OpenAI-compatible provider support
- optional generate-plan flow
- optional parse-plan flow
- secret redaction

Acceptance:

- public users cannot trigger maintainer-paid inference
- user keys do not appear in logs, database records, reports, or artifacts
- deterministic evaluation remains separate from LLM explanation

## First Scenario

Use a three-day plan for a healthy adult with:

- 2,000 kcal/day target
- peanut allergy
- shellfish excluded
- sodium max 2,300 mg/day
- added sugar max 10 percent of calories
- saturated fat max 10 percent of calories

Seeded candidate failures:

- includes peanut sauce in one meal
- exceeds sodium on one day
- includes at least one vague quantity
- removes a required meal-prep safety note compared with baseline

## Remaining Decisions

- Choose implementation language and framework.
- Decide whether `us-adult-general-v1` uses FDA Daily Values, DRI-derived
  values, or user-configured limits for each threshold.
- Define allergen taxonomy for MVP.
- Define unit conversion scope.
- Decide whether FoodData Central lookup is post-MVP or part of hosted MVP.
- Decide exact invite-token/auth shape.
- Decide final project name if MealCheck is only a working name.

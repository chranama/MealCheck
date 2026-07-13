# Implementation Plan

MealCheck should start with a seeded, deterministic proof before live model calls
or hosted service code.

## MVP Definition

The MVP is a public, inspectable meal-plan verification demo with a constrained
hosted local-model path. BYOK and custom model-provider paths remain available
for repo/API/CLI users and self-hosted deployments.

The MVP should include:

- Cloudflare Pages frontend.
- MacBook-hosted backend through Cloudflare Tunnel.
- Local CLI deployment for reviewers who want to run the seeded proof without
  the hosted backend.
- Seeded proof artifact that requires no credentials and no live model calls.
- One healthy-adult meal-plan scenario.
- Versioned guideline pack snapshot.
- Local fixture nutrient catalog sufficient for the seeded scenario.
- Strict normalized meal-plan schema.
- Hosted surface for local-model verification, with repo/local guidance for
  seeded proof artifacts and advanced use.
- Server-owned local-model normalization as the hosted live path.
- BYOK profile-only generation, prompt-based generation, qualification,
  normalization, and bounded JSON repair as repo/API/CLI and self-hosted
  capabilities.
- Local CLI structured JSON validation for debugging, fixtures, regression
  cases, and agent-generated structured inputs.
- Deterministic checks for structure, allergens, nutrient limits, unresolved
  foods, and baseline-versus-candidate regression.
- Human-readable report and machine-readable artifacts.
- Public hosted local-model path behind request, rate, input-size, queue,
  timeout, and retention policy gates.
- One worker and explicit resource limits.

The MVP excludes:

- medical diet recommendations
- disease-specific guidance
- broad FoodData Central search as a required live dependency
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

The web-deployed MVP is complete only when the above is available as a
long-standing public web deployment:

- Cloudflare Pages serves the frontend from `ui/` at a stable public URL.
- The hosted website opens directly on the local-model verification workflow
  without example-run navigation.
- The frontend can call a public API hostname exposed through Cloudflare Tunnel
  and show backend health.
- The MacBook backend runs under process supervision, restarts after reboot, and
  uses Postgres plus filesystem artifact storage outside the Git checkout.
- The public API exposes only the intended HTTP surface and uses CORS limited to
  the production frontend origin.
- Live local-model runs are public, bounded by the configured request, queue,
  upload, input-text, timeout, and retention limits, and can be deleted by the
  user.
- The runbook documents deployment, start, stop, restart, health check, logs,
  tunnel status, smoke tests, backup, and cleanup commands.
- A smoke test from outside the home network can check backend health, create a
  hosted local-model live run, observe completion, and verify provider config is
  rejected for hosted local-model requests.

The local CLI-deployed MVP is complete when a reviewer can install or build a
local `mealcheck` command and run the seeded proof without network access,
provider keys, the MacBook backend, or Cloudflare:

- the README and runbook document the supported local CLI install/build path
- `mealcheck validate` writes the full artifact bundle for the seeded case
- `mealcheck compare` is documented for the current seeded comparison behavior
- `mealcheck decision` applies the documented exit-code policy to an existing
  `decision.json`
- the CLI deployment has a smoke test that starts from a fresh checkout or
  clean build directory
- local CLI artifacts match the shared contract used by the hosted backend and
  frontend

## Local CLI User Story

The first user story is a reviewer pulling the MealCheck repository and running
a seeded validation locally without model API keys.

Current flow:

```bash
go run ./cmd/mealcheck validate \
  --case examples/seeded-one-day-peanut-allergy/case.json \
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

Support five provider modes first:

- `none`: validation of manually entered or fixture normalized plans
- `openai`: optional BYOK profile-generation, prompt-generation, and bounded
  JSON repair through OpenAI's native endpoint
- `anthropic`: optional BYOK profile-generation, prompt-generation, and bounded
  JSON repair through Anthropic's native endpoint
- `gemini`: optional BYOK profile-generation, prompt-generation, and bounded
  JSON repair through Gemini's native endpoint
- `openai_compatible`: optional BYOK profile-generation, prompt-generation, and
  bounded JSON repair through a custom OpenAI-compatible endpoint

No provider is needed for the seeded public demo.

The LLM should not be used as the source of truth for nutrition compliance or
missing nutrition-critical plan details.

### 9. Hosted Access Gate

Public access:

- seeded demo reports
- safe artifacts
- no live model calls

Live BYOK access:

- require per-user access code
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
- optional create-run form for access-code-gated BYOK users
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
  `go run ./cmd/mealcheck fixture-check`

Current status:

- schemas exist in `schemas/`
- source registry and guideline pack exist in
  `data/guidelines/dga-2025-2030-us-adult-general-v1/`
- fixture nutrient catalog exists in `data/nutrients/`
- seeded baseline, candidate, case, expected decision, and expected evidence
  exist in
  `examples/seeded-one-day-peanut-allergy/`
- the fixture catalog is intentionally scoped to the seeded case for Milestone 0
- artifact filenames are fixed in `docs/contracts.md`
- a native Go fixture validator is available as `mealcheck fixture-check`

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

Status: Complete for the seeded proof case.

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

Current status:

- `cmd/mealcheck` implements `validate`, `compare`, and `decision`
- `validate` and `compare` write the shared artifact bundle through
  `internal/artifacts`
- `decision` reads `decision.json` and applies the same exit-code policy
- the seeded example writes Markdown, HTML, JSON, JSONL, source-pack, config,
  and schema artifacts
- tests verify the seeded block exit, compare manifest mode, decision command
  exit behavior, invalid CLI usage, and required artifact files
- `compare` currently shares the seeded validation path and records its mode;
  richer baseline/candidate regression reporting remains future checker work

## Milestone 3: Public Seeded Demo

Status: Complete for the seeded proof case.

Deliver:

- static frontend under `ui/`
- seeded report view
- check details and source references
- backend health state

Acceptance:

- frontend can be deployed as static files
- seeded report remains inspectable from the repository if backend is offline
- no secrets or live provider calls are required

Current status:

- Milestone 3 originally shipped as a no-build static frontend and is now
  superseded by the Milestone 6 Vite/React frontend and the Milestone 29
  hosted example removal
- seeded artifact bundle exists under
  `examples/seeded-one-day-peanut-allergy/artifacts/demo-runs/`
- `docs/seeded-report.html` renders the seeded decision, check details,
  nutrition totals, unresolved foods, and disclaimer as a standalone repo
  artifact
- backend health state is shown for the live workflow and can call
  `/api/health` when an API base URL is configured
- local preview now uses the Vite development server
- no frontend secrets, model calls, or backend dependency are required for the
  repository seeded report

## Milestone 4: Hosted Wrapper

Status: Complete for the first hosted proof.

Deliver:

- hosted API
- one worker
- Postgres-backed run metadata and queue
- filesystem artifact storage
- cleanup job
- Cloudflare Tunnel-compatible local binding

Acceptance:

- backend can serve seeded reports
- one hosted validation run can be queued and completed
- limits are enforced in code
- artifacts expire according to retention policy

Current status:

- `cmd/mealcheck-server` runs the hosted API, one worker, and cleanup loop
- API binds to `127.0.0.1:8080` by default for Cloudflare Tunnel compatibility
- Postgres-backed run metadata and queue storage are implemented through
  `DATABASE_URL`
- tests use the same store contract with an in-memory implementation
- filesystem artifact storage writes under `.mealcheck-data/artifacts/` by
  default
- endpoints cover health, demo runs, run creation, run status, SSE events,
  reports, artifact listing, artifact download, and run deletion
- queue size, upload size, run timeout, and retention are enforced in code
- cleanup deletes expired run artifacts and marks expired runs deleted
- Milestone 4 run creation accepted checked-in case paths; LLM BYOK generation
  and repair were assigned to Milestone 5

## Milestone 5: BYOK Generation And Repair

Status: Implemented in the hosted backend.

Deliver:

- per-user access-code gate
- OpenAI-compatible provider support
- profile-only generate-plan flow
- prompt-based generate-plan flow
- bounded JSON repair flow
- secret redaction

Implemented shape:

- `POST /api/runs` accepts `manual_structured`, `profile_generation`, and
  `prompt_generation` request bodies in addition to checked-in `case_path`
  demo runs.
- Generation modes require a BYOK provider with `model` and `api_key`.
  Supported provider types are `openai`, `anthropic`, `gemini`, and
  `openai_compatible`.
- BYOK keys are stored only in a shared in-memory pending map until the worker
  claims the run.
- Generated or manually submitted plans are written as runtime case files under
  the server data directory and then evaluated by the existing deterministic
  checker.
- Provider output and normalization events are optional artifacts; provider
  config is persisted only as `configs/redacted-provider.json`.
- One bounded JSON repair attempt is allowed by default for generation modes.

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

These decisions remain after Milestone 5:

- final runtime data and artifact paths on the MacBook server
- whether the public demo needs the nutrient catalog expanded beyond the seeded
  fixture set
- whether the first public live frontend includes all three input modes or only
  a narrower invite-gated path
- final production domain names for the Pages frontend and tunneled API
- process supervision shape on macOS, likely `launchd`
- backup policy for Postgres metadata and retained artifacts

## MVP Gap Assessment After Milestone 5

Milestones 0 through 5 prove the core product and hosted backend behavior, but
they do not finish MVP web acceptance. The remaining gaps are:

- The frontend is still a seeded report viewer. It does not yet let an
  invite-gated user create, monitor, view, and delete live runs.
- BYOK disclosure and provider-key handling need to exist in the web surface,
  not only in backend tests and docs.
- The MacBook deployment is not yet packaged as a supervised long-running
  service with final runtime paths, logs, environment files, and restart
  behavior.
- Cloudflare Pages and Cloudflare Tunnel are documented as the intended shape,
  but the production project, hostnames, CORS origin, and external smoke tests
  are not recorded.
- Operational commands for deploy, start, stop, restart, logs, health checks,
  cleanup, backup, and live-run deletion still need exact MacBook-specific
  instructions.
- The current fixture nutrient catalog is enough for the seeded proof, but the
  public live-run UI may need either a narrow food list or a small catalog
  expansion to keep manual entry credible.
- The local CLI exists and is tested through `go run`, but MVP acceptance now
  requires treating it as a local deployment surface: documented build/install
  commands, a stable binary path or install command, a clean-checkout smoke
  test, and explicit confirmation that CLI artifacts match the hosted artifact
  contract.

Local CLI status:

- Done: `validate`, `compare`, and `decision` commands exist under
  `cmd/mealcheck`.
- Done: seeded CLI runs write the shared artifact bundle.
- Done: tests cover seeded block exit behavior, compare manifest mode, decision
  command exit behavior, invalid usage, and required artifact files.
- Remaining: document a supported local CLI build/install path, such as
  `go build -o bin/mealcheck ./cmd/mealcheck` or `go install`.
- Remaining: add a clean local CLI smoke-test procedure to the runbook.
- Remaining: decide whether MVP acceptance needs a checked release binary or
  whether source build from a fresh checkout is sufficient.
- Remaining: update README status once local CLI deployment is accepted.

## Milestone 6: Local Vite/React Live Frontend Prototype

Status: Implemented and locally accepted. Deployment-server and public-hosting
validation remain in later milestones.

Deliver:

- small Vite/React frontend under `ui/` that still builds to static files for
  Cloudflare Pages
- React component structure for the app shell, seeded report viewer, live-run
  workflow, run status, report tabs, and artifact list
- configurable API base URL, including `localhost` during development and a
  public API origin in deployed static output
- seeded artifacts served from the Vite public directory so the public demo
  works without backend uptime
- backend health display from the configured API base URL
- access code entry that is kept out of committed config and frontend build
  output
- BYOK third-party disclosure before generation or repair runs
- manual structured meal-plan entry for the local MVP food/unit scope
- profile-only generation form
- prompt-based generation form
- run creation against `POST /api/runs`
- run progress through SSE or polling
- live report rendering from the run artifact endpoints
- artifact listing and download links
- live-run deletion control
- visible backend-offline state that still leaves the seeded report usable
- no frontend server, server-side rendering, serverless function, or new hosted
  runtime in the production shape
- TypeScript conversion for the frontend, with strict UI contracts for reports,
  runs, artifacts, forms, and API payloads
- LLMEP-derived frontend architecture:
  - `src/App.tsx` as the app shell
  - `src/types.ts` for UI-facing domain and API contracts
  - `src/lib/api.ts` for API base handling, request wrappers, typed endpoint
    functions, and consistent error formatting
  - `src/lib/runtime_config.ts` for public runtime config loaded from
    `/config.json`, with `?api=` kept as the local override path
  - `src/components/shell/` for the app frame, backend status, seeded report
    selection, and summary bands
  - `src/components/common/` for shared form and metric controls
  - `src/components/live-run/` for profile, constraints, input mode, BYOK, run
    status, and deletion controls
  - `src/components/report/` for summary, tabs, checks, nutrition, foods,
    sources, and artifact rendering
  - `src/test/factories/` for reusable report/run/API test fixtures
- frontend tests modeled after LLMEP:
  - Vitest tests for runtime config, API URL joining, full-body JSON parsing,
    error formatting, payload builders, manual-plan normalization, SSE parsing,
    and live-run mode behavior
  - React Testing Library tests for live-run form behavior and payload
    submission boundaries
  - Playwright e2e tests with mocked backend routes for seeded report loading,
    manual run, BYOK profile run, BYOK prompt run, deletion, and provider-key
    non-persistence
- frontend commands for the hardened UI:
  - `npm run typecheck`
  - `npm test`
  - `npm run test:e2e`
  - `npm run build`

Acceptance:

- acceptance can be run entirely on the development/prototyping computer
- `npm run build` produces static frontend assets only
- `npm run typecheck` passes with strict TypeScript settings
- `npm test` passes the frontend unit/integration suite
- `npm run test:e2e` passes mocked-browser flows without requiring a live Go
  backend or model provider
- a user can inspect the seeded report without a backend
- a user with an access code can create one manual structured run from the
  frontend against a local backend
- a user with an access code and BYOK provider key can create one
  profile-generation run and one prompt-generation run from the frontend
  against a local backend
- the frontend can observe run completion or failure and then render the report
- the frontend can delete a live run and the deleted report/artifacts are no
  longer available
- provider keys are not stored in committed files, frontend build output,
  localStorage, reports, or artifacts
- BYOK runs clearly disclose that profile, constraints, prompt text, and
  generated meal-plan content are sent to the user's selected provider
- seeded report viewing remains usable when the backend is offline

Completed implementation notes:

1. Converted the Vite app to TypeScript:
   - rename `vite.config.js` to `vite.config.ts`
   - rename `src/main.jsx` to `src/main.tsx`
   - add `typescript`, `@types/react`, and `@types/react-dom`
   - add `tsconfig.json` and `tsconfig.app.json` with `strict: true`,
     `noEmit: true`, `moduleResolution: "bundler"`, and `jsx: "react-jsx"`
2. Extracted frontend contracts:
   - define `InputMode`, `RunStatus`, `DemoRun`, `Decision`, `Report`,
     `DailyTotal`, `ResolvedFood`, `UnresolvedFood`, `ArtifactItem`,
     `Profile`, `Constraints`, `ProviderConfig`, and run payload types
   - keep backend JSON Schemas as the source of truth; TypeScript types are a
     UI guardrail, not a replacement for runtime validation
3. Split the previous single React file into feature modules:
   - app shell and backend status
   - seeded demo selector
   - live-run workflow
   - report summary/tabs/panels
   - pure payload and formatting utilities
4. Added runtime config:
   - load optional `/config.json` before rendering
   - use precedence: query-string `?api=`, runtime config, Vite public env,
     meta tag, then static-demo mode
   - allow only public values such as API base URL in runtime/build config
5. Added a central API client:
   - normalize API base URLs
   - join endpoint paths in one place
   - return typed endpoint responses
   - format backend errors with status, code, message, and request ID when
     available
6. Added tests:
   - unit-test pure builders and URL/error helpers
   - component-test live-run mode behavior and payload submission boundaries
   - e2e-test mocked flows for seeded, manual, BYOK profile, BYOK prompt,
     deletion, and secret non-persistence
7. Re-ran local verification with `npm run typecheck`, `npm test`,
   `npm run test:e2e`, `npm run build`, `go test ./...`, and
   `go run ./cmd/mealcheck fixture-check`.

## Milestone 7: Local Full-Stack Validation And Security

Status: Implemented and locally accepted. MacBook service configuration,
Cloudflare Pages/Tunnel setup, and public smoke tests remain in later
milestones.

Deliver:

- local CLI deployment smoke test from a clean checkout or clean build
  directory
- local full-stack smoke test commands for static frontend plus local backend
- local test fixture for invite-gated manual run creation
- local test fixture for BYOK generation using either a fake provider or a
  user-supplied key
- browser-level verification that seeded report viewing works without the
  backend
- browser-level verification that live run creation, progress, report viewing,
  artifact listing, and deletion work against the local backend
- local CORS verification for allowed and disallowed origins
- redaction verification for reports, artifacts, runtime files, and any logs
  produced during local runs
- decision on whether the public live UI stays limited to the seeded catalog or
  expands to a small reviewed catalog
- any local catalog expansion needed to support the first credible manual-entry
  UI
- local-only fake provider response path for deterministic BYOK smoke tests

Acceptance:

- acceptance can be run entirely on the development/prototyping computer
- local CLI deployment can build or install `mealcheck`, run the seeded
  validation, inspect `decision.json`, and verify the expected `block` exit
  policy
- local smoke tests cover seeded report viewing, health, manual run creation,
  BYOK run creation, run events, report rendering, artifact listing, and
  deletion
- CORS behavior can be demonstrated locally before Cloudflare is introduced
- provider keys are absent from committed files, frontend build output,
  localStorage, runtime files, reports, artifacts, and test logs
- the first public manual-entry food scope is decided and documented
- all checks pass without MacBook server configuration or public web hosting

Accepted local commands:

- `go run ./cmd/mealcheck local-smoke`
- `cd ui && npm run test:e2e:local`

Completed implementation notes:

1. Added `mealcheck local-smoke` to build the CLI into a temporary clean
   build directory, run the seeded validation, inspect `decision.json`, verify
   the expected `block` exit policy, exercise invite-gated manual and fake-BYOK
   hosted runs, verify run events/report/artifact listing/deletion, check CORS,
   and scan runtime files/artifacts/test logs for the fake provider key.
2. Added `examples/local-smoke/` fixtures for local manual and BYOK smoke
   payloads without real secrets.
3. Added `MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH` as a local smoke-test-only
   provider response source for `mealcheck-server`.
4. Added `npm run test:e2e:local`, which starts the real Go backend with memory
   storage and the Vite frontend, then verifies seeded viewing, manual live run
   creation/deletion, BYOK fake-provider creation/redaction, and CORS headers.
5. Tightened CORS so configured origins receive CORS headers only when the
   request `Origin` matches `MEALCHECK_ALLOWED_ORIGIN`.
6. Decided that the first public manual-entry UI stays limited to the existing
   17-food seeded fixture catalog. No catalog expansion is needed for Milestone
   7; expansion remains post-local-acceptance work if the public demo needs a
   broader manually entered menu.

## Milestone 8: Deployment Package Prepared Locally

Status: Implemented locally. Milestone 8 prepares templates and commands on the
development/prototyping computer; it intentionally does not configure the
MacBook, Cloudflare Pages, or Cloudflare Tunnel with real values.

Deliver:

- documented local CLI build/install command and binary path
- README and runbook instructions for local CLI deployment
- decision on whether source-build CLI deployment is enough for MVP or whether
  release binaries are needed
- final proposed MacBook runtime user, repository path, data path, artifact
  path, log path, and Postgres database name
- production `.env` template with secret placeholders only
- `launchd` service template or equivalent process-supervision template for
  `mealcheck-server`
- Cloudflare Tunnel configuration template with placeholder tunnel credentials
- Cloudflare Pages settings documented with production hostnames
- Postgres setup commands and verification commands drafted
- backend start command using production-style Postgres storage drafted
- runbook sections for deploy, pull, start, stop, restart, status, logs, local
  health, public health, tunnel status, cleanup, backup, and deletion drafted
- public smoke-test checklist drafted with production URLs
- backup policy drafted for Postgres metadata and retained artifacts
- common failure modes and recovery steps drafted

Implemented:

1. Added deployment package templates:
   - `deploy/README.md`
   - `deploy/macos/mealcheck-server.env.example`
   - `deploy/macos/dev.mealcheck.server.plist.template`
   - `deploy/macos/postgres-setup.sql.template`
   - `deploy/cloudflare/tunnel-config.yml.template`
   - `deploy/cloudflare/pages-settings.md`
   - `deploy/cloudflare/config.json.template`
2. Selected internally consistent deployment values:
   - runtime user: `chranama-server`
   - repository: `/Users/chranama-server/MealCheck`
   - data path: `/Users/chranama-server/MealCheck-data`
   - artifact path: `/Users/chranama-server/MealCheck-data/artifacts`
   - log path: `/Users/chranama-server/MealCheck-data/logs`
   - Postgres database and role: `mealcheck`
   - backend launchd label: `dev.mealcheck.server`
   - Postgres launchd label: `dev.mealcheck.postgres`
   - Cloudflare Tunnel name: `mealcheck-api`
   - production frontend URL: `https://mealcheck.dev`
   - production API URL: `https://api.mealcheck.dev`
3. Decided source-build deployment is enough for MVP:
   - `go build -o bin/mealcheck ./cmd/mealcheck`
   - `go build -o bin/mealcheck-server ./cmd/mealcheck-server`
4. Updated README, runbook, backend server doc, and decision log to reference
   the same deployment package, paths, service label, environment names, and
   production hostnames.
5. Added runbook sections for:
   - local CLI deployment
   - MacBook first-time preparation
   - Postgres setup and verification
   - backend deploy or pull
   - backend `launchd` lifecycle
   - logs and local health
   - Cloudflare Pages and Tunnel draft settings
   - public health
   - deletion and retention
   - backup policy
   - public smoke-test checklist
   - common failure modes and recovery steps

Acceptance:

- acceptance can be completed on the development/prototyping computer without
  configuring the MacBook server or Cloudflare
- local CLI deployment instructions have been run successfully from a clean
  checkout or clean build directory
- README, runbook, and implementation plan describe the same local CLI
  deployment path
- deployment templates contain no real secrets
- all paths, service labels, environment variable names, and production
  hostnames are internally consistent across README, runbook, backend server
  doc, and implementation plan
- the package is ready to copy or apply on the MacBook when deployment starts
- remaining unknowns are explicit placeholders, not hidden assumptions

Milestone 8 verification:

- `go build -o /private/tmp/mealcheck-m8-bin/mealcheck ./cmd/mealcheck`
- `go build -o /private/tmp/mealcheck-m8-bin/mealcheck-server ./cmd/mealcheck-server`
- `/private/tmp/mealcheck-m8-bin/mealcheck help`
- `/private/tmp/mealcheck-m8-bin/mealcheck validate --case
  examples/seeded-one-day-peanut-allergy/case.json --out
  /private/tmp/mealcheck-m8-artifacts/seeded` returned the expected `block`
  policy exit after writing artifacts
- `go test ./...`
- `git diff --check`

## Milestone 9: Web Design Hardening

Status: Extended locally. Milestone 9 hardens the static frontend's visual
design, interaction hierarchy, and live-run user experience without changing the
backend API contract.

Deliver:

- static-web design contract in `docs/web-design.md`
- live-run workflow as the default homepage surface
- live-first navigation with seeded demos retained as secondary proof artifacts
- compact top bar without the separate service-status box
- clearer live-check summary, form grouping, and results hierarchy
- resource-derived UX principles captured in `docs/web-design.md`
- visual identity guidance for MealCheck's mark, color roles, graphic language,
  and product voice captured in `docs/web-design.md`
- distinctive static-safe typesetting guidance for the wordmark, operational UI,
  and audit metadata captured in `docs/web-design.md`
- code-native MealCheck brand mark that works in the static top bar without
  external assets
- evidence/audit visual language applied to source references and generated
  artifact rows
- progressive-disclosure objectives for live-check access, profile,
  constraints, meal-plan entry, and report review
- service availability kept out of standalone client-facing cards while still
  blocking unavailable report creation
- immediate validation and state feedback before report creation
- deliberate confirmation before destructive report deletion
- client-facing results panel that hides run IDs, event counts, pipeline stages,
  and raw artifact links from the default live-check viewport
- live-check access form that relies on configured API state instead of exposing
  a Service URL field
- Report tab that exposes one downloadable report PDF instead of a raw artifact
  browser
- compact first-viewport layout with the primary report action visible before
  the full form
- advanced constraints moved behind progressive disclosure
- decorative navigation, segmented-control, and action glyphs removed from the
  client-facing live-check path
- activity details available behind disclosure after report creation starts
- mobile manual-entry rows rendered as labeled card-like controls
- desktop manual-entry rows constrained to the form panel without overflow
- desktop layout that separates form entry from results
- mobile layout that collapses navigation, form, manual rows, and status panels
  cleanly
- disabled destructive action state when no live run exists
- browser tests updated to assert the live-run homepage and demo navigation
- production static build verified after the hardening pass

Acceptance:

- `docs/web-design.md` codifies the static frontend design direction and
  acceptance rules, including the resource-derived UX principles and visual
  identity rules
- the top bar includes a compact MealCheck brand mark that does not depend on
  image assets
- the wordmark, headings, status labels, chips, and metadata use a distinctive
  static-safe type hierarchy without remote font dependencies
- brand/evidence color is distinct from pass, warn, and block status colors
- source, evidence, and artifact graphics reinforce the audit-console identity
  without decorative imagery
- the homepage opens on the live-run workflow without loading a demo report
  first
- seeded demos remain available from navigation and can still render without a
  backend API base
- live-run form, results status, and report tabs remain functional
- the UI is usable when the service is unavailable or no service is configured
- unavailable or missing service states are communicated through disabled
  actions or error feedback without hiding seeded demos
- invalid report submission is prevented or reported immediately
- report deletion requires an explicit confirmation after a report exists
- the primary report action is available near the top of the workspace on
  desktop and mobile
- advanced threshold and policy constraints are hidden behind an expandable
  section by default
- the default live-check viewport does not expose pipeline graphics, run-event
  counts, raw artifact links, visible Service URL fields, or decorative selector
  symbols
- the top bar does not show a standalone service-status card
- the live-check workspace does not show a standalone service-readiness card
- the Report tab exposes one PDF report download
- manual food entry remains contained on desktop and readable on mobile without
  relying on table headers
- provider-key and access-code handling remains non-persistent
- form labels, focus states, status pills, and disabled states are visible
- desktop and mobile layouts avoid text overlap and preserve readable controls
- typecheck, unit tests, browser tests, local full-stack browser tests, and
  production build pass

Implemented:

1. Added `docs/web-design.md` with the static frontend constraint, visual
   system, page anatomy, component rules, accessibility rules, and Milestone 9
   acceptance checklist.
2. Added guiding UX principles from Laws of UX, Material Design 3, shadcn/ui,
   Mobbin, and the Quibbble static-site reference: live-check priority,
   progressive disclosure, visible system state, outcome-before-evidence
   hierarchy, flow-first design, a compact component system, operational visual
   tone, static-deployment resilience, immediate feedback, and deliberate
   destructive actions.
3. Kept the homepage in live-run mode and retained seeded demos as explicit
   navigation choices.
4. Hardened the live-run UI with a more product-facing visual system, live-first
   sidebar, compact top bar, two-column desktop live workspace, companion
   results panel, manual-entry column headers, clearer disabled action states,
   and responsive form/manual-row behavior.
5. Kept service readiness out of standalone client-facing cards, retained
   immediate submit validation and report-ready status affordances, and added a
   confirmation dialog before deleting a report.
6. Compacting the live-run first viewport with a shared action/status strip,
   progressive disclosure for advanced constraints, simplified results, hidden
   activity details, removed decorative selector/control glyphs, and
   mobile-labeled manual entry rows.
7. Updated mocked and local Playwright flows to assert the live-run homepage
   and seeded-demo navigation path.
8. Added MealCheck visual identity guidance, a code-native brand mark, dedicated
   brand/evidence tokens, and audit-style source and artifact graphics.
9. Reworked the mark into an authored inline SVG seal and applied static-safe
   typesetting across the wordmark, summary titles, status labels, chips, and
   audit metadata.
10. Streamlined the brand seal around the `M` and check mark, switched the UI
    sans stack to IBM Plex Sans with static-safe fallbacks, and applied the
    Scientific Ledger palette across brand, evidence, and neutral UI tokens.
11. Simplified the live-check surface to read less like an internal dashboard:
    renamed access and report actions in client-facing language, removed exposed
    workflow/pipeline graphics from the default view, moved events behind
    Activity details, and constrained manual food entry so it does not exceed
    the form bounds.

Milestone 9 verification:

- `cd ui && npm run typecheck`
- `cd ui && npm test`
- `cd ui && npm run test:e2e`
- local full-stack Playwright spec against the rebuilt frontend and temporary
  memory backend
- `cd ui && npm run build`
- in-app browser verification against `http://127.0.0.1:4173`

## Milestone 10: MacBook Service Configuration

Status: Implemented on the MacBook. The backend is installed as a system
`LaunchDaemon` and verified locally against Postgres-backed storage.

Deliver:

- Go, Postgres, `jq`, and any required server packages installed on the MacBook
- repository checkout under the selected runtime user
- MacBook AC-power profile configured for long-running server use with system
  sleep disabled and verification output recorded
- final runtime data, artifact, and log paths created outside the Git checkout
- Postgres database and user created
- production environment file created with real server values
- production environment file permissions restricted to the runtime user
- `mealcheck-server` running under process supervision
- system `LaunchDaemon` mode chosen and documented for before-login startup
  after unattended reboot
- backend logs written to the documented location
- local MacBook health, seeded run, live run, deletion, and cleanup smoke tests
- backup command tested or dry-run output recorded

Acceptance:

- the backend runs on the MacBook with Postgres metadata storage
- the MacBook does not enter idle system sleep while plugged into AC power
- runtime data and artifact storage are outside the Git checkout
- the service starts after reboot once the documented launchd conditions are
  met, or after manual service restart
- `GET /api/health` works locally against the supervised service
- a local seeded run can be queued, completed, viewed, and deleted on the
  MacBook
- cleanup enforces the 7-day retention policy or has a documented verification
  command
- service logs do not contain provider API keys during tested BYOK runs
- this milestone does not require public Cloudflare Pages or Tunnel routing

Implemented:

1. Verified MacBook backend dependencies and power profile:
   - Go `1.26.4`
   - Postgres `17.10`
   - `jq`, `cloudflared`, Git, GitHub CLI, SSH, Homebrew, and Xcode Command
     Line Tools available
   - AC-power `pmset` profile has `sleep 0`, `disksleep 0`, `standby 0`, and
     `powernap 0`
2. Created runtime paths outside the Git checkout:
   - `/Users/chranama-server/MealCheck-data`
   - `/Users/chranama-server/MealCheck-data/artifacts`
   - `/Users/chranama-server/MealCheck-data/logs`
   - `/Users/chranama-server/MealCheck-data/backups`
3. Created the `mealcheck` Postgres role and database.
   - For true before-login recovery, Postgres should be managed by
     `/Library/LaunchDaemons/dev.mealcheck.postgres.plist`, which starts at
     boot but runs the Postgres process as `chranama-server`.
   - The Postgres daemon sets `LC_ALL=en_US.UTF-8` and `LANG=en_US.UTF-8`
     because Postgres failed under launchd without an explicit valid locale.
4. Created `/Users/chranama-server/MealCheck-data/mealcheck-server.env` with
   real local values and `0600` permissions.
5. Built:
   - `/Users/chranama-server/MealCheck/bin/mealcheck`
   - `/Users/chranama-server/MealCheck/bin/mealcheck-server`
6. Added and installed
   `deploy/macos/dev.mealcheck.server.plist.template` as
   `/Library/LaunchDaemons/dev.mealcheck.server.plist` with `root:wheel`
   ownership and `0644` permissions. The daemon waits for local Postgres to
   accept connections before starting `mealcheck-server`.
7. Removed the temporary user `LaunchAgent` so only the system `LaunchDaemon`
   manages port `8080` after login.
8. Verified `GET /api/health` against the supervised daemon:
   - `status: ok`
   - `store: postgres`
   - `queue_size: 3`
   - `retention_days: 7`
9. Ran local seeded API smoke:
   - queued checked-in seeded case
   - observed completion
   - fetched report and artifacts
   - deleted run and confirmed `404`
10. Ran local manual structured live-run smoke:
   - queued invite-gated manual request
   - observed completion
   - fetched report and artifacts
   - deleted run and confirmed `404`
11. Ran BYOK redaction smoke with a fake sentinel provider key:
    - request failed as expected against a dead local provider URL
    - sentinel key was absent from logs, artifacts, and run metadata
    - deleted run and confirmed `404`
12. Ran retention cleanup verification:
    - completed a run
    - expired it in Postgres
    - ran the production cleanup job against the same data paths
    - confirmed API returned `404` and the artifact directory was removed
13. Ran backup command:
    - wrote a non-empty Postgres dump
    - copied retained artifacts to a timestamped local backup directory
14. Reboot-verified the final daemon chain:
    - `dev.mealcheck.postgres` starts as a system `LaunchDaemon`
    - `dev.mealcheck.server` starts as a system `LaunchDaemon`
    - after boot settling, `GET /api/health` returns `status: ok` and
      `store: postgres`
15. Added `deploy/macos/wait-for-mealcheck-ready.sh` so operators can wait for
    Postgres and then the backend health endpoint after reboot.
    - Observed reboot check on the MacBook: Postgres became ready after about
      48 seconds; MealCheck health became ready about 20 seconds later.

Milestone 10 verification:

- `plutil -lint deploy/macos/dev.mealcheck.server.plist.template`
- `plutil -lint deploy/macos/dev.mealcheck.postgres.plist.template`
- `git diff --check`
- `go test ./...`

## Milestone 11: Cloudflare Pages And Tunnel Deployment

Status: Implemented on Cloudflare using a Git-integrated Pages project. The
Cloudflare Tunnel, API DNS route, tunnel LaunchDaemon, Pages project, GitHub
build integration, Pages custom domain, and production CORS pairing are active.

Deliver:

- Cloudflare Pages project connected to the repository
- production frontend URL and branch documented
- Pages settings for root directory, build command, and output directory
- public frontend configuration for the backend API base URL
- Cloudflare Tunnel configured on the MacBook
- public API hostname routed to the local backend service
- `MEALCHECK_ALLOWED_ORIGIN` set to the production frontend origin
- DNS/hostname records documented
- tunnel status and restart commands documented

Implemented:

1. Created Cloudflare Tunnel `mealcheck-api`.
   - tunnel ID: `e8cbd8da-735a-4053-9503-880f636670f6`
   - public API hostname: `api.mealcheck.dev`
   - local service: `http://127.0.0.1:8080`
2. Created MacBook-local cloudflared config at
   `/Users/chranama-server/.cloudflared/mealcheck-api.yml` and stored tunnel
   credentials outside the repository.
3. Added `deploy/macos/dev.mealcheck.tunnel.plist.template`, installed it as
   `/Library/LaunchDaemons/dev.mealcheck.tunnel.plist`, and verified
   `system/dev.mealcheck.tunnel` is running as `chranama-server`.
4. Verified `cloudflared tunnel info mealcheck-api` shows an active connector
   from the MacBook.
5. Verified the tunneled API serves `GET /api/health` through Cloudflare when
   resolving `api.mealcheck.dev` to the observed Cloudflare edge address.
6. Built the frontend with
   `VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.dev`.
7. Created Cloudflare Pages project `mealcheck`.
   - account ID: `0f5ac9230ddfc332774b414898e9f59f`
   - production branch: `main`
   - Pages URL: `https://mealcheck.pages.dev`
   - Git provider: `Yes`
   - repository: `chranama/MealCheck`
8. Deployed the frontend through Cloudflare's GitHub integration.
   - latest deployment ID: `dd76ce42-4a09-4482-b38e-0ba0a8d3b0f4`
   - branch: `main`
   - commit: `94271e5901938d1ced9dd675c264cf095fbbbac6`
   - root directory: `ui`
   - build command: `npm ci && npm run build`
   - build output: `dist`
   - public environment:
     `VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.dev`
9. Attached `mealcheck.dev` as a Pages custom domain. Cloudflare returned
   the frontend from the Git-integrated Pages project after the custom domain
   was rebound.
10. Updated the MacBook backend runtime environment so
    `MEALCHECK_ALLOWED_ORIGIN='https://mealcheck.dev'`.
11. Restarted `dev.mealcheck.server` and verified production CORS behavior
    through the tunneled API:
    - `Origin: https://mealcheck.dev` receives
      `Access-Control-Allow-Origin: https://mealcheck.dev`
    - `Origin: https://not-mealcheck.example` does not receive an
      `Access-Control-Allow-Origin` header
12. Verified `https://mealcheck.pages.dev/demo-runs/index.json` serves the
    checked-in seeded demo index.
13. Verified public live run creation without
    `X-MealCheck-Invite-Token` returns `401` with an access-code-required
    response.

Milestone 11 note:

- The original deliverable said the Pages project should be connected to the
  repository. The first hosted Pages project was a Direct Upload project.
  Cloudflare's API returned `You cannot update the source object in a Direct
  Uploads project` when asked to attach the repository, matching Cloudflare's
  Direct Upload rule that automatic Git deployments require a Git-integrated
  project. The Direct Upload project was deleted and recreated as a
  Git-integrated Pages project, then `mealcheck.dev` was rebound.

Acceptance:

- the production frontend URL loads from outside the home network
- the seeded report loads from the production frontend without backend access
- the production frontend shows backend health when the tunneled API is online
- the public API hostname serves `GET /api/health`
- live run creation is not available without a valid access code
- production CORS allows the Pages origin and does not allow arbitrary browser
  origins to use the write API
- no router port forwarding is required

## Milestone 12: Public Operations And MVP Acceptance Review

Status: Implemented on 2026-06-15 against the production URLs
`https://mealcheck.dev` and `https://api.mealcheck.dev`.

Deliver:

- final runbook commands for deploy or pull, start, stop, restart, status, logs,
  local health, public health, tunnel status, cleanup, backup, and deletion
- public smoke-test results from outside the home network
- source-pack update process
- nutrient catalog update process
- final MVP acceptance checklist with links to the production frontend and API
- confirmation that seeded public demo, live manual run, and live BYOK run work
  through the accepted production path
- confirmation that reports avoid medical claims and display source-pack
  versions
- confirmation that provider keys are absent from database fields, logs,
  reports, and artifact bundles checked during acceptance
- updated README status that the MVP is web-deployed

Implemented:

1. Redeployed the production frontend from commit
   `94271e5901938d1ced9dd675c264cf095fbbbac6`.
   - Pages deployment ID: `dd76ce42-4a09-4482-b38e-0ba0a8d3b0f4`
   - production asset observed on `https://mealcheck.dev`:
     `index-ANuw1idr.js`
2. Switched production live-run gating to per-user access codes by setting
   `MEALCHECK_INVITE_REQUIRED='true'` and leaving the legacy
   `MEALCHECK_INVITE_TOKEN` commented out.
3. Created an MVP smoke-test access code with public ID `wE-QP3n1pww`, expiry
   `2026-07-31T00:00:00Z`, and max-run limit `20`. The full access code was
   stored outside the repository.
4. Verified missing access code rejection through the public API returns `401`
   with `valid access code required`.
5. Ran a public invite-gated manual smoke run:
   - run ID: `run_4b5dbb4b5cf67e81faf990cb`
   - status: `completed`
   - decision: `block`
   - risk: `high`
   - report PDF and artifact list were available before deletion
   - run deletion was verified with subsequent `404` responses
6. Ran a fake-key BYOK privacy probe:
   - run ID: `run_2ee368a4048b694b49c2b81a`
   - status: `failed` as expected because the provider URL was intentionally
     unreachable
   - fake provider key was absent from service logs, artifact files, and
     `pg_dump` output
   - run deletion was verified with a subsequent `404`
7. Ran a production backup capture at
   `/Users/chranama-server/MealCheck-data/backups/20260615-150158`.
   - Postgres dump size: `10335` bytes
   - retained live artifact files at capture time: `0`
8. Verified retention/cleanup posture:
   - public health reports `retention_days: 7`
   - live artifact directory count after smoke-run deletion: `0`
9. Verified production CORS:
   - `Origin: https://mealcheck.dev` receives
     `Access-Control-Allow-Origin: https://mealcheck.dev`
   - `Origin: https://not-mealcheck.example` receives no allow-origin header

Acceptance:

- a public smoke test can inspect the seeded report, check backend health,
  create an invite-gated live run, observe completion, fetch report/artifacts,
  verify redacted provider config, delete the run, and confirm deletion
- backup commands have been run at least once against the deployed MacBook
  service or dry-run output is recorded
- cleanup or retention verification has been run against deployed artifacts
- documented recovery steps cover backend down, tunnel down, Postgres down,
  bad frontend API config, queue full, and provider failure
- README, runbook, backend server doc, and implementation plan all point to the
  same production URLs, paths, and service names
- the MVP acceptance checklist passes without local-only steps
- a reviewer can use the public frontend to understand the product without
  maintainer explanation
- an invite-gated reviewer can exercise the live path without maintainer-paid
  inference
- all operational commands required to keep the deployment running are present
  in `docs/runbook.md`

## Milestone 13: Contract Hardening

Status: Backend implementation completed on 2026-06-18; live API-level BYOK
smoke against real provider keys remains pending.

Purpose:

MealCheck is far enough along that the next implementation slice should harden
the contracts between model providers, backend normalization, artifact
generation, and frontend reporting. The goal is not broader model capability;
it is making the existing MVP path reliable, observable, and contract-driven
when real model outputs differ from the exact MealCheck schema.

Observed local test results:

- Bitwarden key retrieval worked for OpenAI, Anthropic, and Google AI Studio.
- Direct endpoint smoke tests succeeded for:
  - Gemini `gemini-2.5-flash-lite`
  - Anthropic `claude-haiku-4-5`
  - OpenAI `gpt-5.4-mini`
- Local MealCheck backend health, invite-token gating, and bad-provider failure
  handling worked.
- Real BYOK MealCheck runs reached the providers but failed during strict plan
  decoding because provider outputs used near-schema aliases:
  - Gemini returned `food_item`
  - Anthropic returned `meal_type`
  - OpenAI returned `meal`
- Provider keys were not found in the scanned runtime files.

Contract-hardening principle:

Production systems generally do not rely on prompt-only JSON repair as the
primary contract mechanism. The preferred hierarchy is:

1. Use provider-native structured output or tool/schema mode when available.
2. Parse and validate server-side against MealCheck's own contract anyway.
3. Apply small deterministic canonicalization for known safe aliases at the
   model-output boundary.
4. Make at most one bounded repair call with the exact schema, exact error, and
   original output.
5. Fail closed with redacted debug artifacts if the output still cannot become
   a valid MealCheck plan.

Backend improvement plan:

1. Centralize the MealCheck meal-plan output contract.
   - Keep one backend-owned schema description for provider requests,
     generation prompts, repair prompts, canonicalization tests, and validation
     fixtures.
   - Make the prompt skeleton exact enough to show `days[].meals[].name`,
     `days[].meals[].items[]`, and item-level `food`, `quantity`, and `unit`.
   - Explicitly forbid common aliases in prompts: `meal`, `meal_type`, `type`,
     `food_items`, `foods`, `ingredients`, `food_item`, `item`, and item-level
     `name`.

2. Prefer provider-native structured outputs.
   - OpenAI: use JSON Schema structured outputs with strict schema mode instead
     of plain JSON mode when compatible with the selected model.
   - Gemini: send `responseMimeType: "application/json"` and rely on
     server-side validation/canonicalization; the live REST endpoint rejected
     JSON-Schema-style `responseSchema` fields.
   - Anthropic: use JSON schema structured outputs through
     `output_config.format` for supported Claude models.
   - `openai_compatible`: retain current JSON mode as the compatibility
     fallback unless a provider-specific schema capability is explicitly added.

3. Add deterministic canonicalization before LLM repair.
   - Scope canonicalization to expected object positions only.
   - Meal object aliases:
     - `meal`, `meal_type`, `type` -> `name`
   - Meal item-array aliases:
     - `food_items`, `foods`, `ingredients` -> `items`
   - Food item aliases:
     - `food_item`, `item`, item-level `name` -> `food`
   - Preserve strict final validation after canonicalization.
   - Unknown fields outside this bounded alias set still fail.

4. Improve bounded repair.
   - Repair prompt receives the original output, the exact decode or validation
     error, the schema skeleton, and the alias mapping rules.
   - Repair must remove invalid fields after mapping them to allowed fields.
   - Repair may not invent missing foods, quantities, units, nutrition totals,
     or compliance judgments.
   - Only one repair attempt is allowed before failing the run.

5. Add failed-normalization observability.
   - For BYOK runs that fail before the normal artifact bundle is written, write
     redacted debug artifacts under the run artifact directory.
   - Include redacted provider metadata, initial provider output, initial
     decode/canonicalization error, repair output when attempted, and repair
     decode/canonicalization error when present.
   - Do not log or persist provider API keys. Debug artifacts must redact keys
     the same way normal provider config does.

6. Add regression and smoke coverage.
   - Add unit tests for observed aliases: `food_item`, `meal_type`, and `meal`.
   - Add canonicalization tests for `food_items`, `foods`, `ingredients`,
     `item`, and item-level `name`.
   - Add a negative test proving unknown fields outside the alias map still
     fail closed.
   - Add tests for failed-normalization debug artifacts and secret redaction.
   - Rerun local API-level BYOK smoke before UI testing.

Implemented:

1. Added a centralized backend meal-plan contract for provider schema payloads,
   generation prompts, repair prompts, alias rules, and decoder tests.
2. Added native structured-output payloads for OpenAI and Anthropic, Gemini JSON
   mode, and plain JSON mode for generic OpenAI-compatible providers. OpenAI
   uses the strict schema; Anthropic uses a portable schema subset because live
   provider validators reject some JSON Schema keywords.
3. Added deterministic canonicalization for the observed alias set before any
   LLM repair call.
4. Kept strict `DisallowUnknownFields` decoding and final MealCheck plan
   validation after canonicalization.
5. Added generated-plan count validation before artifact generation, so a
   provider output that parses but has the wrong number of days or meals gets
   the single bounded repair attempt before MealCheck writes the report.
6. Redacted provider output and decode errors before sending them to the repair
   prompt, so a provider-echoed API key is not sent back to the provider.
7. Added failed-normalization debug artifacts at
   `debug/normalization-failure.json` for provider runs that fail before the
   normal artifact bundle exists.
8. Added regression tests for provider aliases, unknown-field rejection,
   provider schema request payloads, generated-plan count repair,
   failed-normalization debug artifacts, and secret redaction.

Acceptance:

- API-level BYOK smoke runs complete for at least Gemini
  `gemini-2.5-flash-lite` through the full MealCheck backend path.
- Anthropic `claude-haiku-4-5` and OpenAI `gpt-5.4-mini` either complete or
  fail with redacted debug artifacts that identify the remaining contract issue.
- Provider output is either schema-valid on first pass, canonicalized by a
  bounded deterministic step, repaired once, or failed closed.
- Provider keys remain absent from logs, database fields, normal artifacts,
  debug artifacts, and runtime files scanned during BYOK smoke tests.
- Service-unavailable, rate-limit, and unreachable-provider failures remain
  readable to the frontend and do not create partial reports.
- UI BYOK testing resumes only after at least one provider completes through
  the API-level backend path.

## Milestone 14: BYOK Secret Handling Hardening

Status: Implemented on 2026-06-18.

Purpose:

MealCheck's BYOK path is a technical test surface, not a managed key vault.
The system should be explicit that provider API keys are one-run bearer
secrets, that hosted BYOK requires trusting the MealCheck backend process, and
that users who want the strongest trust posture should run MealCheck locally
from the repository with temporary, scoped, budget-limited provider keys.

Deliver:

- explicit BYOK trust-boundary copy in the web UI
- documentation that positions hosted BYOK as a convenience test surface and
  local operation as the higher-control path
- defensive redaction of successful provider output artifacts, not only
  failure/debug artifacts
- pending-input expiry metadata so queued BYOK keys cannot remain in process
  memory indefinitely
- cleanup-job deletion of expired pending BYOK inputs
- actionable failure if a queued BYOK input expires before a worker claims it
- tests that prove keys are absent from successful artifacts, debug artifacts,
  runtime files, browser storage, and error paths covered by the backend
- operational guidance requiring HTTPS and forbidding request-body/provider-
  header logging for hosted BYOK deployments

Implementation plan:

1. Treat `provider.api_key` as a one-run bearer secret.
   - Accept it only on BYOK generation requests.
   - Hold it only in the pending in-memory input until the worker claims the
     run.
   - Delete it on worker claim, run deletion, store failure, expiry, or cleanup.

2. Bound pending-key lifetime.
   - Add pending-input creation and expiry metadata.
   - Default the pending-input TTL from queue size and run timeout.
   - Expose `MEALCHECK_PENDING_INPUT_TTL` for local or hosted tuning.
   - Fail closed with a clear message if a BYOK run reaches the worker after
     the pending input has expired.

3. Redact every artifact path that can contain provider text.
   - Keep `configs/redacted-provider.json` as the only provider config artifact.
   - Redact exact API-key matches in failed-normalization debug artifacts.
   - Also redact exact API-key matches in successful `optional/llm-output.json`.

4. Make the trust model explicit.
   - The key transits the browser, MealCheck backend, and selected provider.
   - The key may briefly exist in browser state, request bodies, and Go process
     memory.
   - MealCheck does not persist keys to Postgres, runtime case files, reports,
     artifact bundles, logs, metrics, or browser storage.
   - Custom OpenAI-compatible `base_url` endpoints receive the key and must be
     trusted by the user.
   - Users should create temporary, project-scoped, budget-limited, revocable
     keys for MealCheck testing.

5. Document production guardrails.
   - Hosted BYOK requires HTTPS.
   - Reverse proxies, tunnels, observability tools, and application logs must
     not record request bodies or provider authorization headers.
   - Hosted queue sizes should stay small for BYOK test deployments.

Acceptance:

- `go test ./...` passes.
- Frontend tests pass and verify that BYOK form submission clears the API key
  without writing browser storage.
- Fake-provider tests prove a provider key is redacted from successful
  `optional/llm-output.json`.
- Pending-input tests prove fresh inputs are consumed, expired inputs are
  deleted, and cleanup removes expired pending entries.
- An expired queued BYOK run fails closed before provider invocation.
- API, CLI, privacy/safety, contracts, and runbook docs describe the same BYOK
  trust model.

## Milestone 15: Product Surface Tightening

Status: Implemented on 2026-06-19 for product and user-story documentation.

Purpose:

MealCheck's hosted website should have a sharper product role. The hosted
surface is a public demonstration and invite-gated BYOK verification playground,
not the primary structured manual-entry verifier and not a general meal-planning
chatbot. The downloaded repository remains the trusted local deployment and
debug surface, and the CLI preserves structured JSON verification for fixtures,
regression cases, and agent/tool integration.

Deliver:

- update product positioning so hosted MealCheck exposes seeded demos,
  invite-gated BYOK verification, and a local/agent-tool path
- remove the nontechnical hosted manual-entry user story from the target
  product narrative
- preserve structured JSON entry and validation in the CLI/local workflow for
  debugging and regression use
- define what qualifies as a meal plan before verification
- distinguish natural-language prompts, meal-plan outlines, recipes, and
  normalized ingredient-level meal plans
- make qualification separate from guideline verification

Implemented:

1. Updated `docs/product.md` so the primary users are technical users who
   already work with LLM-generated meal plans, plus developers evaluating the
   pattern. The hosted target surface is now demo reports, invite-gated BYOK
   verification, and local/agent-tool instructions.
2. Updated `docs/user-story.md` so the primary story is a technically capable
   user testing whether an LLM-generated or LLM-normalized plan qualifies for
   deterministic verification. Hosted manual structured entry is no longer a
   primary live story.
3. Added a meal-plan eligibility contract:
   - natural language prompts are not directly verifiable
   - vague menus and recipe titles are not enough
   - eligible plans require days, meals, ingredient-level items, quantities and
     units, or explicit unresolved quantity fields
   - qualification answers whether verification can proceed, while verification
     answers `pass`, `warn`, or `block`
4. Updated `README.md` to describe the hosted website as a demo/BYOK surface
   and the downloaded repo as the local verifier/debug and future agent-tool
   surface.

Acceptance:

- docs clearly remove the nontechnical hosted manual-entry story
- docs define what qualifies as a meal plan
- docs distinguish qualification from verification
- docs preserve local CLI structured JSON validation for debugging
- README, product, user-story, and implementation-plan docs describe the same
  product split

## Milestone 16: Meal Plan Qualification Contract

Status: Implemented on 2026-06-19 for backend contract and tests.

Purpose:

MealCheck needs a first-class way to answer whether candidate content qualifies
as a meal plan before deterministic guideline verification runs. Qualification
is separate from verification: it determines whether content can become
normalized ingredient-level MealCheck JSON, while verification determines
`pass`, `warn`, or `block`.

Deliver:

- backend qualification result contract
- deterministic qualification for already-structured MealCheck JSON
- text classification for content that is not a meal plan, too vague, or
  recipe/menu-like but not decomposed
- BYOK-assisted normalization path for text that already contains meal
  structure and ingredient quantities
- explicit `eligible_with_unresolved_items` result for plans that can be
  verified while preserving unresolved foods or quantities
- tests for non-meal text, vague meal outlines, recipe-like text, eligible
  structured JSON, eligible unresolved JSON, and BYOK-assisted text
  normalization

Implemented:

1. Added `MealPlanQualificationRequest` and `MealPlanQualificationResult` in
   `internal/hosted`.
2. Added qualification statuses:
   - `not_meal_plan`
   - `meal_plan_too_vague`
   - `recipe_or_menu_needs_decomposition`
   - `eligible_for_verification`
   - `eligible_with_unresolved_items`
3. Added deterministic structured JSON qualification using the existing strict
   MealCheck decode, bounded alias canonicalization, and `validatePlan`
   contract.
4. Added lightweight text classification before provider calls, so clearly
   non-meal, vague, or recipe-like text does not trigger BYOK inference.
5. Added BYOK-assisted normalization messages for text that already looks
   meal-plan-like and includes quantities or units.
6. Redacted the provider API key from qualification source text before sending
   prompts to a provider.
7. Added focused backend tests for all target statuses and provider-prompt
   secret redaction.

Acceptance:

- qualification can identify content that is not a meal plan
- qualification can identify meal-plan outlines that lack quantities or units
- qualification can identify recipe-like text that needs decomposition before
  verification
- already-normalized MealCheck JSON can qualify without a provider call
- normalized JSON with explicit unresolved items qualifies as
  `eligible_with_unresolved_items`
- detailed pasted text can use a BYOK provider to produce normalized MealCheck
  JSON
- qualification does not decide guideline compliance
- provider API keys are not copied into qualification prompts

## Milestone 17: Hosted BYOK Qualification Surface

Status: Implemented on 2026-06-19 for backend API, local smoke, and docs.

Purpose:

The hosted website should expose a focused BYOK demonstration surface instead
of a general manual structured verifier. Hosted users paste candidate meal-plan
text, ask whether it qualifies for verification, and use BYOK model providers
only when text needs normalization. Structured JSON verification remains
available in the downloaded repo through CLI/local case files for debugging,
regression testing, and future agent-tool integration.

Deliver:

- invite-gated `POST /api/qualify` endpoint for candidate meal-plan text
- qualification response that wraps the Milestone 16
  `MealPlanQualificationResult` contract
- BYOK provider invocation only when qualification text needs normalization
- hosted `/api/runs` limited to checked-in cases, `profile_generation`, and
  `prompt_generation`
- hosted rejection of `input_mode: "manual_structured"` with guidance to use
  the local CLI/debug workflow
- local smoke coverage that no longer depends on hosted manual structured input
- API, contract, runbook, CLI, and backend-server docs aligned with the hosted
  BYOK-only shape

Implemented:

1. Added `POST /api/qualify` to the hosted server.
2. Added `QualifyMealPlanResponse` so the API returns qualification results
   under a stable top-level `qualification` field.
3. Reused the Milestone 16 qualification path for structured JSON,
   deterministic text classification, and BYOK-assisted normalization.
   Already-normalized JSON and deterministic ineligible classifications do not
   require a provider key.
4. Added server-level `CompleterFactory` injection so qualification provider
   calls can be tested without live external endpoints.
5. Rejected hosted `manual_structured` run creation while preserving
   `case_path` compatibility for checked-in examples and smoke tests.
6. Updated the local smoke harness to process a checked-in seeded run plus a
   fake-provider BYOK run.
7. Added tests for invite gating, structured qualification response, BYOK text
   normalization through `/api/qualify`, and hosted manual-mode rejection.
8. Updated API, contract, runbook, CLI, and backend-server documentation.

Acceptance:

- `POST /api/qualify` rejects missing or invalid access codes when invite
  gating is enabled.
- `POST /api/qualify` returns a structured qualification result for already
  normalized meal-plan JSON.
- `POST /api/qualify` can call a BYOK provider to normalize detailed meal-plan
  text.
- hosted `/api/runs` rejects `input_mode: "manual_structured"` and queues no
  pending input.
- checked-in `case_path` runs still work for developer/demo compatibility.
- the CLI/local workflow remains the structured JSON verification path.
- docs describe the same hosted BYOK-only product shape across API, contracts,
  runbook, CLI, and backend-server references.

## Milestone 18: Frontend BYOK Qualification Workflow

Status: Implemented on 2026-06-19 for the Vite/React frontend and local
browser smoke coverage.

Purpose:

Milestone 17 moved the hosted backend contract to a BYOK qualification surface,
but the frontend still exposed the older hosted manual structured workflow. The
web UI should now match the product story: hosted users can paste candidate
meal-plan text, check whether it qualifies for verification, and separately
create BYOK profile or prompt generation runs. Structured JSON verification
stays in the CLI/local case-file workflow.

Deliver:

- frontend API client for invite-gated `POST /api/qualify`
- typed qualification payload, response, and UI state contracts
- hosted live workspace with candidate-text qualification preflight
- removal of hosted manual structured entry and hosted manual run payloads from
  the React workflow
- BYOK provider key clearing after qualification and generation requests
- visible qualification result summary for status, provider use, normalized
  plan size, and missing fields
- local fake-provider qualification support through `cmd/mealcheck-server`
- frontend unit and Playwright tests updated for qualification plus BYOK
  generation

Implemented:

1. Added `qualifyMealPlan` to the frontend API client.
2. Added `QualifyMealPlanPayload`, `QualifyMealPlanResponse`,
   `MealPlanQualificationResult`, and `QualificationState` frontend types.
3. Added `buildQualificationPayload`, which omits provider config unless a
   model or API key was supplied and normalizes provider config when BYOK is
   used.
4. Reworked `LiveWorkspace` so the first hosted action is candidate-text
   qualification and hosted run creation is limited to profile and prompt BYOK
   generation.
5. Added qualification result rendering in the live results panel.
6. Cleared provider API keys after both qualification and generation requests.
7. Wired `cmd/mealcheck-server` so `MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH`
   feeds both worker generation and `/api/qualify` normalization during local
   smoke tests.
8. Updated unit tests, mocked Playwright tests, local full-stack Playwright
   tests, runbook, privacy/safety docs, user story, and UI README.

Acceptance:

- hosted frontend has no manual structured entry mode or manual hosted run
  payload path
- user can submit a qualification request with an access code and candidate
  text
- qualification can run without provider config when deterministic
  classification is enough
- qualification can include BYOK provider config for normalization and clears
  the API key afterward
- BYOK profile and prompt generation still create asynchronous report runs and
  clear provider keys afterward
- local full-stack browser smoke verifies qualification through the fake
  provider and BYOK generation redaction
- frontend typecheck, unit tests, mocked e2e tests, local e2e tests, and build
  pass

## Milestone 19: Text-First Hosted Workspace

Status: Implemented on 2026-06-19 for the Vite/React frontend, user story, and
browser smoke coverage.

Purpose:

The hosted page had too much configuration before the user reached the actual
product question. Profile and constraint fields are still useful because they
parameterize guideline verification and profile-based generation, but the MVP
hosted surface should first ask for candidate meal-plan text and model provider
settings. Defaults should carry the common path, with profile and constraints
available as optional verification settings.

Deliver:

- hosted live workspace ordered around access, meal-plan text, and model
  provider setup
- collapsed Verification Settings section for profile and constraints
- unchanged backend payload contract and default profile/constraint behavior
- tests that assert settings are hidden by default but still configurable
- user-story and implementation-plan documentation aligned with the
  text-first hosted UX

Implemented:

1. Reordered `LiveWorkspace` so Access, Meal Plan Text, and Model Provider are
   the visible primary flow.
2. Moved Profile and Constraints into a collapsed Verification Settings
   disclosure without changing state or payload construction.
3. Renamed hosted UI copy from Qualification/Candidate Text/BYOK Provider to a
   clearer Meal Plan Text and Model Provider flow.
4. Added styling for the collapsed settings area and its nested advanced
   constraints disclosure.
5. Updated unit and Playwright coverage to check the text-first default state,
   hidden settings, settings expansion, and unchanged payload behavior.
6. Updated the user story so hosted BYOK flow treats profile and constraints as
   optional defaults rather than required first-screen configuration.

Acceptance:

- hosted first screen shows Access, Meal Plan Text, and Model Provider before
  optional verification settings
- Profile and Constraints are not visually imposed before the user reaches the
  core text/provider workflow
- expanding Verification Settings exposes all previous profile and constraint
  controls
- edited profile and constraint values still flow into report creation payloads
- qualification and BYOK generation continue to clear provider API keys after
  submission
- frontend typecheck, unit tests, mocked e2e tests, local e2e tests, and build
  pass

## Milestone 20: Hosted Verifier Settings Minimization

Status: Implemented on 2026-06-19 for the hosted frontend, BYOK prompt context,
and product/privacy documentation.

Purpose:

After moving profile and constraints behind Verification Settings, the remaining
panel still exposed fields that the verifier did not use directly. The hosted
surface should ask only for information that can customize verification or
provider meal-plan generation. Compatibility defaults can keep backend structs
valid, but unused demographic/profile fields should not be presented as user
requirements or sent to model providers as if they were user choices.

Deliver:

- hosted Verification Settings limited to direct nutrition targets and
  verification constraints
- removal of age, sex, height, weight, activity, goal, diet pattern, and
  shopping-list-required from the hosted settings UI
- BYOK provider prompt context filtered to the same exposed target/check fields
- frontend and backend tests that prevent reintroducing unused hosted settings
- user-story, privacy, decision-log, and implementation-plan docs aligned with
  the minimized settings surface

Implemented:

1. Renamed the Profile fieldset to Nutrition Targets and kept only calories and
   protein.
2. Kept direct verification constraints: days, meals per day, allergies,
   excluded foods, sodium, added sugar, saturated fat, calorie tolerance, and
   prep-safety requirement.
3. Removed unused hosted UI controls for age, sex, height, weight, activity,
   goal, diet pattern, and shopping-list requirement.
4. Renamed the visible profile-generation mode button to Targets while keeping
   the backend `profile_generation` mode name unchanged.
5. Added provider prompt settings filtering so generation, repair, and
   qualification prompts no longer send hidden compatibility profile/default
   fields.
6. Updated frontend unit tests and hosted backend tests for the minimized field
   set and provider prompt payload.
7. Updated the user story, privacy/safety policy, design notes, and decision
   log.

Acceptance:

- hosted Verification Settings exposes only fields with a direct verifier,
  generation, or provider-prompt effect
- removed fields are not present in the hosted settings DOM
- edited calorie, protein, day-count, and meal-count settings still reach report
  creation payloads
- BYOK provider prompt payloads include nutrition targets and verification
  constraints, but omit unused demographic/profile fields and unused switches
- frontend typecheck, unit tests, backend tests, mocked e2e tests, local e2e
  tests, and build pass

## Milestone 21: Settings Contract Simplification Across API And CLI

Status: Implemented on 2026-06-19 for hosted API requests, CLI case files,
frontend payloads, checked-in examples, and current contract documentation.

Purpose:

Milestone 20 minimized the hosted UI and provider prompt surface, but the API
and CLI still retained the older top-level `profile` and `constraints`
contract. That compatibility shape made the product story harder to explain and
kept unused demographic fields in the public contract. The MVP should expose one
settings contract everywhere: nutrition targets plus verification constraints.

Deliver:

- replace hosted `profile` and `constraints` request fields with one
  `settings` object
- replace CLI case-file `profile` and `constraints` fields with the same
  `settings` object
- reject old profile/constraints case files and unknown hosted request fields
- update frontend payload construction, API client tests, and browser tests to
  submit `settings`
- preserve report compatibility while sourcing report summaries from reduced
  settings
- update examples, API docs, CLI docs, contract docs, runbook, product/privacy
  docs, decision log, and implementation plan

Implemented:

1. Replaced `checker.Case.Profile` and `checker.Case.Constraints` with
   `checker.Case.Settings`.
2. Added `NutritionTargets` and `VerificationConstraints` structs under the
   shared `Settings` contract.
3. Updated deterministic checks, BYOK generation, repair, qualification, and
   runtime case writing to read from `settings`.
4. Added CLI case-file settings validation and regression coverage for rejecting
   old `profile`/`constraints` keys.
5. Updated hosted `POST /api/runs` and `POST /api/qualify` request types to
   accept `settings` and reject missing or invalid settings.
6. Updated the frontend state, normalization helpers, payload builders, and
   tests to send `settings.nutrition_targets` and
   `settings.verification_constraints`.
7. Updated checked-in example cases and local smoke request templates to the
   new contract.
8. Kept existing report artifact keys for compatibility, but populated them from
   reduced settings rather than removed profile/constraint structs.
9. Updated current API, contract, CLI, runbook, product, privacy, architecture,
   user-story, and web-design docs.

Acceptance:

- hosted qualification and run creation use `settings`, not top-level
  `profile` or `constraints`
- CLI case files use `settings`, and old `profile`/`constraints` fields fail as
  unknown fields
- missing CLI case settings fail with actionable validation errors before plan
  loading
- frontend request payload tests assert no top-level `profile` or `constraints`
  fields are emitted
- examples and docs show the same reduced settings contract
- backend tests, frontend typecheck, unit tests, mocked e2e tests, local e2e
  tests, and build pass

## Milestone 22: Public BYOK Access Mode And Policy Gate

Status: Implemented on 2026-06-19 for the hosted backend, frontend, tests, and
current product/API/privacy documentation.

Purpose:

The original access-code gate served two roles: limiting work on a small hosted
server and restricting live use to trusted people. After the product shifted to
BYOK, anonymous users no longer spend maintainer model budget. The remaining
hosted risks are service exhaustion, artifact/storage abuse, provider-key
transit, and custom-endpoint proxy/SSRF behavior. The hosted MVP should support
a public BYOK mode protected by hard admission policies, while preserving invite
mode for private deployments.

Deliver:

- explicit `MEALCHECK_ACCESS_MODE=public_byok|invite_required`
- public-mode admission policy for request rate and daily run count
- public-mode body/text/prompt length configuration
- `/api/health` access-mode and policy metadata for frontend adaptation
- conditional invite-token enforcement so public mode accepts live requests
  without `X-MealCheck-Invite-Token`
- public-mode `openai_compatible` custom endpoints disabled by default
- basic public custom-endpoint URL checks when explicitly enabled
- frontend Access field hidden in public mode and visible in invite mode
- frontend invite headers omitted when no access code is supplied
- docs and tests aligned with public BYOK as the default hosted shape

Implemented:

1. Added access-mode and public-policy fields to hosted config.
2. Added `PolicyLimiter` for per-client request rate and daily run limits.
3. Added `Retry-After` policy responses for public throttling.
4. Added candidate-text and generation-prompt length validation.
5. Added public custom endpoint policy that disables `openai_compatible` by
   default and rejects local/private/non-HTTPS custom URLs when enabled.
6. Exposed `access_mode`, policy metadata, and public custom endpoint status
   through `/api/health`.
7. Updated the frontend API client to parse health metadata and send invite
   headers only when a token exists.
8. Updated `LiveWorkspace` to hide access-code entry in public mode and show it
   in invite-required mode.
9. Updated mocked and local browser tests to exercise public BYOK without an
   access code.
10. Updated API, product, user-story, web-design, privacy/safety, decision-log,
    and implementation-plan docs.

Acceptance:

- public mode accepts `/api/qualify` and `/api/runs` without an access code
- invite-required mode still rejects missing or invalid access codes
- public request-rate and daily-run policy violations return `429`
- policy responses include `Retry-After`
- public hosted mode rejects unrestricted `openai_compatible` endpoints by
  default
- native OpenAI, Anthropic, and Gemini provider paths remain supported
- the frontend hides Access in public mode and omits the invite header
- the frontend shows Access in invite-required mode
- backend tests, frontend typecheck, unit tests, mocked e2e tests, local e2e
  tests, and build pass

## Milestone 23: Local llama.cpp Model Trial Harness

Status: Trial harness implemented locally on 2026-06-21. Server-side candidate
measurements are pending until GGUF models are downloaded and tested on the
MacBook.

Purpose:

MealCheck needs a no-key path to reduce hosted BYOK friction, but the deployed
MacBook has constrained CPU, memory, and disk. Before exposing a server-owned
local provider, the project needs repeatable evidence that small quantized
models can normalize meal-plan text into MealCheck JSON with acceptable latency
and without destabilizing the existing backend.

Deliver:

- a synthetic ingredient-level meal-plan normalization datapoint
- an inline llama.cpp response schema suitable for schema-constrained decoding
- a repeatable local llama.cpp structured JSON smoke script
- output artifacts that preserve raw responses, extracted model JSON, checker
  output, and summary timing data
- a trial matrix shape for 1B, 1.5B-1.7B, and 3B-4B quantized GGUF candidates
- acceptance criteria before any UI exposure of a no-key local model option

Implemented:

1. Added `examples/local-llama/synthetic-meal-plan.txt` as the first synthetic
   normalization datapoint.
2. Added `examples/local-llama/meal-plan-response.schema.json` as an inline
   compact verifier schema for llama.cpp `response_format` constrained
   decoding.
3. Added `examples/local-llama/README.md` with the manual `llama-server`
   command shape and model class ordering.
4. Added `scripts/test-local-llama-structured-json.sh`, which:
   - checks `llama-server` health through `/v1/models`
   - sends the synthetic datapoint through `/v1/chat/completions`
   - requests schema-constrained JSON
   - validates the resulting MealCheck shape with `jq`
   - optionally runs the deterministic MealCheck CLI checker against the model
     output
   - writes per-run artifacts, token/byte metrics, and `summary.jsonl`

Acceptance:

- each candidate model is started manually on `127.0.0.1:11435` with
  `--threads 3`, bounded context/batch sizes, and `--gpu-layers 0`
- the harness passes at least three consecutive synthetic normalization runs
  for a candidate before that candidate advances
- output includes valid MealCheck `schema_version: "0.1"`, one day, three
  expected meals, ingredient-level items, numeric quantities, and only
  supported units
- the deterministic MealCheck CLI can load and evaluate the candidate output
- measured latency and system responsiveness are recorded before backend
  integration begins
- models that fail schema, shape, latency, or memory criteria are not exposed in
  the hosted UI

## Milestone 24: Compact Local llama Contract Adapter

Status: Implemented on 2026-06-22 for the backend adapter, CLI helper, local
llama smoke harness, tests, and documentation.

Purpose:

The direct canonical MealCheck JSON contract still requires small local models
to spend tokens on structural field names such as `schema_version`, `days`,
`meals`, `items`, `food`, `quantity`, and `unit`. Server measurements showed
that lower quantization improved tokens per second but did not materially reduce
wall-clock latency once output length and constrained decoding overhead
dominated. The local model path should instead ask the model only for extracted
meal content, then let trusted MealCheck code expand that compact output into
canonical verifier JSON.

Deliver:

- a local llama compact output contract with meal keys and short item fields
  only
- a strict backend adapter that expands compact output into `checker.Plan`
- a CLI helper that can run the adapter outside the hosted server
- a compact llama.cpp response schema used by the local smoke harness
- local smoke artifacts that preserve compact model output and normalized plan
  output separately
- tests covering adapter success, adapter rejection, schema shape, and CLI
  expansion
- documentation and decision-log entries for the compact-contract boundary

Implemented:

1. Added `internal/hosted/local_llama_contract.go` with
   `DecodeLocalLlamaCompactPlan` and `LocalLlamaCompactResponseSchema`.
2. Added strict compact decoding with unknown-field rejection, required
   `breakfast`, `lunch`, and `dinner` meal keys, positive numeric quantities,
   and the existing supported-unit set.
3. Added canonical expansion to `checker.Plan` with
   `schema_version: "0.1"`, a server-owned plan id, day `1`, and canonical meal
   and food item fields.
4. Added `mealcheck local-llama normalize` to adapt compact JSON into
   `normalized-plan.json`.
5. Added `mealcheck local-llama schema` to emit the compact response schema
   used for llama.cpp constrained decoding.
6. Added `examples/local-llama/compact-meal-plan-response.schema.json` and
   updated the local llama smoke harness to request compact JSON, save
   `compact-plan.json`, adapt it through the Go CLI, and run the existing
   checker against canonical `normalized-plan.json`.
7. Updated `examples/local-llama/README.md` and `docs/cli.md`.
8. Added unit tests for the adapter and CLI helper.

Acceptance:

- compact model output never asks the model to emit canonical wrapper fields
  such as `schema_version`, `plan_id`, `days`, `meals`, `items`, `food`,
  `quantity`, or `unit`
- adapter output is canonical MealCheck JSON accepted by the deterministic
  checker
- malformed compact output fails closed before checker execution
- `mealcheck local-llama schema` matches the checked-in compact schema
- the local smoke harness continues to write raw response, compact output,
  normalized plan, checker output, token/byte metrics, and summary timing data
- backend and CLI tests pass before live server measurements

## Milestone 25: Local llama V2 Tuple Contract Compression

Status: Implemented on 2026-06-22 for the local llama adapter, CLI helper,
smoke harness, schema fixture, tests, and documentation.

Purpose:

The first compact local llama contract removed canonical MealCheck wrapper
fields, but the model still spent tokens on repeated meal names and item keys
such as `breakfast`, `lunch`, `dinner`, `f`, `q`, and `u`. Server measurements
showed the best stable CPU-only local path was near the hardware ceiling, so
the next latency lever is reducing generated token count further.

Deliver:

- a v2 tuple contract with top-level `b`, `l`, and `d` meal keys
- item tuples in the form `[food, quantity, unit]`
- adapter support for both the active v2 tuple shape and older object-item
  compact artifacts
- a shorter llama.cpp schema and prompt used by the local smoke harness
- a lower default local llama max-token cap appropriate for tuple output
- tests and docs that make the active contract explicit

Implemented:

1. Updated `DecodeLocalLlamaCompactPlan` to detect tuple keys and decode the
   v2 shape before expanding it into canonical `checker.Plan`.
2. Preserved legacy object-item compact decoding for old local artifacts.
3. Updated `LocalLlamaCompactResponseSchema` and
   `examples/local-llama/compact-meal-plan-response.schema.json` to emit the
   v2 tuple schema.
4. Updated `scripts/test-local-llama-structured-json.sh` to prompt for `b`,
   `l`, and `d` tuple output and lowered the default output cap from `220` to
   `160` tokens.
5. Updated adapter, CLI, schema, and documentation tests.

Acceptance:

- `mealcheck local-llama normalize` accepts v2 tuple JSON and emits canonical
  MealCheck JSON
- old object-item compact JSON remains accepted by the adapter
- malformed tuple output fails closed before checker execution
- `mealcheck local-llama schema` matches the checked-in active compact schema
- local smoke artifacts continue to preserve raw model content, compact JSON,
  normalized plan, checker output, token/byte metrics, and summary timing data
- backend and CLI tests pass before live server measurements

## Milestone 26: Local llama Server Service Deployment

Status: Implemented on 2026-06-22 for the macOS service wrapper, launchd
template, install helper, deployment docs, and runbook.

Purpose:

The hosted web product is moving toward a server-owned local model mode for the
main public surface. The MacBook deployment therefore needs a supervised
`llama-server` process that matches the direct macOS service shape already used
by Postgres, the backend, the Cloudflare tunnel, and autodeploy.

Deliver:

- a `dev.mealcheck.llama` system `LaunchDaemon` template
- an env-file-driven wrapper for starting `llama-server`
- a non-secret llama service env template with measured CPU-only defaults
- an install/manage helper for install, restart, stop, status, and logs
- deployment documentation and runbook commands

Implemented:

1. Added `deploy/macos/mealcheck-llama.env.example` with the first production
   candidate model path and runtime flags.
2. Added `deploy/macos/mealcheck-llama-server.sh`, which loads
   `/Users/chranama-server/MealCheck-data/mealcheck-llama.env`, validates the
   binary/model paths, and execs `llama-server`.
3. Added `deploy/macos/dev.mealcheck.llama.plist.template`, a system
   LaunchDaemon bound to `127.0.0.1:11435` and running as `chranama-server`.
4. Added `deploy/macos/install-mealcheck-llama-service.sh` for installing and
   managing the service from the server checkout.
5. Updated deployment docs and runbook instructions.

Initial production-candidate defaults:

```bash
LLAMA_MODEL_PATH='/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf'
LLAMA_CTX_SIZE='4096'
LLAMA_THREADS='4'
LLAMA_GPU_LAYERS='0'
LLAMA_PARALLEL='1'
LLAMA_CACHE_RAM='512'
```

Acceptance:

- the plist lints with `plutil`
- shell scripts pass syntax checks
- the service starts under `system/dev.mealcheck.llama`
- `curl -fsS http://127.0.0.1:11435/v1/models | jq .` returns the loaded model
- the local structured-output smoke script passes against the launchd-managed
  model service before backend local-provider integration is enabled

## Milestone 27: Local llama V3 Row Contract

Status: Implemented on 2026-06-22 for the local llama adapter, schema fixture,
smoke harness prompt, backend local-model prompt, tests, and documentation.

Purpose:

The v2 compact tuple contract achieved low latency by asking the local model
for only `b`, `l`, and `d` meal buckets with `[food, quantity, unit]` tuples.
That contract was intentionally fast but fixed the hosted local model path to
one day with breakfast, lunch, and dinner. MealCheck needs a compact contract
that can represent multi-day and variable meal-count requests without going
back to verbose canonical model output.

Deliver:

- a compact row contract: `{"i":[[source_item_id, day, meal_code, food, quantity, unit]]}`
- bounded meal codes for breakfast, snacks, lunch, and dinner
- source item IDs so adapter code can detect omitted or duplicated source rows
- adapter grouping from compact rows into canonical `checker.Plan`
- compatibility with v3 row output, v2 tuple output, and older object-item
  compact artifacts
- schema and prompt updates for llama.cpp constrained decoding
- backend hosted local-model prompt updates using requested day and meal counts
- tests covering v3 expansion, legacy compatibility, schema shape, and hosted
  local provider requests

Implemented:

1. Added row decoding to `DecodeLocalLlamaCompactPlan`.
2. Added strict row validation for day range, meal code, positive quantity, and
   supported units.
3. Added deterministic grouping and ordering by day and meal code before
   canonical MealCheck expansion.
4. Preserved v2 `b`/`l`/`d` tuple decoding and object-item compact decoding for
   old artifacts.
5. Updated `LocalLlamaCompactResponseSchema` and
   `examples/local-llama/compact-meal-plan-response.schema.json` to emit the
   source-ID row schema.
6. Updated `scripts/test-local-llama-structured-json.sh` and hosted
   `local_model` prompts to ask for row output.
7. Raised default local-model output token caps so multi-day row output can
   complete while one-day runs still stop at completed JSON.
8. Strengthened the row contract with source item IDs after live tests showed
   the small local model could otherwise omit source lines while preserving
   valid JSON shape.

Acceptance:

- `mealcheck local-llama normalize` accepts source-ID row JSON and emits canonical
  MealCheck JSON.
- v3 row JSON, v2 tuple JSON, and older object-item compact JSON remain accepted.
- malformed row output fails closed before checker execution.
- missing or duplicated source item IDs fail closed before checker execution.
- `mealcheck local-llama schema` matches the checked-in active compact schema.
- hosted `local_model` no longer hard-requires exactly one day and three meals.
- backend and CLI tests pass before live server measurements.

## Milestone 28: Hosted Local-Model Product Rollout

Status: Implemented on 2026-06-22 for the repository, frontend contract,
backend prompt hardening, deployed smoke script, and documentation. Final
production acceptance requires restarting the deployed backend/llama services
after pulling this milestone on the MacBook host.

Purpose:

The public website should be a no-key MealCheck demonstration: paste a concise
ingredient-level meal plan, normalize it through the server-owned local
llama.cpp model, and run deterministic verification. BYOK and custom model
providers remain important, but they belong to the repo/API/CLI and self-hosted
power-user surface rather than the first hosted workflow.

Deliver:

- hosted UI without provider selector or API key field when
  `MEALCHECK_HOSTED_MODE=local_model`
- default pasted meal-plan example using supported quantity units
- backend local-model prompt support for inline meal-plan lines such as
  `Day 1 breakfast: 1 cup oatmeal, 1 cup berries, and 1 cup yogurt`
- deployed live smoke script that does not require provider keys
- docs that position `mealcheck.dev` as a bounded local-model demo and the repo
  as the BYOK/custom-provider/debug surface
- runbook commands for deployed local-model smoke testing

Implemented:

1. Simplified the hosted UI in local-model mode so the primary workflow is meal
   plan text, local model status, report creation, and results.
2. Updated README, product, user-story, API, contracts, privacy/safety,
   architecture, backend-server, and runbook docs to reflect hosted local model
   mode.
3. Added `scripts/test-deployed-local-model-live.sh`, which checks deployed
   health, submits a local-model run, fetches key artifacts, validates meal
   structure and item count, verifies provider config rejection, verifies
   oversized-input rejection, and deletes created runs by default.
4. Hardened backend local-model source extraction so inline meal descriptions
   are converted into numbered source item rows before prompting the small local
   model.
5. Preserved food names containing `and` while still splitting inline phrases
   when `and` introduces another quantified item.

Acceptance:

- `go test ./...` passes.
- frontend unit tests, typecheck, and build pass.
- `bash -n scripts/test-deployed-local-model-live.sh` passes.
- deployed `/api/health` reports `hosted_mode: "local_model"` and ready
  `local_model` metadata.
- deployed local-model smoke passes after the MacBook host pulls this milestone
  and restarts the backend.
- hosted UI renders without visible API key/provider controls in local-model
  mode.

## Milestone 29: Hosted Example Removal And Repo HTML Report

Status: Implemented on 2026-06-22.

Purpose:

The hosted website should present the local-model meal-check workflow directly,
without a secondary example-run block competing with the core product. The
seeded proof remains useful, but it belongs in the repository as a fixture,
static HTML artifact, and CLI/debug proof path.

Deliver:

- remove seeded example navigation from the hosted React shell
- stop boot-time loading of `demo-runs/index.json` from the frontend app
- add a standalone checked-in HTML report under `docs/`
- move the seeded artifact bundle out of Vite `public/` assets
- update current product/user-story/runbook docs to describe seeded proof as a
  repo artifact rather than a hosted website block

Implemented:

1. Simplified `App.tsx` so it always opens on the live local-model workflow and
   no longer loads the demo index.
2. Simplified `Sidebar` to a single active `New meal check` entry.
3. Moved the seeded artifact bundle from `ui/public/demo-runs` to
   `examples/seeded-one-day-peanut-allergy/artifacts/demo-runs` so Cloudflare
   Pages no longer publishes it as a website asset.
4. Added `docs/seeded-report.html` as a self-contained static HTML report for
   the seeded proof case.
5. Updated README, docs index, product, user-story, runbook, and current MVP
   plan language.

Acceptance:

- production UI has no visible `Examples` block.
- frontend build does not need the seeded demo index for initial render.
- frontend build output does not contain `demo-runs`.
- the seeded proof remains inspectable from the repository.
- backend demo endpoints and checked-in fixtures can remain for compatibility,
  tests, and developer workflows.

## Milestone 30: Per-Day Local-Model Extraction

Status: Implemented on 2026-06-23.

Purpose:

Reduce local-model output pressure and improve multi-day robustness by
decomposing clear multi-day natural-language inputs before they reach the small
llama.cpp model.

Deliver:

- detect unambiguous `Day N` sections for every requested hosted local-model day
- rewrite each section into a one-day extraction prompt
- call the server-owned local model once per day
- restore original day numbers and merge canonical MealCheck plan days
- preserve the existing whole-plan fallback for ambiguous inputs
- record normalization events when decomposition is used

Implemented:

1. Added per-day local-model extraction orchestration in the hosted backend.
2. Added conservative day-section detection that requires complete day coverage
   and at least one resolved source item per section.
3. Reused the compact row contract for each one-day call and merged decoded
   canonical day plans before deterministic verification.
4. Kept total source-item count validation after the merge so omitted rows still
   fail closed.
5. Documented the behavior in the API docs and decision log.

Acceptance:

- `go test ./...` passes.
- clear two-day local-model input creates one provider call per day.
- normalized output preserves original day numbers and total item count.
- ambiguous multi-day text falls back to whole-plan extraction.

## Milestone 31: Seven-Day Unbatched Fallback Capacity

Status: Implemented on 2026-06-23.

Purpose:

Keep per-day extraction as the preferred path for clear `Day N` inputs, while
raising the hosted local-model fallback limits enough to attempt concise
seven-day plans when day boundaries cannot be safely decomposed.

Deliver:

- raise hosted local-model candidate text limit from `4000` to `6000`
  characters
- raise hosted local-model output cap from `512` to `1536` tokens
- raise hosted local-model timeout from `90s` to `240s`
- raise llama.cpp service context from `2048` to `4096`
- keep `Qwen3-0.6B-Q4_K_M.gguf`, CPU-only serving, four threads, one slot, and
  `512` MB prompt cache
- keep the public UI example short by showing two day-labeled days

Implemented:

1. Updated backend defaults and macOS deployment env examples.
2. Updated the llama service wrapper and env template to default to
   `LLAMA_CTX_SIZE=4096`.
3. Updated API, runbook, robustness manifest, and UI tests to advertise the new
   hosted local-model limits.
4. Shortened the hosted UI default meal-plan text from three days to two days.

Acceptance:

- `go test ./...` passes.
- UI typecheck, test, and build pass.
- deployed server env files are updated manually or over SSH, then
  `dev.mealcheck.server` and `dev.mealcheck.llama` are restarted.

## Milestone 32: Graceful Non-Meal-Plan Failure Modes

Status: Implemented on 2026-06-23.

Purpose:

Make hosted local-model verification fail gracefully when the pasted input is
not a verifiable meal plan, both before model invocation and after model
normalization failures.

Deliver:

- pre-model qualification fast-fail for obvious non-meal-plan text
- pre-model refusal for vague meal outlines with no quantities or units
- pre-model refusal for recipe-like text that needs day/meal decomposition
- structured `422 meal_plan_not_verifiable` API response with qualification
  details
- UI rendering that treats structured qualification refusal as expected feedback
  rather than a generic API error
- friendly post-model failed-run messages that do not expose compact JSON,
  parser, model-path, or source-item internals
- debug artifacts that preserve sanitized model/decode details for operators
- synthetic qualification-failure fixtures alongside the acceptable-input
  robustness dataset

Implemented:

1. Reused the deterministic meal-plan classifier in hosted `local_model` run
   creation before queueing.
2. Added a typed qualification rejection error so the run endpoint can return a
   structured `422` without string matching.
3. Added local-model-specific post-model failure wrapping that preserves
   `debug/normalization-failure.json` while storing a user-facing run error.
4. Added frontend API parsing for qualification details inside error envelopes.
5. Added documentation and synthetic failure cases for invalid, vague, and
   recipe-like inputs.

Acceptance:

- obvious non-meal-plan local-model requests return `422` and do not queue a
  run.
- fast-failed requests report `provider_used: false`.
- local-model decode/completeness failures store a friendly public run error.
- redacted debug artifacts still include the detailed normalization failure
  lifecycle.
- focused backend and frontend tests pass.

## Milestone 33: Deterministic Plan Recommendations

Status: Implemented on 2026-06-23.

Purpose:

For `block` and `warn` results, MealCheck should attempt to provide a concrete
modified meal plan when a safe deterministic edit is available. The feature
must support the product story without turning MealCheck into a generative meal
planner: recommendations are explicit edits of the submitted plan, and they are
published only when the modified plan re-evaluates to `pass`.

Deliver:

- backend recommendation generator with no model calls
- structured `recommendation.json` artifact in every bundle
- conservative unavailable response when no bounded edit is available
- explicit change list for available recommendations
- projected checker decision for the modified plan
- artifact wiring through `decision.json` and `manifest.json`
- documentation of recommendation principles and artifact boundary

Implemented:

1. Added `internal/recommend` with deterministic recommendation generation.
2. Supported initial edit classes for prep-safety notes, allergen/excluded-food
   substitutions, and missing vegetable coverage.
3. Required the modified plan to pass `checker.Evaluate` before returning an
   available recommendation.
4. Refused recommendations for missing meal structure and unresolved quantities
   instead of guessing nutrition-critical details.
5. Added `recommendation.json` to artifact bundle generation, manifests, CLI
   artifact expectations, and `decision.json` artifact paths.
6. Added `docs/plan_recommendation.md` and linked it from the docs index,
   README, CLI docs, and contracts.

Acceptance:

- `go test ./...` passes.
- passing source plans produce an unavailable recommendation because no edit is
  needed.
- unsupported or unresolved source plans produce an unavailable recommendation
  without leaking a modified plan.
- supported deterministic edits produce an available recommendation only when
  the projected decision is `pass`.
- generated artifact bundles include `recommendation.json` and list it in
  `manifest.json`.

## Milestone 34: Public Operational Status Page

Status: Implemented on 2026-06-23.

Purpose:

Give users a small public status surface that answers whether MealCheck is
usable right now. This is an SDLC operations and maintenance artifact, not an
admin dashboard: it summarizes user-visible capabilities, supports release
verification, and avoids exposing raw operational diagnostics.

Deliver:

- public `GET /api/status` endpoint with a stable summarized status contract
- public `/status.html` frontend page, also reachable as `/status` in the
  deployed site
- component-level states for the website, meal-check submission, AI meal
  normalization, nutrition and allergen checking, report generation, and the
  sample report
- top-level service message, latest checked time, recent-incident summary, and
  sample-report link
- shared footer navigation across the main app, status page, and consumer
  about page
- deployed frontend runtime config at `/config.json` so static hosting can
  resolve the production API base URL

Non-goals:

- no authenticated admin dashboard
- no logs, queue depth, hostnames, paths, model filenames, raw `/api/health`
  payloads, policy limits, secrets, or recent user input on the public page
- no incident-management workflow beyond the initial recent-incidents summary
- no operational controls or destructive actions

Implemented:

1. Added `/api/status` as the public status contract, separate from
   `/api/health`.
2. Added backend status synthesis for operational, degraded, partial outage,
   and major outage cases.
3. Added redaction-oriented backend tests so public status does not leak raw
   queue, store, or local-model details.
4. Added the React/Vite status page with summarized component rows and recent
   incidents.
5. Added shared footer navigation to the main app, status page, and consumer
   about page.
6. Removed stale `Ready` and `Status` labels from the main page.
7. Added mocked Playwright coverage for the status page and footer navigation.
8. Added `ui/public/config.json` after deployed verification showed that the
   status page could not discover `https://api.mealcheck.dev` from static
   hosting.
9. Updated API, runbook, architecture, and frontend-hosting documentation for
   the public status contract and runtime config boundary.

Acceptance:

- `/api/status` returns HTTP 200 with a summarized public status payload.
- overall service state remains available even when one component is degraded
  or partially unavailable.
- public status output does not expose raw operational details intended for
  operators.
- `/status.html?api=/mock-api` renders the top-level status, component rows,
  recent-incidents summary, and sample-report link.
- deployed `https://mealcheck.dev/config.json` returns the production API base
  instead of the frontend HTML fallback.
- deployed `https://mealcheck.dev/status` shows `All systems operational` when
  the production API and sample report are healthy.
- main, status, and about pages all include footer links to MealCheck, Status,
  About, and GitHub.
- browser-level coverage keeps the status page and footer navigation from
  regressing.

Verification:

- `go test ./...` passes.
- `cd ui && npm run typecheck` passes.
- `cd ui && npm test` passes.
- `cd ui && npm run build` passes.
- `cd ui && npm run test:e2e` passes.
- `cd ui && npm run test:e2e:local` passes against the local stack.
- Chrome verification on the deployed site showed
  `https://mealcheck.dev/status` reporting `All systems operational`.

## Milestone 35: CI Proof Gates

Status: Implemented on 2026-06-24.

Purpose:

Make MealCheck's proof claims executable on every change. The CI workflow should
protect the backend contracts, checked-in fixtures, frontend contracts, mocked
browser flow, and local full-stack path that support the deployed LLM workflow.

Deliver:

- GitHub Actions workflow for pushes to `main`, pull requests, and manual
  dispatch
- backend job for fixture validation, Go tests, and local CLI/API smoke proof
- frontend job for TypeScript, unit tests, production build, and mocked
  Playwright e2e
- local-stack e2e job that starts the real Go backend with memory storage and a
  fake provider response path
- Playwright artifact upload on browser-test failure
- README badge and CI proof summary
- runbook separation between always-on CI gates and deployed release checks

Implemented:

1. Added `.github/workflows/ci.yml` with separate backend, frontend, and
   local-stack e2e jobs.
2. Configured Go from `go.mod` and frontend CI with Node 22 to match the
   documented Cloudflare Pages runtime.
3. Added fixture validation and `go test ./...` as backend gates.
4. Added `go run ./cmd/mealcheck local-smoke` so CLI artifact generation,
   hosted API behavior, CORS, run deletion, and provider-secret redaction remain
   covered in CI.
5. Added frontend typecheck, unit tests, production build, and mocked
   Playwright e2e.
6. Added local-stack Playwright e2e as a dependent CI job after the backend and
   frontend proof gates pass.
7. Added Playwright artifact upload for failed browser jobs.
8. Documented the CI contract in the README and runbook.

Acceptance:

- CI runs automatically on pushes to `main` and pull requests.
- `go run ./cmd/mealcheck fixture-check` passes in CI.
- `go test ./...` passes in CI.
- `go run ./cmd/mealcheck local-smoke` passes in CI.
- `cd ui && npm run typecheck` passes in CI.
- `cd ui && npm test` passes in CI.
- `cd ui && npm run build` passes in CI.
- `cd ui && npm run test:e2e` passes in CI without a live backend.
- `cd ui && npm run test:e2e:local` passes in CI against the local stack.
- failed browser jobs publish Playwright traces or reports when available.
- deployed live checks stay documented as release operations rather than
  required hosted CI checks.

Verification:

- local deterministic checks pass before pushing the workflow.
- first GitHub Actions run for the workflow passes after push.

## Milestone 36: User-Facing Failure Recovery UX

Status: Implemented on 2026-06-24.

Purpose:

Turn backend graceful failure modes into user-facing recovery loops. The
backend already classifies non-verifiable input, capacity failures, provider
failures, local-model failures, and service errors; the frontend should explain
what happened, distinguish input problems from service problems, and give the
next useful action.

Deliver:

- frontend recovery mapper for API error codes and qualification statuses
- inline meal-plan guidance for non-meal-plan, vague, recipe-like, and
  unresolved-item outcomes
- service recovery notices for queue full, rate limits, local-model outage,
  provider failure, missing access code, backend storage failure, and API
  unreachable cases
- failed-run recovery guidance for post-queue local-model normalization
  failures
- public status-page links where the likely issue is service availability
- test coverage for recovery mapping, inline guidance, service-busy behavior,
  and failed-run guidance

Implemented:

1. Added `ui/src/lib/recovery.ts` as the frontend recovery-policy boundary.
2. Added a shared `RecoveryNoticeView` component for consistent recovery
   notices.
3. Replaced raw global API error strings with typed recovery notices.
4. Added inline meal-plan text guidance from qualification results.
5. Added failed-run recovery guidance for local-model normalization failures.
6. Corrected frontend request-ID formatting for backend error envelopes.
7. Added unit and Playwright coverage for the new recovery paths.

Acceptance:

- vague meal-plan qualification displays concrete quantity/unit guidance near
  the meal-plan text box.
- queue-full API responses display service-busy guidance without surfacing raw
  `HTTP 429` text.
- API-unreachable and service-failure states point users toward the public
  status page.
- post-queue normalization failures tell users how to rewrite and retry the
  meal plan.
- existing successful qualification and report-creation flows still pass.

Verification:

- `git diff --check` passes.
- `cd ui && npm run typecheck` passes.
- `cd ui && npm test` passes.
- `cd ui && npm run build` passes.
- `cd ui && npm run test:e2e` passes.
- `cd ui && npm run test:e2e:local` passes.

## Milestone 37: Frontend Quality Pass

Status: Implemented on 2026-06-24.

Purpose:

Preserve the core MealCheck user workflow across common frontend quality risks:
mobile layout, keyboard recovery, accessible destructive-action handling, and
non-essential operational UI that can distract from the primary task.

Deliver:

- remove the local-model status/info box from the main workflow
- keep local-model readiness surfaced through the action strip and disabled
  create action rather than a separate panel
- mobile browser coverage for the primary meal-plan qualification flow
- browser assertions that the fixed mobile action strip does not cause
  horizontal overflow or clipped action labels
- keyboard-accessible delete confirmation with focus on the safe action and
  Escape-to-cancel support
- unit coverage for local-hosted workspace behavior and keyboard deletion
  recovery

Implemented:

1. Removed the main-page `LocalModelPanel` and its dedicated styles.
2. Kept local-model unavailability private to the action strip feedback and
   disabled create button.
3. Added focus management and Escape cancellation to the delete confirmation
   dialog.
4. Adjusted mobile action-strip button wrapping so long labels fit narrow
   screens.
5. Added mocked Playwright coverage for the mobile primary workflow and action
   overflow.
6. Added mocked Playwright coverage for keyboard cancellation of report
   deletion.
7. Updated unit coverage for the simplified local-model workspace and delete
   dialog keyboard path.

Acceptance:

- hosted local-model mode no longer renders a separate availability box or
  repository link in the main workflow.
- unavailable local-model mode still disables report creation and explains the
  blocking state in the action strip.
- mobile users can complete the primary qualification workflow without
  horizontal page overflow or clipped primary action labels.
- keyboard users can open the delete confirmation, land on Cancel, dismiss with
  Escape, or activate Cancel without deleting the report.
- existing desktop mocked workflows, status page checks, and local-stack e2e
  checks continue to pass.

Verification:

- `git diff --check` passes.
- `cd ui && npm run typecheck` passes.
- `cd ui && npm test` passes.
- `cd ui && npm run build` passes.
- `cd ui && npm run test:e2e` passes.
- `cd ui && npm run test:e2e:local` passes.

## Milestone 38: FNDDS-Grounded Catalog And Evaluation Expansion

Status: Implemented on 2026-06-24.

Purpose:

Move the meal-plan catalog expansion from hand-authored estimates to a
repeatable, source-grounded SDLC artifact. The evaluation dataset now provides a
measured basis for deciding which common foods belong in the fast local catalog
and which long-tail or vague inputs should remain unresolved.

Deliver:

- USDA FNDDS 2021-2023 grounded catalog-generation script
- expanded local catalog with at least 100 reviewed foods
- source references on generated catalog foods
- 100-case evaluation dataset covering common, vegetarian, vegan, high-sodium,
  high-added-sugar, allergen-risk, low-protein, long-tail unresolved, and
  vague-quantity scenarios
- baseline evaluation result for the original 17-food catalog
- expanded-catalog evaluation result for the FNDDS-grounded catalog
- fixture validation that keeps catalog labels, food groups, allergens, and
  evaluation categories deterministic
- documentation of the added-sugar proxy limitation and future FPED/FoodData
  Central direction

Implemented:

1. Added `scripts/generate-fndds-evaluation.py` to read the FNDDS 2021-2023 At
   A Glance foods, nutrient-values, and portions workbooks.
2. Expanded `data/nutrients/fixture-catalog-v1.json` to a 151-food reviewed
   FNDDS subset while preserving deterministic exact-match aliases.
3. Added optional catalog `source_refs` support to the checker type and nutrient
   catalog schema.
4. Added `data/evaluation/fndds-grounded-meal-plans-v1.json` with 100
   structured one-day meal-plan cases and expected outcomes.
5. Added `mealcheck eval-checker` as a deterministic resolver/evaluation runner.
6. Updated fixture validation to require at least 100 foods, source-compatible
   catalog quality, and the 100-case evaluation dataset.
7. Recorded baseline and expanded resolver results:
   - original 17-food catalog: 296 of 900 items resolved, 32.89%
   - FNDDS-grounded catalog: 885 of 900 items resolved, 98.33%
   - expected-outcome mismatches with expanded catalog: 0
8. Documented regeneration, evaluation, current results, and the added-sugar
   proxy in `docs/evaluation.md`.

Acceptance:

- the local catalog is generated from reviewed FNDDS rows rather than rounded
  synthetic nutrient estimates.
- every generated food has a source reference, nutrients per 100 g, explicit
  unit conversions, reviewed allergens, and reviewed food groups.
- the evaluation dataset contains exactly 100 cases and all required scenario
  categories.
- strict evaluation passes with zero expected-outcome mismatches on the expanded
  catalog.
- unresolved items in the expanded result are intentional long-tail foods or
  vague quantities, not missing common-food coverage.

Verification:

- `python3 scripts/generate-fndds-evaluation.py` passes.
- `python3 -m py_compile scripts/generate-fndds-evaluation.py` passes.
- `go run ./cmd/mealcheck fixture-check` passes.
- `go run ./cmd/mealcheck eval-checker` passes.

## Milestone 39: WWEIA/NHANES Real-Recall Evaluation Layer

Status: Implemented on 2026-06-24.

Purpose:

Add a realism layer to the evaluation system by transforming public
WWEIA/NHANES dietary interview records into MealCheck evaluation cases. FNDDS
continues to ground the local catalog and food descriptions; WWEIA/NHANES adds
real reported eating occasions, gram weights, and full-day recall patterns that
surface catalog gaps.

Deliver:

- pure-Python NHANES XPT reader for the required public dietary interview files
- generator for a deterministic 100-case WWEIA/NHANES evaluation dataset
- adult reliable-recall filtering using NHANES demographics
- source traceability on each generated case
- source metrics for food-item count, local-catalog coverage, source nutrients,
  and unresolved FNDDS food codes
- evaluation result artifact for the expanded local catalog
- fixture validation for the real-recall dataset shape and required categories
- documentation distinguishing food composition data, synthetic regression
  cases, and dietary recall data

Implemented:

1. Added `scripts/generate-wweia-nhanes-evaluation.py` to read `DR1IFF_L.xpt`,
   `DR2IFF_L.xpt`, `DEMO_L.xpt`, and the FNDDS Foods and Beverages workbook.
2. Generated `data/evaluation/wweia-nhanes-real-recalls-v1.json` with 100
   cases:
   - 40 fully resolved real eating-occasion cases
   - 30 high-coverage full adult recall days
   - 20 high-sodium full adult recall days
   - 10 low-protein full adult recall days
3. Preserved real reported gram quantities and eating occasions while mapping
   FNDDS food codes to user-facing food descriptions.
4. Marked nonlocal FNDDS foods as intentional `unknown_food` unresolved items
   so the dataset drives catalog expansion instead of guessing.
5. Extended `mealcheck eval-checker` to accept per-case source refs, source metrics,
   and top-level dataset summaries.
6. Added `data/evaluation/results/wweia-nhanes-real-recalls-v1.json`.
7. Updated fixture validation so both evaluation datasets must contain exactly
   100 cases and their required categories.
8. Documented source files, regeneration commands, current results, and
   limitations in `docs/evaluation.md`.

Acceptance:

- the real-recall dataset is generated from public deidentified
  WWEIA/NHANES rows, not hand-authored meal examples.
- every case has source text, source refs, expected outcomes, normalized
  MealCheck plan JSON, and source metrics.
- fully resolved real eating occasions provide a clean regression slice.
- full-day recalls expose realistic catalog gaps for future local-catalog or
  FoodData Central fallback work.
- strict evaluation passes with zero expected-outcome mismatches on the
  expanded catalog.

Current Results:

- WWEIA/NHANES dataset: 100 cases and 815 food items.
- Expanded local catalog resolves 496 of 815 items, 60.86%.
- Expected-outcome mismatches: 0.
- Top current catalog gaps include tap water, white rolls, granulated sugar,
  wine, apple juice, instant coffee, saltine crackers, flavored liquid coffee
  creamer, and common mixed dishes.

Verification:

- `python3 scripts/generate-wweia-nhanes-evaluation.py` passes.
- `python3 -m py_compile scripts/generate-wweia-nhanes-evaluation.py` passes.
- `go run ./cmd/mealcheck fixture-check` passes.
- `go run ./cmd/mealcheck eval-checker -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json -out data/evaluation/results/wweia-nhanes-real-recalls-v1.json` passes.

## Milestone 40: FNDDS Reference Database And Candidate Preprocessing

Status: Implemented on 2026-06-24.

Purpose:

Create a full local FNDDS 2021-2023 reference database while keeping the
reviewed MealCheck resolver catalog conservative. The reference layer preserves
all source rows, then classifies foods into eligible, quarantined, and
review-required candidate sets so future WWEIA frequency mining can recommend
catalog additions without blindly adding ambiguous foods.

Deliver:

- full FNDDS food, nutrient, and portion reference artifacts
- source manifest documenting source URLs, raw workbook paths, and generation
  command
- deterministic preprocessing classifier for resolver-candidate eligibility
- split candidate artifacts for eligible, quarantined, and review-required
  foods
- classification summary with status and ambiguity-flag counts
- fixture validation for reference-file existence, count consistency, source
  refs, nutrient completeness, candidate statuses, flags, and known examples
- documentation that distinguishes full source reference data from the reviewed
  MealCheck resolver catalog

Implemented:

1. Added `scripts/import-fndds-reference.py` to read the FNDDS 2021-2023 Foods
   and Beverages, Nutrient Values, and Portions and Weights workbooks.
2. Generated `data/reference/fndds/source-manifest.json`.
3. Generated `data/reference/fndds-2021-2023/foods.jsonl`,
   `nutrients.jsonl`, `portions.jsonl`, `food-index.json`, and
   `manifest.json`.
4. Generated split preprocessing artifacts:
   - `resolver-candidates.jsonl`
   - `quarantined-foods.jsonl`
   - `review-required-foods.jsonl`
   - `classification-summary.json`
5. Added deterministic ambiguity rules for `NFS`, not-specified descriptions,
   broad generic labels, mixed dishes, sandwiches, pizza, burritos, restaurant
   or product-style wording, unclear added-fat preparation, missing required
   nutrients, and multi-component allergen risk.
6. Added allowlist handling for common safe generic foods such as tap water,
   bottled water, brewed coffee, brewed tea, cooked rice, and 100% juice.
7. Extended `mealcheck fixture-check` to validate the FNDDS reference layer.
8. Updated `docs/evaluation.md` with artifact descriptions, generation command,
   current counts, and the role of quarantined rows.

Current Results After Later Resolver Expansion:

- FNDDS food rows preserved: 5,432.
- Eligible resolver candidates: 3,056.
- Quarantined rows: 2,375.
- Review-required rows: 1.
- Resolver match keys: 6,201.
- Source-backed unit conversions: 25,928.
- Approximation proxies: 95.
- Decomposition templates: 6.
- Decomposition rules: 31.
- Known examples:
  - `Water, tap` is `eligible_generic`.
  - `Milk, NFS` is `quarantined_ambiguous`.
  - `Milk, human` is `review_required` because the source row lacks the
    required nutrient row.

Acceptance:

- source rows are preserved rather than deleted.
- eligible candidates have complete required nutrients and no hard quarantine
  flags.
- ambiguous, mixed, restaurant/product-style, and unclear-preparation rows are
  quarantined for review instead of promoted to the resolver.
- split candidate files match the canonical `foods.jsonl` statuses.
- fixture validation fails if counts drift, statuses are invalid, source refs
  are missing, known examples classify incorrectly, or eligible foods carry
  hard quarantine flags.

Verification:

- `python3 scripts/import-fndds-reference.py` passes.
- `python3 -m py_compile scripts/import-fndds-reference.py` passes.
- `go run ./cmd/mealcheck fixture-check` passes.

## Milestone 41: FNDDS SQLite Fallback Resolver

Status: Implemented on 2026-06-24.

Purpose:

Add a conservative runtime lookup path from MealCheck's reviewed local catalog
to the full preprocessed FNDDS reference database. This gives MealCheck a
scalable local fallback for exact, source-backed common foods while preserving
the unresolved behavior for vague, ambiguous, composed, branded, or unsupported
entries.

Deliver:

- generated `data/reference/fndds-2021-2023/fndds.sqlite`
- SQLite schema for FNDDS foods, nutrients, portions, ambiguity flags,
  allergens, and food groups
- indexed exact-description lookup for eligible FNDDS rows
- runtime resolver fallback that is disabled unless a fallback path is supplied
- CLI and evaluation flags for the fallback database
- hosted worker configuration through `MEALCHECK_FNDDS_FALLBACK_PATH`
- tests proving reviewed catalog precedence, eligible fallback resolution,
  quarantined/review-required rejection, explicit `unknown_food` retry, and
  unsupported household-unit behavior
- fixture validation for SQLite table counts, indexes, known statuses, and
  resolver examples
- WWEIA/NHANES fallback coverage artifact

Implemented:

1. Extended `scripts/import-fndds-reference.py` to write `fndds.sqlite` from the
   same normalized FNDDS rows used by the JSONL reference layer.
2. Added `internal/checker/fndds_reference.go` using `modernc.org/sqlite` for
   read-only indexed fallback lookup.
3. Updated the resolver so the reviewed local catalog always wins. If it
   misses, the optional fallback can resolve exact FNDDS descriptions whose
   candidate status is `eligible_specific` or `eligible_generic` and which have
   no ambiguity flags.
4. Allowed explicit `unknown_food` items with quantities to retry the fallback,
   while preserving unresolved behavior for vague quantities, unsupported
   units, and other unresolved reasons.
5. Added `-fndds-fallback` to `mealcheck eval-checker`, `mealcheck validate`, and
   `mealcheck compare`.
6. Added `-skip-expected` to `mealcheck eval-checker` for coverage runs where the
   expected unresolved counts describe no-fallback mode.
7. Wired hosted workers to pass `MEALCHECK_FNDDS_FALLBACK_PATH` into artifact
   generation when explicitly configured.
8. Regenerated evaluation result artifacts with fallback metadata.
9. Updated `docs/evaluation.md` and `docs/architecture.md`.

Current Results After Later Resolver Expansion:

- FNDDS SQLite tables:
  - foods: 5,432
  - nutrients: 5,432
  - portions: 22,045
  - ambiguity flags: 3,503
  - allergens: 3,916
  - food groups: 10,107
- WWEIA/NHANES no-fallback coverage: 496 of 815 items resolved, 60.86%.
- WWEIA/NHANES with FNDDS fallback and later approximation/decomposition
  expansion: 774 of 815 items resolved, 94.97%.
- The fallback coverage run includes 690 exact resolutions, 45 estimated
  approximation resolutions, and 39 decomposed resolutions.
- The fallback coverage run skips expected-outcome comparison because the
  checked-in WWEIA expected unresolved counts describe no-fallback mode.

Acceptance:

- fallback is opt-in and does not change default resolver behavior.
- reviewed catalog matches take precedence over FNDDS fallback rows.
- fallback resolves exact normalized descriptions only; no fuzzy matching or
  alias guessing is performed.
- fallback rows must be eligible and carry no ambiguity flags.
- quarantined and review-required rows do not resolve at runtime.
- fallback supports `g`, `gram`, and `grams`; household portions remain
  unresolved until reviewed unit conversion policy is added.
- fixture validation fails if the SQLite snapshot drifts from the generated
  reference artifacts.

Verification:

- `python3 scripts/import-fndds-reference.py` passes.
- `python3 -m py_compile scripts/import-fndds-reference.py` passes.
- `go test ./internal/checker` passes.
- `go run ./cmd/mealcheck fixture-check` passes.
- `go run ./cmd/mealcheck eval-checker -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite -skip-expected -out data/evaluation/results/wweia-nhanes-real-recalls-with-fndds-fallback-v1.json` passes.

## Milestone 42: FNDDS Resolver Well-Defined Food Gate

Status: Implemented on 2026-06-25.

Purpose:

Keep the FNDDS SQLite fallback from becoming a blind lookup path. The fallback
database contains useful exact-match foods, but runtime lookup should only run
for entries that are specific enough to resolve without guessing.

Deliver:

- resolver gate between reviewed-catalog misses and FNDDS fallback lookup
- specific unresolved reasons for broad foods, mixed dishes, branded or
  restaurant items, unclear preparation, non-food text, and unsupported
  fallback units
- tests proving the reviewed catalog still takes precedence and fallback
  candidates are blocked before database lookup when they are not well-defined
- UI labels for the new unresolved reasons
- meal-plan schema and hosted contract vocabulary aligned with the resolver
  reasons
- documentation of the runtime lookup contract

Implemented:

1. Added `internal/checker/lookup_filter.go` to classify fallback candidates
   before database access.
2. Updated `internal/checker/resolve.go` so reviewed catalog matches still win,
   explicit `unknown_food` items with quantities can retry fallback, and all
   fallback candidates pass through the gate first.
3. Added resolver tests for broad one-word foods, mixed dishes, branded foods,
   non-food text, and unsupported fallback units.
4. Updated UI reason formatting so unresolved report evidence uses human
   language for the new reason codes.
5. Updated `schemas/meal-plan.schema.json`, hosted response schema metadata,
   `docs/evaluation.md`, and `docs/architecture.md`.

Acceptance:

- local reviewed-catalog matches bypass the fallback gate.
- unreviewed fallback lookup is limited to quantified gram-based foods.
- ambiguous, mixed-dish, branded, unclear-preparation, non-food, and
  unsupported-unit items remain unresolved with specific reasons.
- FNDDS fallback remains exact-match only after the gate allows lookup.
- report UI displays actionable labels for all resolver-gate reasons.

Verification:

- `go test ./internal/checker` passes.
- `go test ./...` passes.
- `go run ./cmd/mealcheck fixture-check` passes.
- `go run ./cmd/mealcheck eval-checker -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite -skip-expected -out data/evaluation/results/wweia-nhanes-real-recalls-with-fndds-fallback-v1.json` passes.
- `cd ui && npm run typecheck` passes.
- `cd ui && npm run test` passes.
- `cd ui && npm run build` passes.
- `git diff --check` passes.

## Milestone 43: First-Class Unresolved And De Minimis Policy

Status: Implemented on 2026-06-25.

Purpose:

Treat unresolved foods as explicit verification state while allowing a narrow,
opt-in de minimis exclusion policy for tiny unresolved mass items. The policy
must never count unresolved nutrients and must never silently hide excluded
items.

Deliver:

- optional `settings.verification_constraints.unresolved_policy`
- deterministic split between blocking unresolved items and excluded unresolved
  items
- `excluded-unresolved-foods.json` artifact plus decision/report/metrics
  visibility
- warning-only `quantities_resolvable` state when all unresolved items are
  excluded by policy
- reason-specific UI recovery actions for unresolved foods
- tests for strict default behavior, enabled de minimis warnings, cap overflow,
  vague quantities, allergy contexts, and totals excluding unresolved items

Implemented:

1. Added `settings.verification_constraints.unresolved_policy` to case and
   hosted request settings.
2. Split resolver output into blocking unresolved items and excluded unresolved
   items in `internal/checker/evaluate.go`.
3. Added `ExcludedUnresolvedItem` to decision, report, metric, and artifact
   output.
4. Added `excluded-unresolved-foods.json` to artifact manifests and report
   loading.
5. Updated report UI to show unresolved recovery actions and a separate
   "Excluded From Totals" section.
6. Added backend and frontend tests for strict defaults, enabled de minimis
   warnings, cap overflow, vague quantities, allergy contexts, and excluded
   unresolved report display.

Policy:

- default behavior remains strict: unresolved items block verification.
- de minimis exclusion requires explicit enablement and positive item, daily
  total, and daily count caps.
- only `unknown_food` and `missing_conversion:<unit>` unresolved items with
  deterministic mass quantities are eligible.
- vague, ambiguous, composed, branded, unclear-preparation, unsupported-unit,
  and non-food unresolved reasons remain blocking.
- allergy and excluded-food constraints disable de minimis exclusion.
- excluded unresolved items are never counted in nutrition totals and produce a
  warning, not a pass.

Acceptance:

- default unresolved behavior remains blocking.
- de minimis exclusion is opt-in and cap-bound.
- only deterministic mass quantities with `unknown_food` or
  `missing_conversion:<unit>` reasons can be excluded.
- allergy and excluded-food constraints disable exclusion.
- excluded unresolved items are visible in decision, metrics, artifact, and UI
  surfaces.
- nutrition totals exclude unresolved nutrients.
- `quantities_resolvable` warns, rather than passes, when all unresolved items
  are excluded by policy.

Verification:

- `go test ./...` passes.
- `go run ./cmd/mealcheck fixture-check` passes.
- `cd ui && npm run typecheck` passes.
- `cd ui && npm run test` passes.
- `cd ui && npm run build` passes.
- `git diff --check` passes.

## Milestone 44: FNDDS Match-Key, Approximation, And Decomposition Expansion

Status: Implemented through 2026-06-28.

Purpose:

Improve common-food and common-unit resolution without relaxing the conservative
resolver boundary. The fallback should resolve more ordinary source-backed
foods, but ambiguous, branded, restaurant, vague, unsupported-unit, and
unsafe allergy/exclusion cases must remain visible instead of being guessed.

Deliver:

- generated FNDDS resolver match keys with explicit `auto`, `blocked`,
  `decompose`, and `review` statuses
- source-backed food-specific unit conversions from FNDDS Portions and Weights
- curated and generated approximation proxies for broad/source-code generic
  foods that can be represented safely as estimated nutrition
- curated decomposition templates and source-code-backed family decomposition
  rules for selected composed foods
- runtime resolver support for `estimated` and `decomposed` resolution methods
- artifact, report, and eval visibility for estimated and decomposed foods
- tests for approximation proxies, decomposition rules, unit conversions,
  source-code mappings, and known gap fixes

Implemented:

1. Extended `scripts/import-fndds-reference.py` to generate
   `resolver-match-keys.jsonl`, `unit-conversions.jsonl`,
   `approximation-proxies.json`, and `decomposition-rules.json`.
2. Updated the SQLite reference layer to expose match-key lookup, approximation
   proxy lookup, decomposition-template lookup, decomposition-rule lookup, and
   source-food-code lookup.
3. Added source-backed unit conversions to fallback resolution while preserving
   unsupported-unit failure for units without deterministic conversion.
4. Added approximation resolution that marks resolved items with
   `resolution_method: estimated`, confidence, proxy metadata, and estimate
   reason.
5. Added decomposition resolution that splits eligible composed foods across
   component FNDDS foods and records component metadata.
6. Added `estimated_or_decomposed_foods` as a warning check so approximate
   resolver paths are visible in reports.
7. Added generated approximation proxies for conservative categories such as
   raw fruit, simple beverages, nuts, milk, simple proteins, legumes, and simple
   toppings.
8. Added decomposition rules and regression coverage for selected sandwiches,
   tuna sandwich, burrito, fruit drink, peanut, and other common WWEIA/NHANES
   gaps.

Current Results:

- FNDDS food rows preserved: 5,432.
- Eligible resolver candidates: 3,056.
- Quarantined rows: 2,375.
- Resolver match keys: 6,201, including 3,839 automatic keys.
- Source-backed unit conversions: 25,928.
- Approximation proxies: 95 total, 16 curated and 79 generated.
- Approximation source-code mappings: 95.
- Decomposition templates: 6.
- Decomposition rules: 31, with 33 source-code mappings and 97 components.
- WWEIA/NHANES with FNDDS fallback resolves 774 of 815 items, 94.97%.
- That coverage run includes 690 exact, 45 estimated, and 39 decomposed
  resolutions with zero expected-outcome mismatches when expected comparison is
  skipped for fallback coverage mode.

Acceptance:

- local reviewed-catalog matches still take precedence.
- fallback lookup remains exact-match or preprocessed-auto-key based.
- estimated foods carry explicit proxy metadata and produce a warning.
- decomposed foods carry component metadata and produce a warning.
- approximation and decomposition are disabled when allergy or excluded-food
  constraints make the proxy unsafe.
- source-backed unit conversions resolve only when the generated reference
  layer has deterministic gram factors.
- ambiguous, restaurant/product-style, unsupported-unit, and unresolved vague
  quantities remain visible.

Verification:

- `go test ./...` passes.
- `go run ./cmd/mealcheck fixture-check` passes.
- `go run ./cmd/mealcheck eval-checker -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite -skip-expected -out /tmp/mealcheck-wweia-fallback-current.json` passes.
- `git diff --check` passes.

## Milestone 45: P0 Normalization Evaluation Framework

Status: Deterministic seed tier, opt-in local-model runner, prototyping laptop
live-model regimen, and optional external-source scaffolding implemented.
Reviewed public-source promotion and live baseline analysis remain pending.

Purpose:

Create a measured evaluation loop for the highest-priority hosted failure mode:
turning in-bound pasted meal-plan text into canonical MealCheck JSON before the
deterministic checker runs. This milestone keeps normalization evaluation
separate from resolver coverage so failures can be attributed to source-item
inventory, compact row extraction, adapter expansion, qualification, or local
model behavior.

Deliver:

- documented P0 task boundary and case format
- generated-case plan for public ingredient-parsing datasets
- P0 source manifest and seed dataset generated from reviewed MealCheck
  robustness examples
- deterministic `mealcheck eval-normalization` command
- manifest, success-case JSONL, and failure-case JSONL loading
- source-item inventory scoring without llama.cpp
- compact-row adapter validation without llama.cpp
- tag summaries, failure summaries, source-item preservation rate, and optional
  JSON result output
- opt-in local-model evaluation mode for hosted-path baseline runs
- fixture-check validation for P0 dataset shape and counts
- command-level regression coverage for deterministic P0 evaluation
- command-level regression coverage for the local-model scoring path without a
  live llama.cpp service
- repeatable live-model regimen for prototyping-laptop iteration
- optional NYT Ingredient Phrase Tagger and TASTEset source probes, adapters,
  exploratory generated files, quarantine files, manifest gates, and per-source
  summaries

Implemented So Far:

1. Updated `docs/evaluation.md` to split P0 normalization from P1 food and unit
   resolution.
2. Documented the P0 case format, public source-dataset plan, metrics, proof
   gates, and buildout slices.
3. Added exported local llama source-item helpers in
   `internal/hosted/generation.go` so evaluation code can reuse the same
   deterministic source inventory as hosted prompting.
4. Added the `mealcheck eval-normalization` command entry in
   `cmd/mealcheck/main.go`.
5. Added `internal/commands/evalnormalization` with deterministic manifest
   loading, success/failure JSONL loading, source-inventory comparison, compact
   adapter validation, tag summaries, failure summaries, `-out`, and
   `-allow-mismatch`.
6. Added `scripts/generate-p0-normalization-evaluation.py` to regenerate a
   reviewed seed corpus from `examples/meal-plan-input-robustness` and
   optionally append cases from local NYT Ingredient Phrase Tagger or TASTEset
   source files.
7. Added `data/evaluation/p0-normalization/source-manifest.json`,
   `manifest.json`, `cases-v1.jsonl`, and `failure-cases-v1.jsonl`.
8. Added fixture-check validation for the P0 source manifest, manifest summary,
   success cases, failure cases, source-item IDs, supported units, and expected
   source-item counts.
9. Added `-mode local-llama` for explicit local-model baseline runs. The mode
   uses the production compact extraction prompt and adapter, reports provider
   failures, compact-output decode failures, row-match counts, and local-model
   row-match rate, and keeps the default deterministic path CI-safe.
10. Added a CLI regression test proving `mealcheck eval-normalization` writes a
   deterministic result for the seed corpus.
11. Added local-model scorer regression coverage with a static provider so the
    scoring path is tested without requiring llama.cpp.
12. Added `scripts/run-p0-local-model-regimen.sh` and the P0 live local-model
    regimen in `docs/evaluation.md` for repeatable live-model evaluation on a
    prototyping laptop. The regimen records model endpoint metadata, git
    metadata, machine metadata, deterministic baseline output, repeated
    live-model outputs, per-repeat summaries, and an aggregate gate result.
13. Added optional NYT Ingredient Phrase Tagger and TASTEset probes/adapters in
    `scripts/generate-p0-normalization-evaluation.py`. Raw third-party data is
    not checked in; local source paths can generate exploratory success,
    failure, and quarantine artifacts with source metadata.
14. Extended P0 manifests, fixture checks, and `mealcheck eval-normalization`
    with strict/exploratory gates, source-dataset filtering, per-source
    summaries, and quarantine summaries.
15. Expanded the reviewed robustness seed with paragraph and snack-inclusive
    paragraph cases so deterministic P0 covers natural meal spans as well as
    semi-structured input.

Current Seed Results:

- Total P0 cases: 14.
- Success cases: 11.
- Failure cases: 3.
- Expected source items: 139.
- Source items matched: 139.
- Source item preservation rate: 100%.
- Adapter-valid success cases: 11.
- Qualification failure cases passed: 3.
- Cases with mismatches: 0.

Remaining:

- keep the reviewed seed corpus as the strict gate until external samples are
  manually reviewed.
- add the first small NYT Ingredient Phrase Tagger and TASTEset reviewed
  subsets only after source/license review and expected-row inspection.
- run and summarize live local-model baselines on the prototyping laptop and the
  MacBook model server before treating normalization changes as production-safe.
- add result-directory artifacts for local-model raw compact output, canonical
  JSON, normalization events, timings, and repeat-run instability.

Acceptance:

- deterministic tiers run without a local model.
- success cases verify source item count, source item order, day, meal code,
  source text, food, quantity, and unit.
- adapter validation proves expected compact rows expand into canonical
  MealCheck plan JSON.
- failure cases can assert qualification or source-inventory refusal states.
- result output reports case pass rate, source-item preservation rate, adapter
  valid cases, tag summaries, and ranked failure categories.
- local-model evaluation remains explicitly separate from deterministic CI-safe
  tiers.
- local-model mode reports provider, decode, and canonical row-mismatch failure
  classes without changing default CI behavior.
- prototyping-laptop regimen records enough metadata to compare later against
  the serving MacBook run.

Verification:

- `go test ./...` passes against the current worktree.
- `go run ./cmd/mealcheck fixture-check` passes.
- `go run ./cmd/mealcheck eval-normalization -out /tmp/mealcheck-p0-normalization.json` passes.
- `go run ./cmd/mealcheck eval-normalization -mode local-llama ...` is
  available for manual runs when `MEALCHECK_LOCAL_MODEL_NAME` and a
  llama.cpp-compatible service are configured.
- `bash -n scripts/run-p0-local-model-regimen.sh` passes.
- `git diff --check` passes.

## Milestone 46: Per-Meal Local-Model Normalization Contract

Status: Implemented locally in the current worktree.

Purpose:

Keep the small local model on the critical happy path while making the model
task small enough for reliable edge-device execution. The backend should own
request gating, meal boundaries, source IDs, and reconciliation; the SLM should
own semantic parsing of bounded source spans inside one meal at a time.

Deliver:

- one-day hosted local-model input contract with strict request-level gating
- deterministic meal chunks with full meal text and globally unique source IDs
- explicit `resolved` versus `needs_model_parse` source statuses
- per-meal local-model calls with meal text as context and source items as
  authority
- compact hosted meal-chunk row schema:
  `[source_item_id, food, quantity, unit]`
- server-owned day and meal code reattachment after chunk reconciliation
- exact per-chunk source-ID validation with missing, duplicate, and extra IDs
  rejected
- reverse-measurement parsing for forms such as `chicken, 100 g`,
  `rice - 1 cup`, and `broccoli (2 cups)`
- qualification and P0 local-model evaluation routed through the same chunked
  extraction path as live runs
- hosted meal-chunk JSON Schema separated from the standalone full-row
  `local-llama normalize` compatibility schema
- docs, UI guidance, robustness fixtures, P0 generated artifacts, and tests
  updated for paragraph input and meal chunks

Implemented:

1. Added `localLlamaMealChunk`, source parse status, deterministic meal text
   capture, and globally unique source IDs in `internal/hosted/generation.go`.
2. Changed hosted local-model extraction to call the provider once per meal
   chunk, decode each chunk independently, merge rows, and validate global
   completeness before checker execution.
3. Changed the hosted prompt so the model emits only
   `[source_item_id, food, quantity, unit]`; day and meal code are deterministic
   server metadata.
4. Added paragraph and reverse-quantity source inventory support so
   `chicken, 100 g` becomes one resolved source item instead of two ambiguous
   spans.
5. Added chunk-specific compact decode and reconciliation in
   `internal/hosted/local_llama_contract.go`, including exact per-chunk
   source-ID validation and deterministic day/meal reattachment.
6. Added `LocalLlamaMealChunkResponseSchema` for hosted constrained decoding
   while preserving `LocalLlamaCompactResponseSchema` and
   `full-row-compact-meal-plan-response.schema.json` for standalone adapter
   compatibility.
7. Routed `QualifyMealPlanText` and `mealcheck eval-normalization -mode
   local-llama` through the production chunked extraction path.
8. Updated P0 generation, docs, frontend recovery copy, and robustness fixtures
   for paragraph input and the current one-day, meal-chunk contract.

Acceptance:

- hosted local-model happy path performs one model call per deterministic meal
  chunk.
- model rows do not include day or meal code.
- every chunk response covers exactly the source IDs in that chunk.
- deterministic code can preserve bounded ambiguous spans as
  `needs_model_parse` without rejecting the whole meal plan.
- reverse-measurement input such as `chicken, 100 g` is accepted as one source
  item.
- standalone local llama adapter compatibility remains available for full-row
  JSON.

Verification:

- `go test ./...` passes.
- `cd ui && npm test` passes.
- `cd ui && npm run typecheck` passes.

## Milestone 47: Planning Documentation Consolidation

Status: Implemented locally in the current worktree.

Purpose:

Keep MealCheck's planning history and active priorities authoritative after the
P0 normalization work moved from brainstorming into implementation. Completed
work should live in milestone history, active work should live in current
priorities, and evaluation procedures should live in the evaluation doc instead
of separate short-lived implementation-plan files.

Delivered:

- Recorded completed normalization-evaluation and per-meal local-model work in
  Milestones 45 and 46.
- Moved remaining P0 normalization reliability work into
  `docs/current-priorities.md`.
- Folded the P0 live local-model regimen into `docs/evaluation.md`.
- Updated the docs index, runbook, and milestone references to point to the
  focused docs.
- Deleted obsolete standalone planning docs:
  `docs/normalization-engine-improvement-plan.md`,
  `docs/p0-external-dataset-integration-plan.md`, and
  `docs/p0-live-model-regimen.md`.
- Updated `docs/decision-log.md` with the one-day per-meal hosted local-model
  contract and the documentation-consolidation decision.

Acceptance:

- no active docs index, runbook, evaluation, or procedural page points readers
  to deleted planning filenames.
- `docs/implementation-plan.md` records completed work.
- `docs/current-priorities.md` records remaining work that should still happen.
- `docs/evaluation.md` records P0 metrics, commands, seed results, and live
  regimen details.
- `docs/decision-log.md` records the accepted tradeoffs behind the current
  workflow and documentation shape.

Verification:

- reference scans for deleted planning filenames return no active references
  outside milestone and decision history.
- `bash -n scripts/run-p0-local-model-regimen.sh` passes.
- `git diff --check` passes for the updated documentation files.

## Milestone 48: Checker Evaluation Command Naming

Status: Implemented locally in the current worktree.

Purpose:

Make the CLI distinguish deterministic checker/resolver evaluation from P0
normalization evaluation. The checker evaluation command should no longer be
the generic `eval` command when `eval-normalization` also exists.

Delivered:

- Renamed the user-facing checker evaluation subcommand from `mealcheck eval`
  to `mealcheck eval-checker`.
- Kept the Go package name and import path as `evalchecker` because Go package
  names cannot use hyphens.
- Updated command dispatch, help output, CLI tests, and documentation examples.
- Updated historical milestone command references to use the current
  user-facing name.

Acceptance:

- `mealcheck eval-checker` writes the deterministic checker evaluation result.
- `mealcheck eval-normalization` remains unchanged.
- Active docs do not advertise `mealcheck eval` as the current command.

Verification:

- `go test ./cmd/mealcheck ./internal/commands/evalchecker ./internal/commands/evalnormalization` passes.
- `go run ./cmd/mealcheck eval-checker -out /private/tmp/mealcheck-eval-checker-result.json` passes.
- reference scans for `mealcheck eval` show only `eval-normalization` or
  historical/prose uses that are not the old command.

## Milestone 49: P0 Strict Gate Matches Hosted One-Day Contract

Status: Implemented locally in the current worktree.

Purpose:

Make the P0 live local-model regimen measure the current hosted product
contract. The hosted local-model path accepts one day only, but the previous P0
strict manifest still included two-day and three-day success cases from the
earlier wider scope.

Delivered:

- Ran the three-repeat P0 live local-model regimen on the serving MacBook Air
  at commit `094e8073b76a8564abe0d9bbf6ec992e661e87a6`.
- Recorded the failed pre-split live result in `docs/evaluation.md`; the gate
  failed only on `robustness_three_day_compact`, which is outside the current
  hosted one-day contract.
- Split `data/evaluation/p0-normalization/cases-v1.jsonl` into strict one-day
  success cases and
  `data/evaluation/p0-normalization/multiday-exploratory-cases-v1.jsonl`.
- Updated the P0 manifest so default `-gate strict` runs only hosted one-day
  success cases, while multi-day cases remain available under `-gate
  exploratory`.
- Updated the P0 generation script so future regenerated artifacts preserve the
  strict/exploratory split.
- Updated robustness and evaluation docs to describe the one-day strict gate.

Acceptance:

- default deterministic P0 evaluation measures the one-day hosted input
  contract.
- multi-day robustness cases remain available for local/self-hosted comparison
  without failing the hosted strict P0 gate.
- the live-server artifact path and failure class are recorded.

Verification:

- `go run ./cmd/mealcheck fixture-check` passes.
- `go run ./cmd/mealcheck eval-normalization -out /private/tmp/mealcheck-p0-strict-normalization.json` passes with 8 strict one-day success cases, 3 qualification failures, and 76 expected source items.
- `go run ./cmd/mealcheck eval-normalization -gate exploratory -out /private/tmp/mealcheck-p0-exploratory-normalization.json` passes with 3 exploratory multi-day success cases and 63 expected source items.
- `go test ./...` passes.
- `python3 -m py_compile scripts/generate-p0-normalization-evaluation.py` passes.
- `bash -n scripts/run-p0-local-model-regimen.sh` passes.
- serving MacBook strict one-day P0 live regimen passes at commit
  `8a3a395ab4712c9bf08471bfb45ef655d9c164c5` with 3 of 3 repeats, zero
  mismatches, zero provider failures, zero decode failures, minimum
  row/food/quantity/unit accuracy of 1.0, and max duration 235 seconds.

## Milestone 50: Chunk-Level Local-Model Artifacts

Status: Implemented locally in the current worktree.

Purpose:

Make the SLM-critical happy path inspectable at the same granularity as the
actual model task. Because hosted local-model extraction now calls the small
model once per deterministic meal chunk, operators need artifact evidence that
shows what each chunk sent to the model, what came back, how the compact rows
decoded, whether deterministic reconciliation changed anything, and where time
was spent.

Delivered:

- Added `optional/local-model-chunks.json` for successful hosted local-model
  runs.
- Captured per-chunk prompt messages, meal text, source item IDs, source item
  parse status, raw compact output, decoded rows, reconciliation repairs, and
  prompt/provider/decode/total timings.
- Added extraction-level chunking, expansion, completeness-check, and total
  timings.
- Embedded the same local-model extraction evidence in
  `debug/normalization-failure.json` when normalization fails after model work
  begins.
- Kept hosted production runs single-pass; repeat-run instability remains
  measured by the P0 `eval-normalization` repeat summaries rather than by
  extra production model calls.
- Updated the artifact contract, runbook, deployed local-model smoke script,
  and hosted tests for the new optional artifact.

Acceptance:

- successful hosted local-model runs list `optional/local-model-chunks.json` in
  `manifest.json`.
- the artifact contains one chunk per deterministic meal model call.
- every successful chunk records source IDs, prompt evidence, raw compact
  output, decoded rows, reconciliation result, and non-negative stage timings.
- local-model decode failures include chunk-level evidence in the debug
  normalization artifact.

Verification:

- `GOCACHE=/private/tmp/mealcheck-gocache go test ./internal/hosted -run 'TestLocalModelRunUsesServerOwnedProvider|TestLocalModelRunReportsFriendlyPostModelNormalizationFailure'` passes.
- `GOCACHE=/private/tmp/mealcheck-gocache go test ./...` passes outside the
  sandbox because hosted provider tests need local `httptest` ports.
- `bash -n scripts/test-deployed-local-model-live.sh` passes.
- `git diff --check` passes.

## Milestone 51: Public One-Day Input Contract Hardening

Status: Implemented locally in the current worktree.

Purpose:

Make the hosted local-model happy path clear at the public boundary. MealCheck
should accept one day of meal-labeled ingredient text, while rejecting food
text that is outside the CPU-local SLM contract before queueing or model
inference.

Delivered:

- Added `meal_plan_outside_hosted_contract` as a structured qualification
  status for food-related input that is outside the hosted one-day contract.
- Routed hosted local-model `/api/runs` and `/api/qualify` contract failures
  through `422 meal_plan_not_verifiable` with qualification details, instead
  of generic `400 invalid_request` responses.
- Kept deterministic preflight ordering explicit: public contract markers,
  meal-plan qualification, then source-span and source-item-cap validation.
- Updated hosted UI guidance to name the one-day meal-text contract, the
  configured source-item cap, and excluded weekly plans, recipes, grocery
  lists, and long inventories.
- Updated recovery guidance, API docs, contract docs, robustness docs, and
  current priorities to use the same public contract vocabulary.

Acceptance:

- Multi-day and weekly local-model submissions fail before queueing with
  `meal_plan_outside_hosted_contract`.
- Source inventories over `MEALCHECK_LOCAL_MODEL_MAX_SOURCE_ITEMS` fail before
  queueing with `meal_plan_outside_hosted_contract`.
- Obvious non-meal text remains a `not_meal_plan` qualification failure.
- The browser-facing form tells users the accepted shape without exposing
  parser, model-path, or compact-row internals.

Verification:

- `GOCACHE=/private/tmp/mealcheck-gocache go test ./internal/hosted -run 'TestValidateLocalModelInputContract|TestLocalModelRunRejectsClearMultiDayInputBeforeQueue|TestLocalModelRunRejectsSourceItemOverflowBeforeQueue|TestLocalModelRunFastFailsNonVerifiableTextBeforeQueue|TestQualifyEndpointReturnsQualificationForHostedLocalModelContractFailure|TestQualifyEndpointUsesHostedLocalModelWithoutClientProvider'` passes.
- `npm test` passes in `ui/`.
- `npm run typecheck` passes in `ui/`.
- `GOCACHE=/private/tmp/mealcheck-gocache go test ./...` passes outside the
  sandbox because hosted provider tests need local `httptest` ports.
- `git diff --check` passes.

## Milestone 52: Unsupported-Unit Boundary And Reverse-Measurement Corpus

Status: Implemented locally in the current worktree.

Purpose:

Finish the next P0 tightening pass without weakening the SLM-critical happy
path. MealCheck should accept product-shaped one-day text with supported
measurements, including reverse-measurement phrasing such as `chicken, 100 g`,
but reject clear unsupported portion units before queueing instead of silently
normalizing them into servings.

Delivered:

- Added `meal_plan_unsupported_units` as a first-class qualification status for
  explicit unsupported portion units.
- Wired the status through hosted local-model preflight, `/api/runs` error
  envelopes, qualification tests, UI recovery guidance, API docs, and contract
  docs.
- Updated deterministic source inventory parsing so unsupported inline or
  reverse units such as `1 bowl cereal` and `soup, 1 bowl` remain
  `needs_model_parse` source spans rather than becoming fake `serving` rows.
- Added a strict reviewed success fixture for supported reverse measurements.
- Added strict reviewed qualification-failure fixtures for unsupported inline
  and reverse portion units.
- Updated the P0 generator so regenerated failure JSONL preserves
  unsupported-unit coverage tags.
- Updated current priorities and evaluation docs with the expanded strict
  corpus: 9 one-day success cases, 5 qualification failures, and 85 strict
  expected source items.

Acceptance:

- Clear unsupported units such as bowl, plate, jar, and similar portion words
  fail before queueing with structured recovery guidance.
- Supported units still pass, including reverse-measurement wording.
- Deterministic parsing does not invent supported units for unsupported
  quantities.
- The checked-in P0 corpus remains reproducible through the generation script.

Verification:

- `go test ./...` passes outside the sandbox because app tests need local
  `httptest` ports.
- `GOCACHE=/private/tmp/mealcheck-gocache go test ./cmd/mealcheck ./internal/llm/planextract ./internal/workflow/normalize` passes.
- `GOCACHE=/private/tmp/mealcheck-gocache go run ./cmd/mealcheck fixture-check` passes.
- `GOCACHE=/private/tmp/mealcheck-gocache go run ./cmd/mealcheck eval-normalization -gate strict -out /private/tmp/mealcheck-p0-normalization.json` passes with 14 of 14 strict cases, 9 success cases, 5 qualification failures, and 85 of 85 expected source items preserved.
- `npm test` passes in `ui/`.
- `npm run typecheck` passes in `ui/`.
- `python3 -m py_compile scripts/generate-p0-normalization-evaluation.py` passes.
- `git diff --check` passes.

## Milestone 53: Source-First Local-Model Prompt Evidence

Status: Implemented and deployed.

Purpose:

Make the reverse-measurement success case stable on the serving MacBook while
keeping the SLM as the critical normalization component. The expanded strict
P0 live regimen exposed a stochastic row omission when the meal paragraph
appeared before the source inventory in the model prompt. The fix should keep
the model task semantic, but make the deterministic source inventory visibly
authoritative.

Delivered:

- Reordered the per-meal local-model prompt so authoritative source items appear
  before the context-only meal text.
- Updated prompt wording to say the context-only meal text may clarify listed
  source items but must not be used as an independent extraction source.
- Added an explicit instruction that `source_text` overrides meal-text wording
  and order.
- Updated local prompt and hosted chunk-artifact tests for the new source-first
  prompt labels.
- Rebuilt and restarted the deployed backend on the serving MacBook from commit
  `22f942679c30aa1d59a250159418cde337597e10`.
- Ran the expanded strict P0 local-model regimen on the serving MacBook and
  inspected deployed reverse-measurement chunk artifacts.

Acceptance:

- The expanded strict P0 live regimen passes on the serving MacBook with zero
  mismatches, zero provider failures, zero decode failures, and exact row,
  food, quantity, and unit accuracy.
- Deployed chunk artifacts show the source-first prompt labels.
- The reverse-measurement run preserves all source IDs. Deterministic
  reconciliation may repair source-grounded fields when all rows are present,
  but source-row omissions remain failures.

Verification:

- `GOCACHE=/private/tmp/mealcheck-gocache go test ./internal/llm/planextract` passes.
- `GOCACHE=/private/tmp/mealcheck-gocache go test ./internal/server/app` passes
  outside the sandbox because app tests need local `httptest` ports.
- `GOCACHE=/private/tmp/mealcheck-gocache go test ./...` passes outside the
  sandbox.
- Server checkout fast-forwarded to `22f942679c30aa1d59a250159418cde337597e10`.
- Deployed backend was rebuilt and relaunched as PID `23958`; `/api/health`
  returned `hosted_mode: "local_model"` with the local model ready.
- Strict P0 live regimen artifact:
  `/Users/chranama-server/MealCheck-data/p0-runs/p0-live-local-model-20260701-122257-`.
- Strict P0 live regimen result: 3 of 3 repeats completed, zero mismatches,
  zero provider failures, zero decode failures, 255 of 255 live rows matched,
  minimum row/food/quantity/unit accuracy 1.0, and max duration 272 seconds.
- Targeted deployed reverse-measurement run `run_d7e150060a2632e0911d1867`
  completed with 3 meal chunks, 9 source items, source-first prompt evidence,
  exact source-ID preservation, and 7.775 seconds of extraction timing in
  `/Users/chranama-server/MealCheck-data/p0-runs/reverse-target-20260701-123326-22f9426`.

## Milestone 54: Reviewed P1 Resolver Gap Batch

Status: Implemented locally in the current worktree.

Purpose:

Use measured P1 unresolved evidence to improve common-food coverage without
turning the resolver into fuzzy matching. The WWEIA/NHANES no-fallback result
showed high-frequency ordinary gaps that were safe to add as reviewed,
source-backed catalog rows, while leaving composed foods, alcohol,
restaurant/product-style foods, and ambiguous rows unresolved.

Delivered:

- Added eight reviewed FNDDS-backed local catalog foods:
  - tap water
  - plain carbonated water
  - instant coffee
  - apple juice
  - white sugar
  - toasted white bread
  - frozen cooked green peas
  - white rice with no added fat
- Updated the FNDDS catalog generator selection so the checked-in catalog can be
  regenerated with the same reviewed batch.
- Refreshed the WWEIA/NHANES dataset expected unresolved flags and source
  metrics to match the expanded reviewed catalog.
- Regenerated P1 result artifacts for the FNDDS-grounded strict gate,
  WWEIA/NHANES no-fallback coverage, and WWEIA/NHANES fallback coverage.
- Added a resolver regression test proving the reviewed batch resolves through
  the local catalog even when items arrive as explicit `unknown_food` rows.

Acceptance:

- Strict FNDDS-grounded P1 evaluation still has zero expected-outcome
  mismatches.
- WWEIA/NHANES no-fallback coverage improves from the previous 496 of 815 items
  without introducing expected-outcome mismatches.
- The optional FNDDS fallback remains gate-limited and is not replaced by fuzzy
  local matching.
- Remaining composed, alcohol, branded/product-style, and ambiguous rows remain
  unresolved unless separately reviewed.

Verification:

- `GOCACHE=/private/tmp/mealcheck-gocache go run ./cmd/mealcheck eval-checker -out data/evaluation/results/fndds-grounded-catalog-v1.json` passes with 885 of 900 items resolved and zero expected-outcome mismatches.
- `GOCACHE=/private/tmp/mealcheck-gocache go run ./cmd/mealcheck eval-checker -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json -out data/evaluation/results/wweia-nhanes-real-recalls-v1.json` passes with 550 of 815 items resolved and zero expected-outcome mismatches.
- `GOCACHE=/private/tmp/mealcheck-gocache go run ./cmd/mealcheck eval-checker -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite -skip-expected -out data/evaluation/results/wweia-nhanes-real-recalls-with-fndds-fallback-v1.json` passes with 774 of 815 items resolved.
- `GOCACHE=/private/tmp/mealcheck-gocache go run ./cmd/mealcheck fixture-check` passes.
- `GOCACHE=/private/tmp/mealcheck-gocache go test ./internal/workflow/checker` passes.
- `python3 -m py_compile scripts/generate-fndds-evaluation.py scripts/generate-wweia-nhanes-evaluation.py` passes.

## Milestone 55: P2 Progress, Recovery, And Local-Model Operations

Status: Implemented locally in the current worktree.

Purpose:

Complete the first P2 foundation pass for latency, progress, and capacity UX.
MealCheck should show useful product states while the MacBook-local model works,
give users concrete recovery guidance for expected failure classes, and give
operators a compact summary surface for local-model timing and failure evidence.

Delivered:

- Added a `progress` object to `/api/runs/{id}` with redacted product states for
  queued, normalizing, checking, writing report, ready, failed, and deleted
  runs.
- Redacted filesystem-like paths and key-shaped secrets from public run progress,
  legacy run error fields, and event messages returned through the API.
- Updated the live UI to render backend progress labels and backend-supplied
  recovery details while keeping raw event history available in the activity
  details panel.
- Added recovery guidance for queue-full, rate-limit, local-model unavailable,
  timeout, failed-normalization, and report-artifact loading failures.
- Returned `503 local_model_unavailable` before queueing hosted local-model runs
  when the server-owned model is not configured.
- Added `mealcheck local-model-summary` to summarize completed and failed
  local-model artifact evidence from `optional/local-model-chunks.json` and
  `debug/normalization-failure.json`.
- Persisted hosted dynamic run `input_mode` on run records so queued
  `local_model` jobs can be distinguished below worker process memory.
- Hard-enforced one active local-model run in the memory and Postgres store
  claim paths. A queued local-model run is skipped while another local-model run
  is already running; non-local queued work remains claimable.
- Extended `mealcheck local-smoke` with P2 checks for queue capacity, timeout
  failure progress, local-model unavailable responses, active local-model claim
  gating, artifact writes, and local-model summary generation.
- Documented the new local-model summary command and expanded smoke coverage in
  the CLI docs.

Acceptance:

- Users see clear progress labels instead of only raw run status values.
- Public progress and event messages do not expose provider secrets or internal
  host paths.
- Expected capacity and outage failures have distinct recovery guidance.
- Operators can summarize local-model timing, repair, decode-failure, timeout,
  source-count, chunk-count, model, and final-status data from artifact roots.
- Store-level claims prevent two local-model runs from being active at once
  across worker/process topology.
- Local smoke coverage proves queue, timeout, local-model outage, active-claim,
  artifact, and summary behavior without a live llama.cpp service.

Remaining P2 Work:

- Run `mealcheck local-model-summary` on representative hosted artifacts after
  deployments, prompt changes, and model changes.
- Use measured timing and failure data before raising local-model limits or
  comparing a larger model.

Verification:

- `go test ./...` passes.
- `go test ./internal/state/... ./internal/server/app ./internal/commands/localsmoke`
  passes.
- `npm test -- --run` passes in `ui/`.
- `npm run build` passes in `ui/`.
- `go run ./cmd/mealcheck local-smoke` passes with localhost listener access.
- `go run ./cmd/mealcheck local-model-summary -artifact-root .mealcheck-data/artifacts -format json` passes.
- `go run ./cmd/mealcheck local-model-summary` over a kept local-smoke
  local-model artifact root reports one completed run, three chunks, zero
  failed runs, zero timeouts, zero decode failures, and zero repairs.
- `git diff --check` passes.

## Milestone 56: P3 Normalized-Plan Review Pause

Status: Implemented locally in the current worktree.

Purpose:

Add the first report-trust slice between hosted local-model normalization and
deterministic checking. MealCheck should show users the normalized source-linked
rows before the checker gives those rows authority.

Delivered:

- Added `awaiting_review` run status and review events for ready, confirmed,
  rejected, and rewrite-requested review states.
- Split hosted local-model worker processing so it writes review artifacts and
  pauses after normalization instead of immediately writing the checker bundle.
- Added `/api/runs/{id}/review`, `/review/confirm`, `/review/reject`, and
  `/review/rewrite` endpoints.
- Added a source-linked normalized-plan review artifact with trust signals,
  normalized plan, and rows carrying day, meal, source text, normalized food,
  quantity, unit, and unresolved reason.
- Restored review artifacts, local-model chunks, normalization events, and
  redacted provider configuration into the final manifest after confirmation.
- Added a live UI review panel with trust-signal metrics, source-to-normalized
  rows, and `Check now`, `Rewrite input`, and `Reject` actions.
- Extended local smoke so the local-model path fetches and confirms review
  before inspecting final local-model artifacts.

Acceptance:

- Hosted local-model runs pause before deterministic checking.
- Users can inspect source-linked normalized rows before report generation.
- Confirmation runs the deterministic checker and produces the normal report
  artifact bundle.
- Reject and rewrite actions stop checker execution and leave action artifacts.
- Confirmed reports retain review artifacts in the manifest.

Verification:

- `GOCACHE=/private/tmp/mealcheck-gocache go test ./...` passes with localhost
  listener access for `httptest` packages.
- `npm test -- --run` passes in `ui/`.
- `npm run typecheck` passes in `ui/`.
- `npm run build` passes in `ui/`.
- `GOCACHE=/private/tmp/mealcheck-gocache go run ./cmd/mealcheck local-smoke --root /Users/chranama/career/MealCheck --work-dir /private/tmp/mealcheck-local-smoke-p3` passes.

## Milestone 57: P3 Completed-Report Normalization Trace

Status: Implemented locally in the current worktree.

Purpose:

Carry the normalized-plan review evidence into the completed report surface so
users can trace the final checker decision back to the same source-linked
interpretation they confirmed before checking.

Delivered:

- Extended live artifact loading to fetch optional completed-run normalization
  artifacts:
  - `review/normalized-plan-review.json`
  - `review/review-actions.jsonl`
  - `optional/local-model-chunks.json`
  - `optional/normalization-events.json`
- Added a `Normalization` report tab for completed reports with trace artifacts.
- Rendered source inventory, normalized rows, repair rows, review actions, and
  normalization events from retained artifacts.
- Matched report unresolved-food rows back to normalized-plan review rows and
  displayed source item IDs and source text when available.
- Kept static/demo reports tolerant of missing normalization artifacts.

Acceptance:

- Confirmed local-model reports expose the normalization trace after report
  generation.
- Users can inspect source text, normalized rows, repairs, unresolved reasons,
  review action history, and normalization events from the completed report.
- Unresolved foods in the report can be traced back to source-linked review
  rows when matching evidence exists.
- Reports without normalization artifacts continue to render.

Verification:

- `npm test -- --run` passes in `ui/`.
- `npm run typecheck` passes in `ui/`.

## Milestone 58: P3 Review Correction And Recommendation Trust

Status: Implemented through the current repository history.

Purpose:

Complete the current P3 trust layer by allowing users to correct normalized
rows before checking, then carrying those corrections and deterministic
recommendation evidence into completed reports.

Delivered:

- Added editable normalized-plan review rows while runs are in
  `awaiting_review`.
- Strictly validated review corrections, preserving source-item identity and
  updating only the candidate normalized plan used by `Check now`.
- Recorded correction action artifacts with source row, before value, after
  value, reason, and timestamp.
- Included correction details in completed-report normalization traces.
- Added completed-report `Recommendation` rendering for available and
  unavailable recommendation artifacts.
- Hardened recommendation artifacts so available recommendations must include
  deterministic changes, a modified plan, and a projected `pass` decision.
- Kept unsupported recommendation classes unavailable instead of exposing
  attempted edits as advice.

Acceptance:

- Users can correct model interpretation before the deterministic checker gives
  normalized rows authority.
- Correction artifacts are auditable and can later feed fixture promotion.
- Completed reports show normalization and recommendation evidence without
  trusting generative explanations.
- Recommendation rows are shown only when deterministic projection proves the
  modified plan passes.

Verification:

- Frontend report fixtures cover available and unavailable recommendation
  states.
- Backend recommendation tests cover the supported missing-vegetable edit class,
  source-plan immutability, unsupported warning classes, and hidden attempted
  edits whose projected checker result does not pass.
- `docs/current-priorities.md` now marks P3 implementation work complete for
  the current product shape.

## Milestone 59: P4 Replay, Portable Evaluation, And Operator Summaries

Status: Implemented through the current repository history.

Purpose:

Make MealCheck easier to operate, replay, and explain without expanding product
scope. The maintainer workflow should be able to reproduce the hosted
local-model path, preserve useful failure evidence, compare evaluation outputs,
and review run artifacts without reading every raw JSON file by hand.

Delivered:

- Added a focused operator walkthrough to the runbook.
- Extended the deployed local-model smoke path to treat `awaiting_review` as
  the expected midpoint, fetch review artifacts, confirm review, inspect
  completed artifacts, exercise local-model rejection policy, and verify
  deletion.
- Added a production-parity local-model deployment profile under
  `deploy/local-model/` using host-local Postgres, filesystem artifacts, a
  source-built API/worker, and a loopback llama.cpp endpoint.
- Kept Docker Postgres only as a disposable developer fallback, not the
  production-parity deployment shape.
- Added portable JSONL/CSV row exports for P0 normalization and P1 checker eval
  commands.
- Added Python operator tooling for eval export comparison.
- Added Python operator tooling for run-artifact summaries over canonical
  artifact directories.
- Added deterministic cross-run clusters and a priority queue for unresolved
  foods, source phrases, units, failure stages, timing outliers, and
  repair-heavy local-model chunks.

Acceptance:

- Operators can run one documented path through local validation, deployed
  local-model smoke, artifact capture, review confirmation, report inspection,
  and deletion.
- P0/P1 eval outputs can be compared across commits from stable structured
  exports.
- Run artifacts can be summarized into compact JSON and Markdown review queues.
- P2 model comparison remains the next pending engineering slice after the
  operator-summary prerequisite.

Verification:

- `python3 -m unittest discover -s tools/mealcheck_ops/tests` passes.

- `docs/current-priorities.md` now records no immediate P4 engineering slice
  beyond the operating loop.

## Milestone 60: P4 UI Contracts, Source Inspection, And Copy Cleanup

Status: Implemented through the current repository history.

Purpose:

Make the live product and reviewer-facing proof surface more robust by
tightening frontend/backend contracts, exposing deterministic report-source
inspection, and avoiding copy that overstates MealCheck as a broad medical,
ML-training, or model-infrastructure project.

Delivered:

- Added TypeScript runtime contract parsers for key backend workflow responses
  and runtime config.
- Added a shared report-creation preflight helper for API configuration,
  invite-code, backend health, local-model readiness, text limits, and busy
  state.
- Added completed-report source inspection that resolves decision-check source
  references to guideline citations.
- Called out missing source references and traced normalized source text to
  resolved, unresolved, and excluded food outcomes.
- Tightened public README and frontend copy so MealCheck is described as a
  bounded meal-plan check against declared constraints and source-backed
  findings.
- Fixed the frontend CI mock failure discovered during the copy cleanup pass.

Acceptance:

- Frontend code fails closer to the contract boundary when backend workflow
  responses drift.
- Completed reports let users inspect the deterministic sources behind report
  findings without introducing open-ended RAG.
- Public copy stays aligned with the verifier architecture and does not imply
  medical, model-training, or broad model-infrastructure claims.

Verification:

- Frontend contract and source-inspection tests cover the new parsing and
  report-inspection surfaces.
- The GitHub Actions frontend gate was fixed by the CI mock update.

## Milestone 61: Tooling Directory Cleanup

Status: Implemented locally in the current worktree.

Purpose:

Prevent `scripts/` from becoming a junk drawer by making it a shell-only
entrypoint directory and moving Python command surfaces into importable packages
under `tools/`.

Delivered:

- Removed all Python files from `scripts/`.
- Added `scripts/README.md` describing script ownership and runtime
  expectations.
- Moved data-generation and reference-import logic into `tools/mealcheck_data`.
- Added `python -m mealcheck_data ...` command dispatch for:
  - `generate-fndds-evaluation`
  - `generate-p0-normalization-evaluation`
  - `generate-wweia-nhanes-evaluation`
  - `import-fndds-reference`
- Added `python -m mealcheck_ops ...` command dispatch for:
  - `compare-eval-exports`
  - `summarize-run-artifacts`
- Updated active CLI/evaluation docs and checked-in generation metadata away
  from removed `scripts/*.py` commands.
- Updated Python operator tests to exercise package module commands instead of
  script wrappers.

Acceptance:

- `scripts/` contains only shell scripts plus `README.md`.
- There are no `.py`, `.pyc`, or `__pycache__` files under `scripts/`.
- Python tooling remains runnable from a source checkout through package module
  commands.
- Historical `docs/implementation-plan.md` milestone references to old
  `scripts/*.py` paths remain historical records, not current command
  instructions.

Verification:

- `PYTHONPATH=tools/mealcheck_ops/src python3 -m mealcheck_ops --help` passes.
- `PYTHONPATH=tools/mealcheck_data/src python3 -m mealcheck_data --help` passes.
- `PYTHONPATH=tools/mealcheck_ops/src python3 -m mealcheck_ops compare-eval-exports --help` passes.
- `PYTHONPATH=tools/mealcheck_ops/src python3 -m mealcheck_ops summarize-run-artifacts --help` passes.
- `PYTHONPATH=tools/mealcheck_data/src python3 -m mealcheck_data generate-p0-normalization-evaluation --help` passes.
- `PYTHONPATH=tools/mealcheck_data/src python3 -m mealcheck_data import-fndds-reference --help` passes.
- `python3 -m py_compile` over the new `mealcheck_data` package, new
  `mealcheck_ops` module entry point, and updated operator CLI passes.
- `python3 -m unittest discover -s tools/mealcheck_ops/tests` passes.

## Milestone 62: Functional Run-Lifecycle Package Boundaries

Status: Implemented locally in the current worktree.

Purpose:

Separate HTTP transport from run lifecycle behavior and reusable MealCheck
workflows so package names communicate ownership and dependency direction.

Delivered:

- Moved invite security and public access policy to `internal/access`.
- Replaced `internal/server/app` with HTTP-only `internal/server/httpapi`.
- Added `internal/runs/submission` for request validation and queue creation.
- Added `internal/runs/runinput` for expiring, non-persistent sensitive input.
- Added `internal/runs/execution` for worker and cleanup jobs.
- Added `internal/runs/review` for review corrections, artifact transactions,
  deterministic confirmation checks, and run-state transitions.
- Added `internal/runs/progress` for redacted public progress projections.
- Updated the server composition root and local smoke harness to construct the
  HTTP server, input vault, worker, and cleanup job explicitly.
- Removed the production `app` type and dependency facade.

Acceptance:

- HTTP routes and JSON contracts remain unchanged.
- Hosted run, BYOK, local-model, review, cleanup, status, and access-policy tests
  retain their behavior.
- Run packages do not import `net/http` or `internal/server/httpapi`.
- Workflow packages do not import run-lifecycle or HTTP packages.

Verification:

- `go test ./...` passes outside the restricted loopback sandbox.
- `go test -race ./...` passes outside the restricted loopback sandbox.
- `go vet ./...` passes.
- `go mod tidy -diff` reports no changes.

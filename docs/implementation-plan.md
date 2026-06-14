# Implementation Plan

MealCheck should start with a seeded, deterministic proof before live model calls
or hosted service code.

## MVP Definition

The MVP is a public, inspectable meal-plan verification demo with a constrained
live BYOK path.

The MVP should include:

- Cloudflare Pages frontend.
- MacBook-hosted backend through Cloudflare Tunnel.
- Local CLI deployment for reviewers who want to run the seeded proof without
  the hosted backend.
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

The web-deployed MVP is complete only when the above is available as a
long-standing public web deployment:

- Cloudflare Pages serves the frontend from `ui/` at a stable public URL.
- The seeded public report loads from that URL without login, provider keys, or
  a running backend.
- The frontend can call a public API hostname exposed through Cloudflare Tunnel
  and show backend health.
- The MacBook backend runs under process supervision, restarts after reboot, and
  uses Postgres plus filesystem artifact storage outside the Git checkout.
- The public API exposes only the intended HTTP surface and uses CORS limited to
  the production frontend origin.
- Live manual and BYOK runs are invite-gated, bounded by the configured queue,
  upload, timeout, and retention limits, and can be deleted by the user.
- The runbook documents deployment, start, stop, restart, health check, logs,
  tunnel status, smoke tests, backup, and cleanup commands.
- A smoke test from outside the home network can inspect the seeded report,
  check backend health, create an invite-gated live run, observe completion, and
  verify no provider keys appear in persisted artifacts.

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
- seeded report remains inspectable if backend is offline
- no secrets or live provider calls are required

Current status:

- Milestone 3 originally shipped as a no-build static frontend and is now
  superseded by the Milestone 6 Vite/React frontend
- seeded artifact bundle exists under
  `ui/public/demo-runs/seeded-3-day-peanut-allergy/`
- the frontend renders the seeded decision, check details, evidence, daily
  nutrition totals, resolved and unresolved foods, source references, and
  artifact links
- backend health state is shown as static-demo by default and can call
  `/api/health` when an API base URL is configured
- local preview now uses the Vite development server
- no frontend secrets, model calls, or backend dependency are required for the
  seeded report

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

- invite-token access gate
- OpenAI-compatible provider support
- profile-only generate-plan flow
- prompt-based generate-plan flow
- bounded JSON repair flow
- secret redaction

Implemented shape:

- `POST /api/runs` accepts `manual_structured`, `profile_generation`, and
  `prompt_generation` request bodies in addition to checked-in `case_path`
  demo runs.
- Generation modes require an `openai_compatible` BYOK provider with `model`
  and `api_key`; `https://api.openai.com/v1` is the default base URL.
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
- invite token entry that is kept out of committed config and frontend build
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
- a user with an invite token can create one manual structured run from the
  frontend against a local backend
- a user with an invite token and BYOK provider key can create one
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
   `go run ./cmd/mealcheck-fixture-check`.

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

- `go run ./cmd/mealcheck-local-smoke`
- `cd ui && npm run test:e2e:local`

Completed implementation notes:

1. Added `cmd/mealcheck-local-smoke` to build the CLI into a temporary clean
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
  examples/seeded-3-day-peanut-allergy/case.json --out
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
- provider-key and invite-token handling remains non-persistent
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

Status: Implemented on Cloudflare using a direct-upload Pages project. The
Cloudflare Tunnel, API DNS route, tunnel LaunchDaemon, Pages project, direct
Pages deployment, Pages custom domain, and production CORS pairing are active.

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
   - Git provider: `No`
8. Deployed `ui/dist` to Cloudflare Pages.
   - deployment ID: `80e4ae36-17cb-4128-8a1a-d09e97fc6818`
   - branch: `main`
   - commit: `e990d207d6d431be50bf5c4519186ae361b6ebe9`
9. Attached `mealcheck.dev` as a Pages custom domain. Cloudflare returned
   `status: active`, `validation_data.status: active`, and
   `verification_data.status: active` after the apex DNS record was added.
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
    `X-MealCheck-Invite-Token` returns `401` with `invite token required`.

Milestone 11 note:

- The original deliverable said the Pages project should be connected to the
  repository. The implemented project is a direct-upload Pages project because
  Wrangler exposed project creation and deployment but not repository
  connection. Push-to-deploy remains optional follow-up work unless the MVP
  explicitly requires Cloudflare's Git integration.

Acceptance:

- the production frontend URL loads from outside the home network
- the seeded report loads from the production frontend without backend access
- the production frontend shows backend health when the tunneled API is online
- the public API hostname serves `GET /api/health`
- live run creation is not available without the invite token
- production CORS allows the Pages origin and does not allow arbitrary browser
  origins to use the write API
- no router port forwarding is required

## Milestone 12: Public Operations And MVP Acceptance Review

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

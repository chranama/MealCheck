# Decision Log

This file records decisions that affect implementation, scope, contracts, or
public expectations.

Use this log instead of separate ADR and RFC files until a decision becomes too
large to keep readable here.

## 2026-06-10: Keep Documentation Focused

Status: Accepted

Decision:

MealCheck will use a small current documentation set:

- `README.md`
- `docs/product.md`
- `docs/user-story.md`
- `docs/nutritional-guidelines.md`
- `docs/contracts.md`
- `docs/architecture.md`
- `docs/backend_server.md`
- `docs/implementation-plan.md`
- `docs/runbook.md`
- `docs/decision-log.md`

Reason:

The project is early. Separate planning, RFC, and ADR files add navigation cost
before implementation exists.

Consequences:

- Decisions live in this log by default.
- New docs need to justify their existence against the documentation rule in
  `docs/README.md`.
- MVP user flow lives in `docs/user-story.md`.
- Guideline sources and preprocessing live in
  `docs/nutritional-guidelines.md`.

## 2026-06-10: Product Is A Verifier, Not A Meal Planner

Status: Accepted

Decision:

MealCheck's primary value is checking LLM-generated meal plans against declared
constraints and source-backed rules. Plan generation is optional.

Reason:

Generic meal-plan generation is easy to get from existing chat products. The
defensible product gap is evidence-backed verification.

Consequences:

- The seeded demo can work without live model calls.
- Reports should center check evidence, not generated prose.
- The LLM may generate, parse, or explain, but deterministic code owns the
  decision.

## 2026-06-10: Strong Claims Require A Source Of Truth

Status: Accepted

Decision:

MealCheck may only make strong judgments when the check is grounded in at least
one of:

- user-supplied constraint
- expected answer
- source material
- versioned guideline-pack rule
- trusted baseline

Reason:

The product should avoid vague health claims and unsupported nutrition advice.

Consequences:

- Reports must distinguish hard evidence from weaker guidance.
- Unresolved foods or unsupported inferences become visible warnings.
- The product should say "this plan violates the configured sodium limit," not
  "this is unhealthy."

## 2026-06-10: Guideline Packs Are Versioned Local Artifacts

Status: Accepted

Decision:

Public guideline sources will be transformed into local, versioned guideline
packs. Normal evaluation runs will use the checked-in pack snapshot rather than
scraping live sites.

Reason:

Versioned packs make reports reproducible and keep the MacBook-hosted service
stable, cheap, and auditable.

Consequences:

- Every report records the guideline pack identifier.
- Source updates require an explicit pack update.
- The initial pack is `dga-2025-2030-us-adult-general-v1`.
- The first pack should stay limited to healthy-adult general checks.

## 2026-06-10: Nutrient Totals Are Computed, Not Trusted

Status: Accepted

Decision:

MealCheck will compute nutrition totals from a nutrient catalog where possible.
It will not trust calories, sodium, macros, or other nutrient totals supplied by
the LLM.

Reason:

LLMs can produce plausible but incorrect nutrition numbers. The product's value
depends on independent verification.

Consequences:

- Meal plans need quantities and units.
- Unresolved foods are explicit warnings or blocks.
- The first seeded proof uses local fixture nutrient data.
- FoodData Central runtime lookup is post-MVP unless a later decision promotes
  it.

## 2026-06-10: MacBook Hosting Uses Fixed-Cost Constraints

Status: Accepted

Decision:

The first hosted shape uses a static frontend on Cloudflare Pages and a
MacBook-hosted backend exposed through Cloudflare Tunnel. Public users get
seeded demos. Live generation or bounded repair uses bring-your-own-key
execution.

Reason:

The project should remain fixed-cost and run comfortably on the available 2019
MacBook Air.

Consequences:

- No anonymous maintainer-paid inference.
- No local LLM inference as the primary path.
- One worker, small queue, short retention, and strict upload/runtime limits.
- If the backend is offline, the frontend can still show seeded reports.

## 2026-06-10: First Proof Scenario Is A Seeded Three-Day Plan

Status: Accepted

Decision:

The first proof scenario will compare or validate a seeded three-day meal plan
for a healthy adult with explicit constraints, including a peanut allergy,
excluded shellfish, calorie target, sodium limit, added sugar limit, and
saturated fat limit.

Reason:

This scenario is understandable to general users, but still bounded enough for
deterministic evaluation.

Consequences:

- The first implementation starts with fixtures, not provider integrations.
- The candidate fixture should include clear failures.
- Reports should show daily nutrition totals, unresolved foods, and source-pack
  citations.

## 2026-06-10: MVP Input Modes Converge On One JSON Contract

Status: Accepted

Decision:

MealCheck will support three MVP input modes:

- manual structured entry without an LLM
- profile-only LLM generation
- prompt-based LLM generation

All three modes must produce the same normalized meal-plan JSON before
verification starts.

Reason:

This keeps MealCheck's product boundary clear. The core product is the verifier,
not the LLM generation surface. Manual entry proves the checker can work without
model calls, while the LLM paths make the product easier to use.

Consequences:

- The verifier evaluates the normalized JSON artifact, not natural language.
- The frontend needs a profile step, a constraints step, and input-mode-specific
  entry or generation screens.
- Hosted BYOK is needed only for the LLM paths and bounded repair.
- Reports and artifacts can stay identical across input modes.

## 2026-06-10: Nutrition Sources Are Preprocessed Into Guideline Packs

Status: Accepted

Decision:

The first guideline pack, `dga-2025-2030-us-adult-general-v1`, will use
official U.S. public sources documented in `docs/nutritional-guidelines.md`:

- Dietary Guidelines for Americans 2025-2030
- USDA DRI Calculator
- USDA FoodData Central
- FDA Daily Values / Nutrition Facts label guidance
- FDA Food Allergies
- FoodSafety.gov

Guideline source material will be preprocessed into reviewed, versioned JSON
rules. Normal runs will not scrape or reinterpret public guideline pages.

Reason:

Preprocessed guideline packs make reports reproducible, keep the MacBook-hosted
service stable, and prevent live guideline interpretation from becoming an
unbounded or ambiguous runtime dependency.

Consequences:

- `docs/nutritional-guidelines.md` owns source selection and preprocessing.
- The MVP uses a checked-in fixture nutrient catalog for seeded demos.
- FoodData Central runtime lookup is post-MVP unless a later decision promotes
  it.
- Reports must cite the guideline pack and source rule IDs used for decisions.

## 2026-06-10: LLM Repair Is Bounded And Non-Authoritative

Status: Accepted

Decision:

When an LLM generation result is invalid JSON or has minor schema mismatches,
MealCheck may make a bounded repair attempt using the user's BYOK provider key.
Repair may fix syntax, wrapping, or minor schema shape issues. It must not invent
missing quantities, units, foods, nutrient totals, or other nutrition-critical
details.

Reason:

LLMs often produce nearly valid structured output. A bounded repair step can
improve usability without weakening the verifier's evidence model.

Consequences:

- Repair output and original LLM output must be retained in artifacts when used.
- Missing nutrition-critical fields remain unresolved rather than guessed.
- Deterministic validation still decides whether the plan can be evaluated.
- LLM output remains non-authoritative for nutrition totals and guideline
  compliance.

## 2026-06-10: Privacy And Safety Defaults Are Part Of MVP

Status: Accepted

Decision:

MealCheck will include explicit privacy and safety defaults before hosted live
runs ship. These defaults are documented in `docs/privacy-and-safety.md`.

MVP defaults:

- seeded demos require no account
- live BYOK reports are private by default
- live BYOK artifacts and metadata expire after 7 days
- provider API keys are never persisted
- profile, prompt, and meal-plan payloads are not written to application logs
- manual structured entry can run without sending data to an LLM provider
- BYOK flows must disclose third-party model-provider data transfer
- user-triggered deletion is required before public live BYOK access ships

Reason:

MealCheck handles health-adjacent profile and meal-plan data. Even when the
project is not intended to be HIPAA-covered or clinical, the product should
minimize collection, disclose third-party processing, and avoid creating
unnecessary sensitive records.

Consequences:

- Logging, artifact, and secret-redaction tests are implementation requirements.
- The database should store operational metadata, not duplicated profile details,
  unless a field is needed for queueing, deletion, or status.
- Shared reports require an explicit user action.
- The project should not claim HIPAA compliance or medical suitability.

## 2026-06-10: MVP Technical And Product Defaults

Status: Accepted

Decision:

MealCheck will use these defaults for the first implementation:

- Keep the project name `MealCheck` for now.
- Use Go for the checker engine, CLI, hosted API, worker, and cleanup job.
- Use JSON Schema contracts for cases, meal plans, guideline packs, decisions,
  reports, and artifact manifests.
- Allow Python only as an offline preprocessing helper if it is useful for
  guideline or nutrient data preparation; do not position Python as a product
  differentiator.
- Start with a hand-authored fixture nutrient catalog scoped to the seeded proof
  case. Expand toward 30 to 60 common foods later if the public demo needs it.
- Do not require live FoodData Central lookup in the MVP.
- Normalize quantities to grams internally.
- Accept `g`, `oz`, `cup`, `tbsp`, `tsp`, and `serving` in the MVP only where
  the nutrient fixture defines the conversion for that food.
- Use exact food matches plus reviewed aliases in the MVP; do not use fuzzy food
  matching until unresolved and false-positive behavior is understood.
- Use the FDA major allergen categories as the MVP allergen taxonomy: milk,
  eggs, fish, crustacean shellfish, tree nuts, peanuts, wheat, soybeans, and
  sesame.
- Treat allergen and declared-exclusion matching conservatively.
- Use user-entered calorie and protein targets first. Optional estimated targets
  can be added later, but must be labeled as estimates and overrideable.
- Make nutrient thresholds warnings by default unless the user or fixture marks
  a limit as hard.
- Block on declared allergen violations, declared excluded-food violations,
  missing required structure, and unresolved nutrition-critical quantities.
- Warn on sodium above 2,300 mg/day, saturated fat above 10 percent of calories,
  meals above the guideline-pack added sugar threshold, calories outside target tolerance,
  protein below a configured minimum, weak food-group variety, or incomplete
  prep-safety evidence.
- Require invite-token access for live BYOK runs and a separate admin credential
  for queue, cleanup, and operational controls.
- Keep live reports private by default. Sharing requires an explicit action.

Reason:

These defaults keep the first build implementable on the MacBook Air target while
preserving MealCheck's evidence model. The product should prove deterministic
verification with small, inspectable data before adding broad lookup, fuzzy
matching, account systems, or larger nutrition domains.

Consequences:

- Milestone 0 can begin with contracts, fixtures, and schemas instead of more
  product design.
- The first public demo can be deterministic and run without network access.
- The Go implementation should expose the same engine through CLI and hosted
  API surfaces.
- Broad FoodData Central search, fuzzy matching, estimated targets, full account
  management, and expanded nutrition domains remain post-MVP decisions.

## 2026-06-10: Milestone 0 Uses Seed-Scoped Fixtures

Status: Accepted

Decision:

Milestone 0 is complete with a nutrient catalog scoped to the seeded proof case,
not the broader 30 to 60 food target. The first catalog has 17 foods and exists
to exercise the schema, resolver, allergen, sodium, unit, and unresolved-quantity
paths needed by the seeded example.

MealCheck will use a native Go fixture validator:

```bash
go run ./cmd/mealcheck-fixture-check
```

The validator checks JSON Schema conformance plus cross-file integrity for case
paths, guideline pack IDs, nutrient catalog IDs, source references, source claim
references, and expected seeded failures.

Reason:

Milestone 0 should prove contracts and fixtures, not build a broad hand-authored
nutrition database. A larger fixture catalog increases maintenance cost and can
imply false precision before the resolver and FoodData Central strategy exist.

Consequences:

- The current 17-food catalog is sufficient for Milestone 0.
- Public demo credibility may still require a 30 to 60 food fixture set later.
- Formal fixture validation is part of the local development workflow.
- Bash may be added later as a thin wrapper, but durable project validation
  logic should live in Go.

## 2026-06-10: Milestone 1 Checker Core Targets Seeded Proof First

Status: Accepted

Decision:

The first checker core implements the deterministic path needed by the seeded
case before broad guideline-pack coverage. It resolves foods by exact name or
reviewed alias, normalizes supported units to grams, calculates nutrient totals
from fixture catalog values, runs the first check set, and aggregates the final
decision.

The checker rejects unknown fields in loaded plan JSON. This means
LLM-supplied nutrition totals are flagged instead of trusted.

Reason:

The value of the first implementation is proving the evidence path end to end.
Serving-count rules and detailed food-safety numeric rules are encoded in the
guideline pack, but implementing every advisory rule before the resolver is
stable would blur Milestone 1 into a broader rules engine.

Consequences:

- `go test ./...` is now the primary seeded checker verification command.
- The seeded candidate produces a `block` decision.
- Unresolved quantities are visible in evaluation output.
- Detailed DGA serving-count checks and FoodSafety temperature/time checks remain
  future checker expansion work.

## 2026-06-10: Milestone 2 CLI Writes The Shared Artifact Contract

Status: Accepted

Decision:

MealCheck's first local CLI surface is:

- `mealcheck validate`
- `mealcheck compare`
- `mealcheck decision`

`validate` and `compare` write the same artifact bundle shape documented in
`docs/contracts.md`. `decision` reads an existing `decision.json` and applies the
same exit-code policy.

The seeded `compare` command records `compare` mode in `manifest.json` but uses
the same deterministic evaluation path as `validate` for Milestone 2.
Baseline-specific regression reporting can expand later without changing the
external command shape.

Reason:

The hosted API should wrap the same checker and artifact writer used locally.
Proving the bundle and exit-code policy in the CLI first gives the future
frontend and backend a stable contract.

Consequences:

- `go run ./cmd/mealcheck validate --case <case> --out <dir>` is the primary
  local seeded run command.
- The seeded fixture exits `1` because it intentionally produces a `block`
  decision.
- Warnings continue to exit `0` unless `--strict` is used.
- Invalid configuration, unreadable decisions, or unusable artifacts exit `2`.
- The artifact writer refuses to use the repository root as the output
  directory.

## 2026-06-10: Milestone 3 Uses A No-Build Static Frontend

Status: Superseded by `2026-06-11: Milestone 6 Moves Live Frontend To Vite/React`

Decision:

The first public demo frontend lives under `ui/` as plain static HTML, CSS, and
JavaScript. It reads checked-in seeded artifacts from `ui/demo-runs/` and can be
deployed directly by Cloudflare Pages with no build command.

The frontend shows backend health only when an API base URL is configured. The
seeded report path remains fully inspectable without a backend.

Reason:

The immediate goal is to prove the product and artifact contract, not frontend
framework complexity. A no-build static frontend is cheap to host, easy to
inspect, and stays available even when the MacBook-hosted backend is offline.

Consequences:

- `ui/` is the Cloudflare Pages static root for the first demo.
- Seeded artifacts are committed for public offline inspection.
- No model provider keys, backend secrets, tunnel credentials, or live calls are
  present in the frontend.
- A frontend framework can be introduced later only if the manual entry or live
  BYOK flows justify it.

## 2026-06-10: Milestone 4 Hosts The Existing Checker Behind A Small API

Status: Accepted

Decision:

MealCheck's first hosted wrapper is a Go HTTP server in
`cmd/mealcheck-server`. It runs the API, one worker, and cleanup loop in one
process. It uses Postgres for run metadata and queue state when `DATABASE_URL`
is configured, and filesystem storage for artifact bundles.

The API binds to `127.0.0.1:8080` by default for Cloudflare Tunnel exposure. A
memory store exists for tests and local development, but the intended hosted
store is Postgres.

Milestone 4 run creation accepted checked-in case paths and executed the
existing checker/artifact writer. BYOK provider generation and JSON repair were
assigned to Milestone 5.

Reason:

This proves the hosted shape without creating a second evaluation engine or
adding provider complexity too early. One process is adequate for the MacBook
Air target and keeps operational setup small.

Consequences:

- The backend can serve seeded reports and generated run artifacts.
- Queue size, one-active-run worker behavior, timeout, upload limit, and
  retention are enforced in code.
- Expired run artifacts are deleted by cleanup.
- The initial Postgres schema is applied at server startup.
- Future BYOK work should reuse the same run, event, artifact, and cleanup
  contracts.

## 2026-06-10: Milestone 5 Uses In-Memory BYOK Input And Deterministic Judging

Status: Accepted

Decision:

MealCheck supports three hosted live input modes: `manual_structured`,
`profile_generation`, and `prompt_generation`. Manual structured input requires
a normalized meal-plan JSON object and does not require a provider. Generation
modes require an `openai_compatible` BYOK provider with a user-supplied model
and API key.

Provider credentials are held only in a shared in-memory pending-input map
between run creation and worker claim. They are not written to Postgres, report
artifacts, runtime case files, or logs. The worker writes only redacted provider
metadata to `configs/redacted-provider.json`.

Remote LLM output can generate the candidate plan or make one bounded JSON
repair attempt. It cannot make the nutrition decision. The local deterministic
checker remains the source of truth for guideline checks, resolved foods,
unresolved foods, decision, and report artifacts.

Reason:

This preserves the fixed-cost hosting model: the server operator is not paying
for user inference, and the MacBook only runs bounded parsing, normalization,
evaluation, and artifact writing. In-memory provider credentials are simpler and
safer than durable encrypted job payloads for the early invite-only backend.

Consequences:

- Server restarts can strand queued BYOK jobs because pending provider input is
  intentionally not durable.
- Live BYOK flows require an invite token before they can trigger provider
  calls.
- One repair attempt is allowed by default for generation modes, but repair
  prompts must preserve missing or vague nutrition-critical details as
  unresolved.
- Optional artifacts can include provider output and normalization events, but
  API keys must not appear in artifact bundles.

## 2026-06-11: MVP Requires Long-Standing Web Deployment

Status: Accepted

Decision:

MealCheck's MVP acceptance criteria include web deployment, not only local code
or local backend behavior. The accepted MVP must have a public Cloudflare Pages
frontend, a MacBook-hosted API exposed through Cloudflare Tunnel, process
supervision for the backend and tunnel, Postgres-backed metadata, filesystem
artifact storage outside the Git checkout, and documented operational commands.

The public frontend must remain useful when the backend is offline by showing
seeded reports without login, provider keys, or paid inference. Live manual and
BYOK runs must be invite-gated, bounded, deletable, and private by default.

Reason:

The portfolio value is not just the checker engine. It is the full-stack shape:
a low-cost static frontend, a constrained personal backend, evidence artifacts,
and a fixed-cost-friendly BYOK model that can be inspected by someone outside
the development machine.

Consequences:

- MVP acceptance now requires public smoke tests from outside the home network.
- The runbook must include concrete deployment, service, tunnel, health, log,
  cleanup, backup, and deletion commands before the MVP is accepted.
- The frontend must grow beyond seeded report viewing if the first public live
  path is expected to be usable without hand-written API calls.
- CORS, invite-token configuration, retention, and secret redaction become
  deployment acceptance criteria, not only implementation details.

## 2026-06-11: Milestone 6 Moves Live Frontend To Vite/React

Status: Accepted

Decision:

MealCheck's live frontend will use a small Vite/React app under `ui/` beginning
in Milestone 6. The app still builds to static assets for Cloudflare Pages and
does not add server-side rendering, frontend server runtime, or serverless
functions.

The React UI owns browser state for the app shell, seeded report viewer,
profile and constraints forms, manual/profile/prompt input modes, BYOK
disclosure, run status, report tabs, and artifact links. The backend API
contract, artifact contract, BYOK cost model, and MacBook-hosted backend shape
remain unchanged.

Reason:

The live-run workflow now has enough state that hand-written DOM mutation is
becoming a credibility and maintainability risk. Vite/React gives the frontend
clear component boundaries for the workflow without undermining the fixed-cost
hosting goal, because production output is still static.

Consequences:

- `ui/` now has an npm build step for the frontend.
- Cloudflare Pages should run the Vite build and publish `ui/dist`.
- Seeded artifacts move under `ui/public/demo-runs` so Vite copies them into
  the deployed static site.
- The public API base URL is safe to expose through static config or a Vite
  public environment variable; secrets still must never be embedded in frontend
  code or build output.
- A larger frontend framework, router, component library, SSR layer, or account
  dashboard remains out of scope until real workflow complexity justifies it.

## 2026-06-11: Frontend Hardening Uses LLMEP UI Patterns

Status: Accepted

Decision:

MealCheck will reuse selected architectural patterns from
`llm-extraction-platform`'s UI as part of Milestone 6 frontend hardening:

- TypeScript with strict checking for UI contracts and component props.
- A central API client for URL joining, typed endpoint wrappers, and consistent
  backend error formatting.
- Runtime public config loaded from `/config.json`, with `?api=` retained for
  local development overrides.
- Feature-oriented React modules instead of a single large entrypoint.
- Test factories plus Vitest and Playwright coverage for UI contracts, payload
  builders, report rendering, and live-run flows.

MealCheck will not copy LLMEP's admin-console shape, playground-first product
model, embedded `VITE_API_KEY` pattern, or broader endpoint surface. MealCheck
provider keys and invite tokens remain user-entered runtime secrets and must not
be placed in frontend config or build output.

Reason:

The current React frontend proves the local live workflow, but the single-file
implementation is already carrying API calls, form state, report rendering,
artifact URL construction, polling, deletion, and BYOK secret handling. LLMEP's
UI demonstrates a more maintainable boundary: typed API contracts, runtime
config, feature modules, and reusable frontend tests.

These patterns improve credibility by making the UI testable and easier to
audit without changing the fixed-cost hosting shape.

Consequences:

- Milestone 6 is not fully closed until the frontend is converted to
  TypeScript, split into feature modules, and covered by basic unit/component
  and mocked e2e tests.
- `npm run typecheck`, `npm test`, `npm run test:e2e`, and `npm run build`
  become frontend acceptance commands once the hardening work is implemented.
- The deployed frontend remains static Cloudflare Pages output.
- Backend JSON Schemas remain the runtime source of truth; TypeScript types are
  guardrails for frontend development and API wiring.
- Runtime config can expose public API origin and feature flags only; secrets
  remain excluded from frontend source, config, storage, reports, artifacts, and
  build output.

## 2026-06-11: Milestone 6 Frontend Hardening Is Locally Accepted

Status: Accepted

Decision:

Milestone 6 is accepted for local development/prototyping scope. The Vite UI is
now TypeScript-based, split into feature modules, backed by a central API
client and runtime config loader, and covered by unit/component tests plus
mocked Playwright flows.

The accepted browser flows are:

- seeded report loading without a backend
- manual structured run creation
- live run deletion
- BYOK profile-generation run creation
- BYOK prompt-generation run creation
- provider-key non-persistence in rendered page text, localStorage, and post-run
  form state

Reason:

The remaining frontend credibility risk in Milestone 6 was not visual styling;
it was maintainability and verification. The implementation now has typed UI
contracts, isolated API/payload/config boundaries, and repeatable frontend tests
that do not require a live Go backend or a model provider.

Consequences:

- Deployment-server configuration, public hosting, Cloudflare Pages/Tunnel
  wiring, process supervision, and public smoke tests move to later milestones.
- Frontend acceptance commands are `npm run typecheck`, `npm test`,
  `npm run test:e2e`, and `npm run build`.
- The production frontend shape remains static Cloudflare Pages output.
- The Go backend and CLI remain the runtime source of truth for validation and
  artifact generation.

## 2026-06-11: Milestone 7 Local Acceptance Uses Deterministic Smoke Harnesses

Status: Accepted

Decision:

Milestone 7 is accepted for local development/prototyping scope. Local
acceptance is demonstrated by:

- `go run ./cmd/mealcheck-local-smoke`
- `cd ui && npm run test:e2e:local`

The Go smoke command builds the CLI in a temporary clean build directory,
verifies the seeded `block` exit policy, exercises invite-gated manual and
fake-BYOK hosted runs, checks run events/report/artifact listing/deletion,
verifies allowed and disallowed CORS behavior, and scans runtime outputs for the
fake provider key.

The local Playwright suite starts the real Go backend with memory storage and
the Vite frontend. It verifies seeded report viewing without an API base,
manual run creation/deletion against the real backend, BYOK generation through a
fake provider response, provider-key non-persistence, artifact redaction, and
CORS headers.

Reason:

Milestone 7 needed repeatable local proof that the full stack works before
MacBook service configuration or public hosting begins. Deterministic smoke
harnesses keep that proof fixed-cost, offline-friendly, and independent of a
real model provider key.

Consequences:

- `MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH` exists only for local smoke tests and
  must not be set in the deployed MacBook service.
- CORS now returns allow headers only when the request `Origin` exactly matches
  `MEALCHECK_ALLOWED_ORIGIN`.
- Deployment packaging, process supervision, Cloudflare Pages/Tunnel setup, and
  public smoke tests remain later milestones.

## 2026-06-11: Public Manual Entry Stays Seed-Catalog Scoped For Now

Status: Accepted

Decision:

The first public manual-entry UI remains limited to the existing 17-food fixture
catalog used by the seeded proof. Milestone 7 does not expand the nutrient
catalog.

Reason:

The existing catalog is enough to prove the manual live-run path, CORS,
redaction, deletion, and report rendering. Expanding the catalog before a
reviewed food-source strategy would imply broader nutrition coverage than the
system can honestly support.

Consequences:

- The frontend manual food picker remains narrow and explicit.
- Broader manual entry requires a later reviewed catalog expansion or FoodData
  Central strategy.
- The public product should describe unresolved foods and catalog scope clearly
  rather than presenting MealCheck as a broad nutrition database.

## 2026-06-11: Milestone 8 Uses Source-Build Deployment Templates

Status: Accepted

Decision:

MealCheck's MVP deployment package uses source-build deployment for the CLI and
backend server instead of release binaries. The accepted build commands are:

```bash
go build -o bin/mealcheck ./cmd/mealcheck
go build -o bin/mealcheck-server ./cmd/mealcheck-server
```

The proposed MacBook runtime layout is:

- runtime user: `chranama-server`
- repository: `/Users/chranama-server/MealCheck`
- data path: `/Users/chranama-server/MealCheck-data`
- artifact path: `/Users/chranama-server/MealCheck-data/artifacts`
- log path: `/Users/chranama-server/MealCheck-data/logs`
- environment file:
  `/Users/chranama-server/MealCheck-data/mealcheck-server.env`
- Postgres database and role: `mealcheck`
- backend launchd label: `dev.mealcheck.server`
- Postgres launchd label: `dev.mealcheck.postgres`
- Cloudflare Tunnel name: `mealcheck-api`
- production frontend URL: `https://mealcheck.dev`
- production API URL: `https://api.mealcheck.dev`

The deployment package lives under `deploy/` and contains placeholder-only
secrets and machine-local values for the MacBook environment file, `launchd`,
Postgres setup, Cloudflare Tunnel, Cloudflare Pages settings, and frontend
runtime config.

Reason:

The MVP targets one known MacBook Air and is still early. Source builds keep
the operational path understandable, avoid release-binary ceremony, and allow
the deployment package to be verified locally before real server setup begins.

Consequences:

- Release binaries are deferred until there are multiple deployment targets or
  a public download story.
- Real secrets and tunnel credentials must be supplied only during MacBook and
  Cloudflare configuration.
- Milestone 10 applies these templates on the MacBook.
- Milestone 11 configures Cloudflare Pages and Tunnel for the accepted
  production hostnames.

## 2026-06-11: Milestone 9 Hardens The Static Web Design

Status: Accepted

Decision:

MealCheck adds a dedicated Web Design Hardening milestone before MacBook and
Cloudflare deployment. The frontend remains a static Vite/React build, but the
homepage is now designed as a live-run verification console rather than a
seeded-demo report page.

The design contract lives in `docs/web-design.md` and sets the static frontend
constraints, visual system, page anatomy, component rules, accessibility rules,
and acceptance criteria.

Reason:

The functional frontend proved the workflows, but MVP web acceptance also
requires a reviewer to understand the product without maintainer explanation.
Design hardening before public deployment reduces the risk of shipping a
technically working page that feels unclear, demo-first, or visually unfinished.

Consequences:

- The live-run workflow is the homepage.
- Seeded demos remain public proof artifacts reachable from navigation.
- MacBook service configuration moves to Milestone 10.
- Cloudflare Pages and Tunnel deployment moves to Milestone 11.
- Public operations and MVP acceptance review moves to Milestone 12.

## 2026-06-14: Milestone 10 Uses LaunchDaemon For Backend Supervision

Status: Accepted

Decision:

MealCheck's MacBook backend should run as a system `LaunchDaemon` with
`UserName` set to `chranama-server`, label `dev.mealcheck.server`, localhost
binding on `127.0.0.1:8080`, and logs under
`/Users/chranama-server/MealCheck-data/logs`.

The repository keeps only the accepted backend server launchd template:

- `deploy/macos/dev.mealcheck.server.plist.template` for the Milestone
  10 server mode.

The earlier user `LaunchAgent` template was removed because it had the same
label as the production daemon, did not model before-login startup, and could
lead to two launchd domains competing for `127.0.0.1:8080`.

Reason:

This MacBook is the long-running backend server. A user `LaunchAgent` was
useful during early local validation, but it starts only when the
`chranama-server` GUI session is available. A system `LaunchDaemon` is the
correct production supervision mode for unattended reboot recovery while still
running the service as the least-privileged runtime user.

Consequences:

- Server acceptance uses the `LaunchDaemon` installed and verified after
  restart or reboot.
- Any leftover user `LaunchAgent` must be unloaded and removed before final
  daemon acceptance, or both launchd domains may compete for `127.0.0.1:8080`
  after the next login.
- The MacBook must keep idle system sleep disabled on AC power so launchd
  supervision actually matters for long-running service availability.

## 2026-06-14: Milestone 10 Uses A Non-Root Postgres LaunchDaemon

Status: Accepted

Decision:

The MacBook deployment should not use Homebrew's generated system
`homebrew.mxcl.postgresql@17` daemon for the MVP Postgres service. Instead,
MealCheck installs `deploy/macos/dev.mealcheck.postgres.plist.template` as
`/Library/LaunchDaemons/dev.mealcheck.postgres.plist`. The daemon starts at
boot but uses `UserName` set to `chranama-server`, with the data directory at
`/usr/local/var/postgresql@17` and logs at
`/usr/local/var/log/postgresql@17.log`. It also sets `LC_ALL` and `LANG` to
`en_US.UTF-8`.

Reason:

Running `sudo brew services start postgresql@17` created a system daemon that
tried to execute Postgres as root. Postgres rejects root execution, so the
daemon repeatedly exited with code 1 and the MealCheck backend could not start
after a restart. A project-owned LaunchDaemon keeps before-login startup while
honoring Postgres's requirement to run under an unprivileged user. Postgres
also failed under launchd with `postmaster became multithreaded during startup`
until the daemon provided an explicit valid locale.

Consequences:

- The broken Homebrew system Postgres daemon must be unloaded and removed from
  `/Library/LaunchDaemons` before installing `dev.mealcheck.postgres`.
- The backend `dev.mealcheck.server` daemon depends on local Postgres being
  available; after changing Postgres supervision, restart the backend daemon.
- Future Postgres upgrades may require checking the binary path in
  `dev.mealcheck.postgres.plist.template`.

## 2026-06-14: Launchd Labels Use The `mealcheck.dev` Reverse DNS Namespace

Status: Accepted

Decision:

MealCheck launchd labels and deployment plist template filenames should use
the reverse-DNS namespace for the accepted production domain:

- backend: `dev.mealcheck.server`
- Postgres: `dev.mealcheck.postgres`

Reason:

Launchd labels conventionally use reverse-DNS ownership. MealCheck uses
`mealcheck.dev` as the accepted production domain, so `dev.mealcheck.*` is the
correct namespace. Earlier labels worked technically but implied a different
domain namespace and made the deployment identity less clear.

Consequences:

- The readiness script and runbook inspect `system/dev.mealcheck.postgres` and
  `system/dev.mealcheck.server`.
- Future launchd-managed services for MealCheck should use the same
  `dev.mealcheck.*` prefix.

## 2026-06-14: Milestone 11 Uses A Tunnel LaunchDaemon And Direct Pages Upload

Status: Accepted

Decision:

The Cloudflare Tunnel connector should run under a third system
`LaunchDaemon`, `dev.mealcheck.tunnel`, using the MacBook-local config
`/Users/chranama-server/.cloudflared/mealcheck-api.yml`. This keeps the public
API route alive after restart without router port forwarding.

The first Cloudflare Pages deployment was created as a direct-upload project
named `mealcheck` and deployed from `ui/dist` with Wrangler. The project is not
Git-connected; Cloudflare reported `Git Provider: No`. Direct upload is
accepted for the MVP because it produces the required public frontend without
adding GitHub dashboard coupling to the deployment path.

Reason:

The tunnel is now production infrastructure, not a foreground proof command, so
it needs the same before-login supervision model as Postgres and the backend.
Wrangler can create and deploy a Pages project directly, which was enough to
make the frontend live. Wrangler does not expose repository connection or
custom-domain DNS management commands in the installed version, so those steps
must be completed through the Cloudflare dashboard or API.

Consequences:

- The accepted launchd labels are now `dev.mealcheck.postgres`,
  `dev.mealcheck.server`, and `dev.mealcheck.tunnel`.
- The public API hostname `api.mealcheck.dev` routes through tunnel
  `mealcheck-api`
  (`e8cbd8da-735a-4053-9503-880f636670f6`) to `127.0.0.1:8080`.
- `mealcheck.pages.dev` and `mealcheck.dev` serve the uploaded production
  frontend bundle.
- The apex DNS record `CNAME @ -> mealcheck.pages.dev` is active through
  Cloudflare.
- Push-to-deploy through Cloudflare's Git integration can be added later, but
  it is not required for MVP acceptance.

## 2026-06-12: Frontend Defaults To Client-Facing Report Language

Status: Accepted

Decision:

MealCheck's public static frontend should hide internal system detail from the
default client-facing path. The live-check surface should not expose standalone
service-status cards, service URL fields, run IDs, event counts, pipeline
graphics, or raw artifact browsers unless those details are deliberately placed
behind troubleshooting or activity disclosures.

The report surface should prioritize meal-plan verification results over
internal contract validation. Checks shown by default are meal-plan checks, not
schema, manifest, or artifact contract checks. Evidence should be rendered in
natural language. Guideline packs, source claims, and field names should use
human-readable labels instead of raw identifiers. The report download surface is
one `Report` tab with one shareable PDF, not a list of implementation artifacts.

MealCheck's visual identity should remain static-safe and code-native for the
MVP: a streamlined `M` plus check mark brand mark, static-safe type choices, and
brand/evidence colors that are distinct from pass, warn, and block status
colors.

Reason:

The product is a verifier for people evaluating a meal plan, not an internal
pipeline dashboard. Exposing service plumbing, contract checks, raw JSON
evidence, or artifact bundles makes the MVP harder to understand and weakens
trust in the report. Static-safe visual identity rules keep the Cloudflare Pages
deployment simple while giving the product a recognizable first impression.

Consequences:

- Internal workflow state belongs in activity details, logs, tests, and
  developer documentation, not in the first client-facing viewport.
- Reports should use outcome-first language, natural evidence, and
  human-readable source labels.
- Raw artifacts remain available through the backend/API contract, but the
  primary web UI presents a single report PDF.
- Future UI changes should preserve the separation between brand/evidence color
  and semantic status colors.

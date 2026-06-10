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

Status: Accepted

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

Milestone 4 run creation accepts checked-in case paths and executes the existing
checker/artifact writer. BYOK provider generation and JSON repair stay in
Milestone 5.

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

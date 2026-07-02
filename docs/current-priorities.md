# Current Priorities

This document is MealCheck's active engineering-priority surface. The
implementation plan records milestone history; this document ranks the next
work by user impact, observed failure modes, and operational value.

Last reviewed: 2026-07-01.

## Prioritization Rule

Prioritize work by:

```text
priority = user impact x observed frequency x confidence - implementation cost
```

User impact means the degree to which a failure makes a reasonable user lose
trust in MealCheck. Observed frequency should come from local testing, Chrome
live-run inspection, debug artifacts, robustness fixtures, deployed smoke tests,
or production logs. Confidence means how sure we are that the proposed change
will reduce the failure. Implementation cost includes code complexity,
operational risk, and risk to the deterministic trust boundary.

Correct `block` decisions are the product working. The highest-priority failure
class is a reasonable in-bound meal plan failing for an arbitrary, opaque, or
self-inflicted reason before the verifier can do useful work.

## How To Read The Priority Sections

Each priority area separates three different kinds of work:

- Current implementation state: what is already built and should remain stable
  until new evidence changes the priority.
- Discrete engineering work now: concrete implementation tasks that should be
  scheduled immediately.
- Ongoing operating loop: recurring review, corpus, evaluation, and monitoring
  work that keeps the priority healthy as a standing loop.

When a priority is in operating-loop mode, use observed failures, evaluation
gaps, or missing operator tooling to promote an operating-loop item into an
implementation slice.

## Product Boundary

New work should preserve this run path:

```text
concise meal-plan text
  -> bounded local-model normalization
  -> source-linked normalized plan
  -> deterministic food, nutrient, allergen, and guideline checks
  -> report artifacts
  -> repeatable validation
```

New architecture work should strengthen the verifier path, run inspection,
replay, and recovery. Go remains the authoritative service path for validation,
run state, artifacts, privacy policy, and report contracts. Additional runtimes
own distinct product or operational jobs:

- Python owns evaluation generation, result exports, run-artifact analysis,
  unresolved-item clustering, model-comparison summaries, and other data-heavy
  operator tools.
- TypeScript owns frontend contracts, UI state, runtime config validation,
  typed API clients, and narrowly scoped edge or backend-for-frontend adapters
  when they reduce frontend/backend friction.
- Secondary services consume explicit contracts or artifacts from the
  authoritative verifier.

The best near-term improvements are therefore:

- make model normalization inspectable and correctable by users
- turn existing timing and chunk artifacts into compact operator summaries
- compare local-model quality and latency after artifact summaries exist
- make the deployed workflow reproducible enough to debug outages and release
  changes
- export evaluation summaries in a portable format from canonical verifier
  outputs
- keep Python and TypeScript work attached to evaluation, operations, UI, or
  edge-integration needs
- add source inspection when it helps users understand completed reports and
  preserves deterministic source-pack authority

## P0: In-Bound Normalization Reliability

Goal:

MealCheck should reliably normalize concise ingredient-level meal plans that
match the public input guidance. The preloaded example and a small regression
corpus of natural rewrites must work before broader feature work gets priority.
The immediate release condition is that the new one-day, per-meal,
SLM-critical path works on the serving MacBook with enough artifacts to explain
failures.

Why this is first:

- Normalization is the gateway to every hosted live report.
- Failures here make the whole product feel broken, even when the deterministic
  checker is correct.
- Recent live issues were concentrated here: hidden day/meal-count constraints,
  unsupported units, natural phrasing, and local-model representation mismatch.

Good enough:

- The reviewed robustness corpus succeeds through the hosted per-meal
  local-model path on the serving MacBook and in deterministic or
  prototyping-laptop tests.
- The preloaded hosted example succeeds locally and on the deployed path.
- The acceptable-input robustness corpus succeeds for normalization correctness,
  independent of whether the final decision is `pass`, `warn`, or `block`.
- Each accepted model chunk has enough redacted evidence to debug prompt,
  source-inventory, model-output, decode, reconciliation, and timing failures.
- Common natural rewrites of the example either normalize successfully or fail
  before queueing with specific, actionable guidance.
- Source item preservation is exact: every resolved source item appears once.
- Every default day-count and meal-count setting is deliberately exposed to the
  user.

Current evidence:

- The serving MacBook strict one-day P0 live regimen passed on 2026-07-01 at
  commit `22f942679c30aa1d59a250159418cde337597e10`: 3 of 3 repeats, zero
  mismatches, zero provider failures, zero decode failures, 255 of 255 live
  rows matched, and minimum row/food/quantity/unit accuracy of 1.0. The artifact
  directory is
  `/Users/chranama-server/MealCheck-data/p0-runs/p0-live-local-model-20260701-122257-`.
- Hosted local-model runs now write `optional/local-model-chunks.json` with
  per-meal prompt messages, source IDs, raw compact output, decoded rows,
  reconciliation repairs, and stage timings. Post-model normalization failures
  embed the same chunk evidence in `debug/normalization-failure.json`.
- The public hosted contract is now explicit across the API, UI, and docs:
  one day of meal-labeled ingredient text, up to the configured source-item
  cap, with weekly plans, multi-day plans, recipes, grocery lists, and long
  inventories failing before model inference with structured qualification
  guidance.
- The strict deterministic P0 corpus now covers first-class unsupported-unit
  qualification failures and supported reverse measurements such as
  `chicken, 100 g`. It passes with 14 of 14 strict cases, 9 accepted one-day
  success cases, 5 qualification failures, and 85 of 85 expected source items
  preserved.
- A deployed reverse-measurement run on 2026-07-01 completed through the
  refreshed backend with 3 meal chunks, 9 source items, exact source-ID
  preservation, and the source-first prompt visible in chunk artifacts. Breakfast
  and lunch decoded directly; dinner required 11 deterministic
  source-measurement repairs but preserved every source ID and produced the
  correct normalized rows. The collected artifact directory is
  `/Users/chranama-server/MealCheck-data/p0-runs/reverse-target-20260701-123326-22f9426`.

Current implementation state:

The main P0 implementation slices for the current product shape are complete:
the hosted one-day contract is explicit, normalization runs through the
per-meal local-model path, source preservation is checked, chunk-level model
artifacts exist, and the serving MacBook live regimen has passed. Treat P0 as a
reliability and regression discipline until new evidence shows a reasonable
in-bound input failing for an arbitrary, opaque, or self-inflicted reason.

Discrete engineering work now:

P0 implementation work is trigger-based. Promote work into P0 when one of these
signals appears:

1. An observed in-bound natural rewrite passes the public input contract but
   fails normalization, source preservation, decode, or report generation.
2. Chunk artifacts show recurring omissions, duplicates, decode failures, or
   source-measurement repairs that exceed source-preserving cleanup.
3. A user-facing qualification or failure message exposes model/parser internals
   or gives guidance that is too vague to recover from.
4. Adding and reviewing P0 regression cases becomes manual enough that a small
   corpus or artifact-summary tooling slice would pay for itself.

Ongoing operating loop:

1. Add observed in-bound natural rewrites to the reviewed corpus before using
   broad generated external data as a release signal.
2. Keep deterministic unit normalization conservative: normalize supported
   units when the conversion is visible, and handle vague or genuinely
   unsupported quantities through preservation, rejection, and recovery guidance.
3. Watch repair-heavy reverse-measurement chunk artifacts for recurrence and
   preserve the evidence when repair remains source-preserving.
4. Keep user-facing failure messages guidance-oriented, with compact row,
   schema, model-path, and parser details reserved for artifacts.
5. Generate and manually review small NYT Ingredient Phrase Tagger and TASTEset
   exploratory slices after source/license review and after the product-shaped
   reviewed corpus has exposed the obvious gaps; promote external subsets to
   strict after expected rows and failure categories have been inspected.

Metrics to track:

- `preloaded_example_success`: must be 100%.
- `normalization_success_rate` on the acceptable-input corpus.
- `source_item_preservation_rate` on normalized runs.
- `unsupported_unit_false_failure_count` from observed in-bound inputs.
- `post_queue_normalization_failure_count` for inputs that passed preflight.
- `local_model_chunk_decode_failure_count` by meal code and source-item count.
- live P0 row, food, quantity, and unit accuracy on the serving MacBook model.
- chunk artifact completeness for accepted local-model runs.
- per-stage duration for qualification, chunk extraction, checker execution,
  artifact writing, and report loading.

## P1: Common Food And Unit Resolution

Goal:

Ordinary foods and ordinary measured portions should resolve often enough that a
reasonable report feels useful, while unresolved items remain visible when
MealCheck would otherwise need to guess.

Why this is second:

- A report with too many ordinary unresolved foods feels pedantic and loses
  trust.
- The resolver now has a measured evaluation framework and an optional FNDDS
  fallback, so improvements can be targeted from evidence.
- Resolution work follows normalization reliability because the resolver
  operates after the plan becomes canonical MealCheck JSON.

Good enough:

- Strict FNDDS-grounded evaluation continues to pass with zero expected-outcome
  mismatches.
- Common user-facing gaps are addressed through reviewed source-backed batches
  and recovery copy.
- Household unit conversions are added when source-backed or policy-reviewed
  for that food.
- Ambiguous, mixed-dish, branded, vague, unclear-preparation, unsupported-unit,
  and non-food entries remain unresolved with specific reasons.

Current evidence:

- The reviewed local catalog now contains 159 foods. The latest P1 batch added
  exact source-backed coverage for tap water, plain carbonated water, instant
  coffee, apple juice, white sugar, toasted white bread, frozen cooked green
  peas, and white rice with no added fat.
- Strict FNDDS-grounded evaluation still passes with 885 of 900 items resolved,
  a 98.33% resolver rate, and zero expected-outcome mismatches.
- WWEIA/NHANES no-fallback coverage improved from 496 of 815 items to 550 of
  815 items, raising the reviewed local-catalog resolver rate from 60.86% to
  67.48% with zero expected-outcome mismatches.
- WWEIA/NHANES with the optional FNDDS fallback remains 774 of 815 items
  resolved, a 94.97% resolver rate. The local catalog now handles more of
  those items before fallback.

Current implementation state:

The resolver has the basic implementation shape it needs for the current
product: a reviewed local catalog, strict gates with zero expected-outcome
mismatches, optional gate-limited FNDDS fallback, and measured WWEIA/NHANES
coverage. P1 is now primarily a curation and evaluation loop.

Discrete engineering work now:

P1 implementation work starts when a specific high-frequency gap cluster or copy
problem is selected. Promote work into P1 when one of these triggers appears:

1. A mined unresolved cluster contains high-frequency ordinary foods or units
   that can be resolved with source-backed rows, aliases, or conversions.
2. The remaining unresolved items are technically correct and need clearer
   report copy for user recovery.
3. Mining unresolved foods from live reports, robustness fixtures, or
   WWEIA/NHANES outputs is too manual and needs a reusable summary artifact.
4. A policy decision explicitly admits a currently deferred class such as a
   composed food, alcohol, branded/product-style food, or fuzzy substitution.

Ongoing operating loop:

1. Mine unresolved items from live reports, robustness fixtures, and
   WWEIA/NHANES evaluation results after each catalog batch.
2. Review the next remaining high-frequency gap cluster, and include composed
   foods, alcohol, branded/product-style foods, and fuzzy substitutions after a
   specific policy decision.
3. Add source-backed aliases, conversions, or rows for high-frequency safe
   cases.
4. Keep the FNDDS fallback match-key based, gate-limited, and exact.
5. Improve report labels and recovery copy for unresolved reasons so users know
   whether to rename a food, use grams, decompose a dish, or add a measured
   quantity.

Metrics to track:

- `resolved_item_rate` on the acceptable-input corpus.
- `resolved_item_rate` on the FNDDS-grounded dataset.
- `resolved_item_rate` on the WWEIA/NHANES dataset, with and without fallback.
- Top unresolved food and unit reasons by observed frequency.
- Expected-outcome mismatch count; this should remain zero for strict gates.

## P2: Latency, Progress, And Capacity UX

Goal:

MealCheck should feel alive and bounded while the MacBook-local model works.
The product can tolerate MVP latency when users understand whether their run is
queued, normalizing, checking, writing a report, failed, or ready. Operators
should also be able to inspect compact timing, decode, repair, timeout, and
queue evidence from a summary surface.

Why this is third:

- Local llama.cpp on the MacBook Air is a real resource constraint.
- Slow but transparent can be acceptable for this MVP; slow and silent feels
  broken.
- The first timing capture belongs to P0 because it proves the SLM-critical
  path; P2 turns those measurements into user-facing progress, capacity policy,
  and tuning decisions.

Good enough:

- The UI shows a useful state quickly after submission.
- Queue-full, rate-limit, model-unavailable, timeout, and failed-run states have
  distinct recovery guidance.
- Stage timing, decode failures, repair counts, timeout counts, and queue
  behavior are visible to operators through a compact summary as the default
  review surface.
- Accepted one-day inputs stay within configured timeout limits under normal
  load.

Current evidence:

- The backend now returns a redacted `progress` object on `/api/runs/{id}` with
  product states for queued, normalizing, checking, writing report, ready,
  failed, and deleted runs.
- The live UI renders the backend progress label and recovery details while
  keeping raw event details available for inspection.
- `mealcheck local-model-summary` scans hosted local-model artifacts and failed
  normalization debug artifacts for per-run and per-chunk timings, source-item
  counts, repair counts, decode failures, timeout flags, model metadata, and
  final status.
- `mealcheck local-smoke` now exercises queue-full behavior, timeout failure
  progress, local-model unavailable responses, the one-active-local-model claim
  gate, local-model artifact writes, and local-model summary generation.
- Hosted local-model creation returns `503 local_model_unavailable` before
  queueing when the server-owned local model is not configured.
- Hosted dynamic runs now persist their `input_mode`, and the memory and
  Postgres store claim paths skip queued `local_model` runs while another
  `local_model` run is active.
- Foundation validation on 2026-07-01 included `go test ./...`,
  `npm test -- --run`, `npm run build`, `mealcheck local-smoke`, and
  `mealcheck local-model-summary`.
- Active-run hardening validation on 2026-07-01 included `go test ./...`,
  `mealcheck local-smoke`, and `mealcheck local-model-summary` against both
  the default artifact root and a kept local-smoke artifact root.

Current implementation state:

The main P2 foundation slices are complete in the current implementation:
operator summaries exist, product progress states are redacted and surfaced,
recovery guidance distinguishes the expected failure classes, and smoke
coverage exercises queue, timeout, artifact, local-model outage, and active
local-model claim behavior. P2 now moves into deployment measurement and
model-limit tuning.

Near-term engineering slices:

1. Run `mealcheck local-model-summary` against representative hosted artifacts
   after each local-model deployment or prompt/model change.
2. Use measured data before raising model limits again.
3. Keep hosted local-model input bounded to one day and the configured source
   item cap before revisiting larger scopes.
4. Compare the production `Qwen3-0.6B-Q4_K_M` model against one larger local
   candidate after the operator timing summary exists. Record quality,
   latency, timeout, memory, repair rate, decode failures, and user-visible
   tradeoffs.

Metrics to track:

- Queue wait time.
- Local-model extraction time by source-item count.
- End-to-end run time.
- Timeout count.
- Queue-full count.
- Local-model unavailable count.
- Decode failure count by meal code and source-item count.
- Source-measurement repair count by model and input pattern.
- Artifact-summary coverage for completed and failed local-model runs.

## P3: Normalized-Plan Review And Report Trust

Goal:

Users should be able to trust MealCheck's semantic interpretation before the
deterministic checker gives it authority. After normalization, the product
should show a source-linked normalized plan and let the user confirm, correct,
or reject it before checker execution. Completed reports should then make the
verifier's conclusion easy to trust and easy to act on.

Why this follows the earlier priorities:

- Report polish matters after users can reliably get reports.
- Normalized-plan review is the right trust layer for SLM risk: the model stays
  critical to the happy path, but the user can catch semantic errors before the
  checker turns them into authoritative-looking findings.
- The current artifact and report system already carries the facts needed to
  explain decisions; the next improvements should make those facts easier to
  find and act on.
- A correction loop turns user-visible SLM mistakes into review artifacts that
  can later become regression cases.

Implemented slices:

- Hosted local-model runs now pause in `awaiting_review` after normalization
  and before deterministic checker execution.
- `/api/runs/{id}/review` returns a source-linked normalized-plan review with
  source item count, normalized row count, unresolved count, repair count, and
  failed chunk count.
- The live product shows review rows by day, meal, source text, normalized
  food, quantity, unit, and unresolved reason.
- `Check now` confirms the normalized plan and runs the checker; `Reject` and
  `Rewrite input` end the run before checking and leave review action artifacts.
- Rejected and rewrite-requested review runs return users to the meal-plan text
  with recovery guidance instead of leaving the failure as a generic stopped
  run.
- Review rows can be corrected while the run is awaiting review; corrections
  are strictly validated, preserve source-item identity, update the candidate
  normalized plan used by `Check now`, and recalculate review trust signals.
- Review correction actions record the source row, before value, after value,
  reason, and timestamp, and completed reports show those details in the
  normalization trace.
- Completed confirmed runs keep review artifacts, local-model chunks,
  normalization events, and redacted provider details in the final manifest.
- Completed live reports now include a `Normalization` tab when trace artifacts
  are present, showing source inventory, normalized rows, repairs, review
  actions, and normalization events.
- Report unresolved-food rows now include source item IDs and source text when
  they can be matched back to normalized-plan review rows.
- Report unresolved foods now include a summary grouped by unresolved reason,
  count, affected day/meal, and product recovery action.
- Completed live reports now load `recommendation.json` as an optional artifact
  and include a `Recommendation` tab with status, reason, source decision,
  projected decision, and change count.
- Available recommendation rows show day, meal, source item when traceable,
  original item, replacement or addition, addressed checks, and recommendation
  reason.
- Unavailable recommendation rows show the artifact reason and remaining
  blocking checks.
- Frontend report fixtures cover available and unavailable recommendation
  states, and backend recommendation tests cover the supported missing-vegetable
  edit class.
- Recommendation artifacts are now included with a bundled schema. Available
  recommendations must carry deterministic changes, a modified plan, and a
  projected `pass` decision; unavailable recommendations do not expose attempted
  edits as product recommendations.
- Recommendation tests cover source-plan immutability, unsupported warning
  classes that stay unavailable, and attempted deterministic edits that remain
  hidden unless the checker projection passes.

Near-term engineering slices:

P3 implementation work is complete for the current product shape. Promote a new
P3 slice when review artifacts, completed reports, or user review show that
users cannot understand or act on deterministic report facts already present in
the product.

Metrics to track:

- normalized-plan review completion rate.
- normalized-plan rejection or rewrite rate by reason.
- report failures prevented by review-stage user correction.
- Correction artifacts reviewed and promoted into fixtures.
- User-visible unresolved reason coverage.
- Recommendation visibility in completed reports for available and unavailable
  states.
- Recommendation availability for supported deterministic edit classes.
- Reports with missing or confusing artifact links.
- Reports needing artifact reconciliation to reproduce the final decision.

## P4: Operations, Replay, And Source Inspection

Goal:

Make MealCheck easier to operate, replay, and explain within the current core
product scope. A maintainer should be able to reproduce the hosted workflow,
inspect a completed run, compare evaluation outputs, and verify that reports
trace back to deterministic sources.

Why this follows the earlier priorities:

- Production issues are harder to diagnose when the run path is scattered across
  docs, scripts, artifacts, and live outputs.
- Replayable operations improve release confidence while keeping the
  deterministic verifier authoritative.
- Portable evaluation outputs and source inspection help maintainers compare
  changes, review unresolved cases, and explain final report decisions.
- Runtime boundaries keep implementation ownership clear: Go remains the
  authoritative verifier/API, while Python and TypeScript are added for
  evaluation, operations, UI, or narrow edge integration needs.

Implemented slices:

- The runbook now has a focused operator walkthrough that starts with local
  validation, runs the deployed local-model smoke path, captures review and
  report artifacts, verifies deletion, and names the evidence bundle a
  maintainer should preserve for failures.
- The deployed local-model smoke script now treats `awaiting_review` as the
  expected midpoint, fetches the normalized-plan review artifact, confirms the
  review, verifies completed review/recommendation artifacts, exercises
  local-model rejection policy, and verifies deleted runs are no longer
  retrievable.
- A local-model deployment profile now lives under `deploy/local-model/`. It
  replays the hosted local-LLM path with host-local Postgres, filesystem
  artifacts, a source-built API/worker, and a loopback llama.cpp endpoint whose
  model id can be resolved from `/v1/models`. A Docker Postgres profile is kept
  only as a disposable developer fallback, not the production-parity path.
- The P0 normalization and P1 checker eval commands can now write portable
  per-case JSONL or CSV row exports alongside their aggregate JSON results, so
  normalization and resolver changes can be compared across commits from stable
  structured outputs.
- A Python operator comparison tool now consumes portable eval JSONL exports and
  reports added and removed cases, regressions, fixes, still-failing cases,
  changed metrics, and eval-specific summaries for P0 normalization and P1
  checker runs.
- Python operator code now has a minimal `tools/mealcheck_ops` package
  structure, with scripts kept as thin wrappers and tests using package imports.
- Python operator tooling can now summarize canonical run artifact directories
  into JSON and Markdown review queues, extracting unresolved normalized rows,
  source-row mismatches, deterministic normalization repairs, failed chunks,
  normalization failures, and manifest-listed missing artifacts.
- The run-artifact summary now emits deterministic cross-run clusters and a
  priority queue over repeated unresolved foods, source phrases, units, failure
  stages, timing outliers, and repair-heavy local-model chunks.

Near-term engineering slices:

1. Add TypeScript backend-adjacent code for the static UI or edge path, such as
   typed API clients, runtime config validation, report preflight, or a narrow
   Cloudflare Worker adapter backed by the Go API contract.
2. Add a compact source and citation inspection surface when it helps users
   trace report findings to source facts, guideline references, normalized
   foods, quantities, and unresolved reasons. Deterministic source packs remain
   the source of verification truth.
3. Keep product copy focused on checking a bounded meal plan against declared
   constraints, source-backed findings, and concrete recovery guidance.

Metrics to track:

- Operator walkthrough coverage for live product, local commands, validation,
  deletion, outage behavior, and artifact inspection.
- Reproducible deployment smoke success.
- Portable eval summary freshness and parity with canonical eval outputs.
- Python operator outputs reconcile to canonical Go eval/report artifacts.
- TypeScript API/config/report helpers stay contract-compatible with the Go API.
- Source inspection coverage for reports and guideline references.
- Public/docs claims that overstate medical, ML-training, or model-infra scope.

## Deferred Work Triggers

Promote these areas when the named product evidence appears:

- Add recipe decomposition after repeated in-bound users paste ingredient-level
  recipes that fail for decomposition reasons and reviewed decomposition rules
  can preserve source traceability.
- Add meal-planning or chat-style flows after verification, review, correction,
  and report recovery are reliable enough to support generation safely.
- Add clinical or disease-specific surfaces after the privacy, safety,
  regulatory, and professional-review requirements are explicitly redesigned.
- Add account history after users need persistent run retrieval, comparison, or
  deletion beyond unguessable run IDs.
- Add fuzzy food matching when reviewed exact aliases, FNDDS fallback, and
  unresolved recovery copy leave common ordinary-food failures unresolved.
- Add FoodData Central live lookup after source-backed cache policy, failure
  behavior, rate limits, and key management are designed.
- Add a dynamic frontend server after static hosting blocks a concrete user,
  operations, or security requirement.
- Add model-training, fine-tuning, or GPU-serving experiments when they answer a
  specific MealCheck model-quality, latency, or cost question.
- Add agent or RAG tooling when it helps source inspection or operator analysis
  while preserving deterministic source packs as verification authority.
- Expand deterministic recommendation classes after unresolved-state UX and
  normalization reliability are stable.
- Add generative recommendation explanation after deterministic recommendation
  artifacts, unresolved-state UX, and completed-report source visibility show a
  concrete explanation gap that static artifact views do not solve.
- Promote NYT/TASTEset-generated P0 cases after product-shaped reviewed cases
  expose the gap and manual review classifies expected rows and failures.
- Compare larger models after operator timing summaries exist.
- Raise local-model limits after stage timing shows enough headroom.

## Operating Loop

Use this loop when choosing the next engineering task:

1. Inspect the latest failed or disappointing hosted run.
2. Classify it as normalization, qualification, resolution, verifier, report,
   latency, or infrastructure.
3. Decide whether the failure is in-bound according to the meal-plan robustness
   boundary.
4. If in-bound, add or update a regression fixture before or with the fix.
5. Fix the smallest deterministic layer that owns the problem.
6. Run the relevant validation gate.
7. Update this document when priority order, target metrics, or the active next
   slices change.

## Validation Gates By Priority

P0 validation should include:

- robustness corpus normalization checks
- serving-MacBook live P0 regimen for the per-meal local-model path
- chunk-level artifact completeness for accepted model calls
- backend tests for source-item preservation and unsupported-unit handling
- frontend tests for actionable recovery messages

P1 validation should include:

- `go run ./cmd/mealcheck fixture-check`
- `go run ./cmd/mealcheck eval-checker`
- WWEIA/NHANES evaluation when changing catalog or fallback behavior
- resolver tests for new unresolved reasons or conversions

P2 validation should include:

- local and deployed smoke tests
- timing summaries from representative one-day inputs, derived from the P0
  instrumentation path
- operator summaries over local-model chunk artifacts
- queue, timeout, and model-unavailable recovery checks
- load/smoke coverage for queue behavior, artifact writes, and model outage

P3 validation should include:

- artifact bundle validation
- frontend tests for source-linked normalized-plan review and confirmation
- frontend tests for source-linked normalization inspection
- report rendering tests
- frontend tests for unresolved-item labels and recovery actions
- frontend tests for available and unavailable recommendation report states
- correction artifact validation and promotion checks
- recommendation tests when deterministic edits change

P4 validation should include:

- operator walkthrough verification against the current product
- reproducible deployment smoke test
- portable eval summary generation
- source inspection tests when that surface changes

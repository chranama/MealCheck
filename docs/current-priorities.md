# Current Priorities

This document is MealCheck's active engineering-priority surface. The
implementation plan records milestone history; this document ranks the next
work by user impact, observed failure modes, and proof value.

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

The highest-priority failure class is not a correct `block` decision. Correct
blocks are the product working. The highest-priority failure class is a
reasonable in-bound meal plan failing for an arbitrary, opaque, or
self-inflicted reason before the verifier can do useful work.

## P0: In-Bound Normalization Reliability

Goal:

MealCheck should reliably normalize concise ingredient-level meal plans that
match the public input guidance. The preloaded example and a small regression
corpus of natural rewrites must work before broader feature work gets priority.
The immediate proof is that the new one-day, per-meal, SLM-critical path works
on the serving MacBook with enough artifacts to explain failures.

Why this is first:

- Normalization is the gateway to every hosted live report.
- Failures here make the whole product feel broken, even when the deterministic
  checker is correct.
- Recent live issues were concentrated here: hidden day/meal-count constraints,
  unsupported units, natural phrasing, and local-model representation mismatch.

Good enough:

- The reviewed robustness corpus succeeds through the hosted per-meal
  local-model path on the serving MacBook, not only in deterministic or
  prototyping-laptop tests.
- The preloaded hosted example succeeds locally and on the deployed path.
- The acceptable-input robustness corpus succeeds for normalization correctness,
  independent of whether the final decision is `pass`, `warn`, or `block`.
- Each accepted model chunk has enough redacted evidence to debug prompt,
  source-inventory, model-output, decode, reconciliation, and timing failures.
- Common natural rewrites of the example either normalize successfully or fail
  before queueing with specific, actionable guidance.
- Source item preservation is exact: every resolved source item appears once,
  with no omissions or duplicates.
- No hidden default setting imposes a day count or meal count that is not
  deliberately exposed to the user.

Current evidence:

- The serving MacBook strict one-day P0 live regimen passed on 2026-06-30 at
  commit `8a3a395ab4712c9bf08471bfb45ef655d9c164c5`: 3 of 3 repeats, zero
  mismatches, zero provider failures, zero decode failures, and minimum
  row/food/quantity/unit accuracy of 1.0. The artifact directory is
  `/Users/chranama-server/MealCheck-data/p0-runs/p0-live-local-model-20260630-8a3a395`.
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
- The chunk-artifact change was deployed and smoke-verified on 2026-06-30 at
  commit `4482f8f0c5e0c1f0a34a3cefe95b8d63bd553abb`. Live run
  `run_a0363caa5080a7ede2d01d90` completed through the deployed path with 3
  meal chunks, 9 source items, exact source-ID preservation, 2 deterministic
  source-measurement repairs, and about 6.0 seconds of recorded local-model
  extraction time. The server artifact directory is
  `/Users/chranama-server/MealCheck-data/artifacts/run_a0363caa5080a7ede2d01d90`.

Near-term engineering slices:

1. Run the expanded strict P0 corpus through the serving MacBook live local-model
   regimen and inspect chunk artifacts for the new reverse-measurement success
   case.
2. Keep adding observed in-bound natural rewrites to the reviewed corpus before
   using broad generated external data as a release signal.
3. Keep deterministic unit normalization conservative: normalize supported
   units only when the conversion is visible and preserve or reject vague and
   genuinely unsupported quantities rather than inventing serving equivalents.
4. Keep user-facing failure messages guidance-oriented and avoid exposing compact
   row, schema, model-path, or parser internals.
5. Generate and manually review small NYT Ingredient Phrase Tagger and TASTEset
   exploratory slices only after source/license review and after the
   product-shaped reviewed corpus has exposed the obvious gaps; promote no
   external subset to strict until expected rows and failure categories have
   been inspected.

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
  fallback, so improvements can be targeted rather than hand-waved.
- Resolution work should follow normalization reliability because the resolver
  cannot help if the plan never becomes canonical MealCheck JSON.

Good enough:

- Strict FNDDS-grounded evaluation continues to pass with zero expected-outcome
  mismatches.
- Common user-facing gaps are addressed in reviewed batches, not by fuzzy
  matching.
- Household unit conversions are added only when source-backed or
  policy-reviewed for that food.
- Ambiguous, mixed-dish, branded, vague, unclear-preparation, unsupported-unit,
  and non-food entries remain unresolved with specific reasons.

Near-term engineering slices:

1. Mine unresolved items from live reports, robustness fixtures, and WWEIA/NHANES
   evaluation results.
2. Rank gaps by commonness and user-facing credibility, not by raw catalog
   completeness.
3. Add reviewed aliases and conversions for high-frequency safe cases.
4. Keep the FNDDS fallback match-key based and gate-limited; do not turn it
   into fuzzy food search.
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
The product does not need SaaS-like latency, but users should understand whether
their run is queued, normalizing, checking, writing a report, failed, or ready.

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
- Stage timing is visible to operators through logs or artifacts.
- Accepted one-day inputs stay within configured timeout limits under normal
  load.

Near-term engineering slices:

1. Convert P0 stage timings and failure stages into clear user/operator states
   without leaking user secrets or internal host paths.
2. Add distinct recovery guidance for queue-full, rate-limit, model-unavailable,
   timeout, failed-normalization, and report-loading failures.
3. Use measured data before raising model limits again.
4. Keep one active local-model run unless measurements show safe headroom.
5. Keep hosted local-model input bounded to one day and the configured source
   item cap before revisiting larger scopes.
6. Compare the production `Qwen3-0.6B-Q4_K_M` model against one larger local
   candidate only after P0 stage timing and live-regimen artifacts are recorded.

Metrics to track:

- Queue wait time.
- Local-model extraction time by source-item count.
- End-to-end run time.
- Timeout count.
- Queue-full count.
- Local-model unavailable count.

## P3: Normalized-Plan Review And Report Trust

Goal:

Users should be able to trust MealCheck's semantic interpretation before the
deterministic checker gives it authority. After normalization, the product
should show a source-linked normalized plan and let the user confirm, correct,
or reject it before checker execution. Completed reports should then make the
verifier's conclusion easy to trust and easy to act on.

Why this follows the earlier priorities:

- Report polish matters only after users can reliably get reports.
- Normalized-plan review is the right trust layer for SLM risk: the model stays
  critical to the happy path, but the user can catch semantic errors before the
  checker turns them into authoritative-looking findings.
- The current artifact and report system already carries strong evidence; the
  next improvements should focus on unresolved-item clarity and practical
  recovery.

Near-term engineering slices:

1. Add a normalized-plan review step between local-model normalization and
   checker execution after the expanded P0 live regimen passes.
2. Keep the review step efficient for broad users: make `Check now` the obvious
   action for clean normalizations, but require explicit confirmation when
   unresolved items, source repairs, vague quantities, or other trust signals
   are present.
3. Preserve source links by day, meal, source text, normalized food, quantity,
   unit, and unresolved reason so the user can see exactly what the SLM changed.
4. Start with confirm/rewrite recovery; add direct normalized-plan editing only
   with strict validation and clear source preservation.
5. Make unresolved items easier to scan by reason and affected day/meal.
6. Surface concrete edit guidance where deterministic and safe.
7. Keep deterministic recommendations conservative and verification-gated.
8. Avoid adding generative explanation until the deterministic evidence and
   unresolved-state UX are boringly reliable.

Metrics to track:

- normalized-plan review completion rate.
- normalized-plan rejection or rewrite rate by reason.
- report failures prevented by review-stage user correction.
- User-visible unresolved reason coverage.
- Recommendation availability for supported deterministic edit classes.
- Reports with missing or confusing artifact links.
- Reports whose final decision cannot be reproduced from artifacts.

## Not First Right Now

The following work may be valuable, but should not outrank P0/P1 unless a new
observed failure changes the priority calculation:

- broad recipe decomposition
- open-ended meal planning or chatbot flows
- disease-specific or clinical nutrition support
- full account history dashboards
- fuzzy food matching
- FoodData Central live search as a required runtime dependency
- adding a dynamic frontend server
- expanding deterministic recommendation classes before unresolved and
  normalization reliability improve
- promoting NYT/TASTEset-generated P0 cases before product-shaped reviewed cases
  and manual review
- comparing larger models before serving-MacBook P0 artifacts and stage timing
  exist
- raising local-model limits without stage timing evidence

## Operating Loop

Use this loop when choosing the next engineering task:

1. Inspect the latest failed or disappointing hosted run.
2. Classify it as normalization, qualification, resolution, verifier, report,
   latency, or infrastructure.
3. Decide whether the failure is in-bound according to the meal-plan robustness
   boundary.
4. If in-bound, add or update a regression fixture before or with the fix.
5. Fix the smallest deterministic layer that owns the problem.
6. Run the relevant proof gate.
7. Update this document only when priority order, target metrics, or the active
   next slices change.

## Proof Gates By Priority

P0 proof should include:

- robustness corpus normalization checks
- serving-MacBook live P0 regimen for the per-meal local-model path
- chunk-level artifact completeness for accepted model calls
- backend tests for source-item preservation and unsupported-unit handling
- frontend tests for actionable recovery messages

P1 proof should include:

- `go run ./cmd/mealcheck fixture-check`
- `go run ./cmd/mealcheck eval-checker`
- WWEIA/NHANES evaluation when changing catalog or fallback behavior
- resolver tests for new unresolved reasons or conversions

P2 proof should include:

- local and deployed smoke tests
- timing summaries from representative one-day inputs, derived from the P0
  instrumentation path
- queue, timeout, and model-unavailable recovery checks

P3 proof should include:

- artifact bundle validation
- frontend tests for source-linked normalized-plan review and confirmation
- report rendering tests
- frontend tests for unresolved-item labels and recovery actions
- recommendation tests when deterministic edits change

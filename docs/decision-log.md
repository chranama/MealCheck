# Decision Log

This file records decisions that affect implementation, scope, contracts, or
public expectations.

Use this log instead of separate ADR and RFC files until a decision becomes too
large to keep readable here.

Ordering: entries are reverse chronological. Add new accepted decisions near
the top of this file, immediately below this ordering note, so the latest
architecture and scope decisions are visible first. Keep superseding decisions
above the decisions they supersede.

## 2026-07-02: Python Tools Live Under `tools`, Not `scripts`

Status: Accepted

Decision:

Python command surfaces should live under importable packages in `tools/`.
`scripts/` should remain a shell-entrypoint directory for deployment, smoke,
and local-model orchestration scripts. Data-generation and reference-import
commands belong to `tools/mealcheck_data`; operator artifact and eval commands
belong to `tools/mealcheck_ops`.

Run Python tools from a source checkout with package module commands, such as:

```bash
PYTHONPATH=tools/mealcheck_ops/src \
  python3 -m mealcheck_ops compare-eval-exports --help

PYTHONPATH=tools/mealcheck_data/src \
  python3 -m mealcheck_data generate-p0-normalization-evaluation --help
```

Reason:

`scripts/` was beginning to mix shell orchestration, compatibility wrappers,
large Python data generators, and operator tooling. That shape makes ownership
unclear and encourages the directory to become a junk drawer. Python packages
make code importable, testable, and easier to grow without relying on script
path conventions.

Consequences:

- `scripts/` contains shell scripts plus its ownership README only.
- Python wrappers under `scripts/` are removed rather than retained for
  compatibility.
- Active docs use `python -m mealcheck_ops ...` and
  `python -m mealcheck_data ...`.
- Historical milestone references to old `scripts/*.py` paths remain historical
  records, not current command instructions.
- A later packaging pass may replace `PYTHONPATH=... python3 -m ...` with
  installed console scripts, but the source-tree command surface is explicit
  now.

## 2026-07-02: Review Corrections Are Source-Preserving Artifacts

Status: Accepted

Decision:

Normalized-plan review may allow user corrections before deterministic
checking, but corrections must preserve source-item identity and remain
auditable. A correction updates the candidate normalized plan used by
confirmation, records before/after values and user reason in review action
artifacts, and appears in the completed-report normalization trace.

Reason:

The local model performs semantic interpretation. A review pause without
correction lets users reject bad interpretation, but it does not let them
recover from small, visible model mistakes. Corrections strengthen the trust
boundary only if they remain tied to the original source row and are preserved
as review evidence rather than silently mutating the run.

Consequences:

- Corrections are allowed only in the review stage before checker execution.
- Source item identity is preserved; corrections are not fuzzy new extraction.
- Correction artifacts become candidate material for future P0 fixture
  promotion.
- Completed reports show correction history in the normalization trace.
- Corrections are not automatic training data.

## 2026-07-02: Source Inspection Is Deterministic Report Evidence, Not RAG

Status: Accepted

Decision:

Completed-report source inspection should trace MealCheck decisions back to
deterministic source packs, guideline citations, normalized source text,
resolved foods, unresolved foods, and excluded foods. It should not introduce
open-ended RAG, vector search, or agent tooling unless a future product or
operator requirement shows that those tools improve source inspection while
preserving deterministic source-pack authority.

Reason:

External AI-app benchmarks and job postings often reward retrieval language,
but MealCheck's product value comes from bounded verification. Rebranding or
expanding the verifier into generic RAG would weaken the trust boundary. A
compact source-inspection surface captures the useful product signal without
changing the verifier's authority model.

Consequences:

- Source inspection lives in completed reports and operator artifacts.
- Missing source references should be visible instead of hidden.
- Source packs and checked-in guideline artifacts remain the authority for
  report findings.
- RAG and agent tooling stay deferred until they serve a concrete inspection or
  operator-analysis need.

## 2026-07-01: Local Replay Means Local-Model Runtime Replay

Status: Accepted

Decision:

Operational replay for MealCheck should treat "local" as the server-owned
local-model path, not merely a laptop-hosted mock. The reproducible deployment
profile should run the Go API and worker against Postgres and filesystem
artifacts while using a private loopback llama.cpp endpoint for `local_model`
normalization. The canonical replay should use host-local Postgres and
llama.cpp services administered outside the API runner, matching production's
non-Docker MacBook shape.

Reason:

The deployed product is now centered on the hosted local-model verifier. A
mocked provider can still help deterministic tests, but it does not exercise the
runtime behavior that most affects operations: local model readiness, model id
selection, normalization review, artifact writes, queueing, and cleanup.

Consequences:

- The replay profile lives under `deploy/local-model/`.
- The profile smoke path reuses `scripts/test-deployed-local-model-live.sh`
  against `http://127.0.0.1:8080`.
- MacBook/Cloudflare production deployment remains documented separately.
- Docker Postgres is allowed only as a disposable developer-laptop fallback, not
  as the production-parity replay path.
- Mocked provider paths remain test fixtures, not the primary local deployment
  profile.

## 2026-07-01: Completed Reports Include Normalization Trace When Present

Status: Accepted

Decision:

Completed local-model reports should surface retained normalization artifacts in
the product report view. The report should load normalized-plan review rows,
local-model chunk artifacts, normalization events, and review action JSONL when
those artifacts exist, then render source inventory, normalized rows, repairs,
review actions, and normalization events from the completed report surface.

Reason:

The pre-check review step prevents the checker from acting on unseen semantic
interpretation, but trust also depends on post-check traceability. After report
generation, users should still be able to connect unresolved foods and final
checker findings back to the source text and normalized rows they confirmed.

Consequences:

- The completed report artifact loader tolerates optional normalization
  artifacts and missing artifacts.
- The report surface includes a `Normalization` tab for trace artifacts.
- Unresolved-food rows can show matched source item IDs and source text.
- Richer correction and P0-promotion workflows can build on the same retained
  review/action artifacts instead of introducing a separate trace format.

## 2026-07-01: Local-Model Runs Pause For Normalized-Plan Review

Status: Accepted

Decision:

Hosted local-model runs should pause after normalization in an `awaiting_review`
state. The product should expose a source-linked normalized-plan review artifact
before deterministic checker execution. Confirmation resumes checking and
produces the report bundle. Rejection and rewrite requests end the run before
checking and record review actions.

Reason:

The local model performs semantic interpretation that can look authoritative
once deterministic checks run over it. Users need a product surface that shows
what MealCheck extracted from their source text before the checker converts that
interpretation into a report decision.

Consequences:

- Local-model worker processing now writes review artifacts and releases the run
  lease before checker execution.
- `/api/runs/{id}/review` is available before a report manifest exists.
- Confirmation restores review and normalization artifacts into the final
  manifest after the bundle writer recreates the artifact directory.
- Review action artifacts become the starting point for future correction and
  P0-promotion workflows.
- The first slice supports confirm/reject/rewrite; direct normalized-plan
  editing remains a later P3 slice.

## 2026-07-01: Local Model Capacity Is Enforced In Store Claims

Status: Accepted

Decision:

Hosted dynamic runs should persist their `input_mode` on the durable run record.
The memory and Postgres store claim paths should enforce the local-model
capacity policy by skipping queued `local_model` runs while another
`local_model` run is already running. Non-local queued work can still be claimed
while the local model is busy.

Reason:

The MacBook-hosted local model is the scarce resource. A single worker process
limits concurrency only inside one server process, but deployment drift,
restarts, or a second worker can bypass that assumption. The capacity rule needs
to live at the durable claim boundary, where every worker process competes for
work.

Consequences:

- Run records expose `input_mode` for hosted dynamic work.
- A second local-model run remains queued until the active local-model run
  completes, fails, expires, or is deleted.
- Postgres claims serialize through a transaction advisory lock before
  evaluating local-model capacity.
- `mealcheck local-smoke` exercises active local-model claim gating.
- Model-limit increases and larger-model comparisons still depend on
  summarized timing and failure evidence.

## 2026-07-01: P2 Progress Uses Redacted Product States And Artifact Summaries

Status: Accepted

Decision:

Hosted run status should expose a redacted product-level `progress` contract
instead of requiring the frontend to infer user-facing state from raw run status,
worker events, or internal error strings. The progress contract should name
states users can understand: queued, normalizing, checking, writing report,
ready, failed, and deleted. It should also carry structured recovery guidance
for expected queue, rate-limit, local-model, timeout, normalization, and
report-artifact failures.

Local-model operator review should use a compact artifact summary command over
the existing run artifacts. `mealcheck local-model-summary` should summarize
`optional/local-model-chunks.json` and failed `debug/normalization-failure.json`
evidence by run, source-item count, meal chunk, model, commit/version, stage
timing, repair count, decode failure count, timeout, and final status.

Reason:

P2 is about making slow local-model work understandable and bounded. Raw events
and debug artifacts are useful to maintainers, but they are too noisy and too
close to implementation details for the main product surface. A public progress
contract gives the UI stable language and recovery affordances while preserving
raw artifacts for deeper inspection. A summary command lets operators review
many local-model runs without opening each chunk artifact manually.

Consequences:

- `/api/runs/{id}` includes a `progress` object alongside the existing run
  document.
- Public progress, legacy error fields, and event messages are redacted for
  internal host paths and key-shaped secrets.
- The live UI renders backend progress labels and backend recovery guidance.
- `mealcheck local-smoke` includes P2 operational coverage for queue capacity,
  timeout failure progress, local-model unavailability, artifact writes, and
  summary generation.
- Future model-limit increases and model comparisons should use summarized
  timing and failure evidence rather than one-off run inspection.
- The follow-on local-model capacity decision records store-level claim
  enforcement for one active local-model run.

## 2026-07-01: Unsupported Portion Units Fail Qualification

Status: Accepted

Decision:

The hosted local-model path should reject explicit unsupported portion units
before queueing with `meal_plan_unsupported_units`. Supported units remain
grams, ounces, cups, tablespoons, teaspoons, slices, and servings. Clear
unsupported portion words such as bowl, plate, handful, scoop, packet, can, jar,
bottle, loaf, piece, box, and bag should not be silently converted into
`serving`.

Supported reverse-measurement phrasing, such as `chicken, 100 g`, remains
inside the strict success contract. Deterministic source inventory may reorder
that into the model-facing source text `100 g chicken`, because the conversion
is visible, supported, and test-covered.

Reason:

The MacBook-hosted SLM should stay in the critical path for semantic parsing,
but deterministic guardrails must define the boundary. Inventing serving units
for unsupported measurements hides uncertainty from users and can create false
confidence in nutrition totals. A specific qualification status gives users a
clear recovery path while preserving the distinction between not-a-meal text,
vague meal outlines, recipes, hosted-contract overflow, and supported
normalization.

Consequences:

- `/api/runs` and `/api/qualify` can return unsupported-unit recovery guidance
  without calling the local model.
- The strict P0 corpus now includes supported reverse-measurement success cases
  and unsupported-unit qualification failures.
- Deterministic source inventory tests must prove unsupported units are not
  normalized into fake supported measurements.
- Common unsupported units should only move into deterministic normalization
  after a reviewed, source-backed conversion policy exists.

## 2026-07-01: Seeded Sample Report Is One Day

Status: Accepted

Decision:

The checked-in seeded proof fixture and public sample report are rescoped from
the earlier three-day fixture to a one-day fixture. The canonical case ID is
`seeded-one-day-peanut-allergy`, and its demo bundle lives under
`examples/seeded-one-day-peanut-allergy/artifacts/demo-runs`.

The seeded candidate still intentionally demonstrates blocking findings and
warnings: a peanut allergen, a vague quantity, high sodium, low calories, low
protein, and missing prep-safety language.

Reason:

The public hosted local-model product shape is explicitly one day at a time.
The sample report should model that contract directly instead of teaching or
preserving a multi-day success path.

Consequences:

- Status, demo-run, fixture-check, local-smoke, and frontend tests reference the
  one-day seeded bundle.
- `docs/seeded-report.html` mirrors the one-day report totals and unresolved
  item evidence.
- Multi-day examples remain only where they exercise rejection, robustness, or
  legacy checker behavior.

## 2026-06-30: Hosted Contract Failures Use Qualification Envelope

Status: Accepted

Decision:

Hosted `local_model` contract failures should return
`422 meal_plan_not_verifiable` with a structured qualification result whenever
the input is food-related but outside the public one-day contract. The new
`meal_plan_outside_hosted_contract` status covers weekly plans, multi-day
plans, grocery lists, source inventories, and source-item-cap overflows.

Reason:

These inputs are not malformed HTTP requests; they are understandable user
inputs that MealCheck deliberately excludes from the MacBook-Air-hosted SLM
happy path. Treating them as qualification failures lets the UI give recovery
guidance and preserves the distinction between "not a meal plan", "too vague",
"recipe-like", and "outside the hosted contract".

Consequences:

- `/api/runs` and `/api/qualify` expose the same public status vocabulary for
  hosted local-model preflight failures.
- Oversized character payloads still return `400 invalid_request` because they
  exceed the request text limit before meal-plan qualification is meaningful.
- The deterministic preflight order is public contract markers, meal-plan
  qualification, then source-span and source-item-cap validation.
- Documentation and UI copy must describe one day of meal-labeled ingredient
  text with the configured source-item cap as the hosted input contract.

## 2026-06-30: Hosted Local-Model Chunk Evidence Is Single-Run

Status: Accepted

Decision:

Successful hosted local-model runs should write `optional/local-model-chunks.json`
with one evidence record per deterministic meal chunk. Each record should include
the redacted prompt messages, meal text, source item IDs and parse statuses, raw
compact model output, decoded rows, reconciliation repairs, and stage timings.
When post-model normalization fails, the same extraction evidence should be
embedded in `debug/normalization-failure.json`.

Hosted production runs should not add repeated model calls just to measure
nondeterminism. Repeat-run instability remains part of the P0
`mealcheck eval-normalization -mode local-llama -local-model-repeats N`
regimen, where the extra inference cost is deliberate and operator-initiated.

Reason:

The product happy path should stay bounded for the MacBook Air CPU inference
environment. Chunk evidence needs to explain the actual run the user requested;
repeat stability is important, but measuring it by default would multiply
latency and capacity cost for every hosted report.

Consequences:

- Operators can debug prompt, source-inventory, model-output, decode,
  reconciliation, and timing failures from hosted run artifacts.
- The deployed smoke test can assert that local-model chunk evidence exists.
- Production artifacts explicitly note that repeat instability is not measured
  for the single hosted run.
- Regression and release-candidate stability still depend on the P0
  repeat-regimen artifacts.

## 2026-06-30: Checker Evaluation Command Uses `eval-checker`

Status: Accepted

Decision:

Rename the deterministic checker/resolver evaluation CLI command from
`mealcheck eval` to `mealcheck eval-checker`. Keep
`mealcheck eval-normalization` for the P0 normalization task. Keep the internal
Go package name as `evalchecker` because package identifiers and import paths
should avoid hyphens.

Reason:

The generic `eval` command became ambiguous after MealCheck added a separate
normalization evaluation surface. The new name makes command intent visible at
the call site: `eval-checker` evaluates canonical plans through the checker and
resolver; `eval-normalization` evaluates pasted-text normalization.

Consequences:

- User-facing docs and examples should use `mealcheck eval-checker`.
- Existing local scripts or operator muscle memory that call `mealcheck eval`
  need to be updated.
- The normalization command remains unchanged.

## 2026-06-30: Fold Working Plans Into Focused Docs

Status: Accepted

Decision:

Delete standalone planning docs once their completed work is recorded in
`docs/implementation-plan.md` and their remaining useful work is recorded in
`docs/current-priorities.md` or the relevant focused document. Fold the P0 live
local-model regimen into `docs/evaluation.md` because it is part of the P0
evaluation surface, not a separate product or architecture contract.

Reason:

Separate implementation-plan files become stale quickly and make it harder to
tell which document is authoritative. The repo already has a documentation rule:
milestones belong in the implementation plan, next slices belong in current
priorities, evaluation methods belong in evaluation docs, and accepted tradeoffs
belong in the decision log.

Consequences:

- `docs/normalization-engine-improvement-plan.md`,
  `docs/p0-external-dataset-integration-plan.md`, and
  `docs/p0-live-model-regimen.md` are removed.
- Milestone 45 records completed P0 evaluation and external-source scaffolding.
- Milestone 46 records the per-meal local-model normalization contract.
- `docs/current-priorities.md` tracks remaining P0 live-regimen,
  unsupported-unit, artifact, and external-sample review work.
- `docs/evaluation.md` is the authoritative home for P0 evaluation commands,
  metrics, seed results, and live local-model regimen details.

## 2026-06-30: Hosted Local Model Uses One-Day Per-Meal Chunks

Status: Accepted

Decision:

Make the hosted local-model happy path a one-day meal-plan workflow that calls
the small local model once per deterministic meal chunk. The backend owns
request-level gating, day and meal identification, source-item IDs, meal text
capture, and final day/meal reattachment. The model sees one meal's text span
and source inventory, then returns only compact rows in the shape:

```text
[source_item_id, food, quantity, unit]
```

The deterministic source inventory distinguishes source items that are already
measurement-resolved from bounded spans that still need model parsing. It should
accept concise natural-language meal paragraphs as well as semi-structured
bullets, including reverse-measurement forms such as `chicken, 100 g`.

Reason:

MealCheck needs the SLM to remain on the critical happy path, but the previous
whole-plan contract made the model responsible for too much structure at once.
Constraining hosted input to one day and decomposing it into per-meal chunks
keeps the semantic parsing task useful while bounding prompt size, output
tokens, and failure blast radius for the 2019 MacBook Air deployment.

Removing day and meal code from the model's output also narrows the error
surface: the backend can deterministically preserve meal assignment and use the
model for food/quantity/unit semantics inside a bounded span.

Consequences:

- Hosted local-model verification remains SLM-critical because every accepted
  meal chunk must pass through the model before deterministic checking.
- The public hosted input contract is one day of concise ingredient-level meal
  text with the configured source-item cap.
- Multi-day or weekly plans are out of scope for the hosted happy path unless
  they move to a future explicitly measured workflow.
- The backend rejects chunk outputs with missing, duplicate, or extra source
  IDs before checker execution.
- Standalone full-row compact JSON remains available for CLI/debug adapter
  compatibility, but hosted constrained decoding uses the meal-chunk schema.
- P0 local-model evaluation must route through the same chunked extraction path
  used by live hosted runs.
- A fresh live regimen on the serving MacBook is required before treating the
  per-meal contract as production-stable.

## 2026-06-29: P0 Normalization Evaluation Is Deterministic First

Status: Accepted

Decision:

MealCheck's P0 evaluation framework should measure meal-plan normalization as a
separate task from food and unit resolution. The first runnable evaluation tier
should be deterministic and CI-safe: load a P0 manifest and JSONL cases, compare
the backend's source-item inventory against expected source items, and validate
expected compact rows through the existing local llama adapter without calling
llama.cpp.

Local-model evaluation should be a later explicit tier that is opt-in and
records model outputs, normalization events, failure classes, and timing. Public
ingredient-parsing datasets such as NYT Ingredient Phrase Tagger and TASTEset
should be used as external source material for generated evaluation cases, not
as checked-in raw data or as training data by default.

Reason:

Hosted user trust currently depends first on whether in-bound pasted meal-plan
text becomes canonical MealCheck JSON. Mixing that task with nutrient resolver
coverage hides the owner of failures. A deterministic first tier can catch
source-item segmentation, day/meal assignment, and adapter regressions without
requiring the MacBook model service, while a later local-model tier can measure
the deployed path separately.

Consequences:

- P0 metrics are reported separately from P1 resolver metrics.
- `mealcheck eval-normalization` starts with deterministic manifest, success
  case, failure case, source-inventory, and adapter checks.
- CI should run only deterministic P0 tiers unless a future workflow explicitly
  provisions a model server.
- Raw third-party datasets remain outside the repository until size and license
  handling are reviewed.
- Fine-tuning, constrained decoding, or other model-weight changes should wait
  until deterministic and local-model baselines identify stable high-frequency
  failures that simpler parser, prompt, or contract changes cannot address.

## 2026-06-23: Recommendations Are Deterministic And Verification-Gated

Status: Accepted

Decision:

MealCheck may emit a modified-plan recommendation for `block` or `warn`
results, but only through deterministic backend edits. The recommendation
backend must not call the local model, BYOK providers, or any other model
endpoint. A recommendation is available only when the modified plan is
re-evaluated through the checker and the projected decision is `pass`.

If MealCheck cannot make a bounded edit, it returns an unavailable
recommendation with a reason and the remaining failed or warning checks.

Reason:

The product promise is verification, not generative meal planning. A plausible
model-authored repair could look helpful while still failing rules, inventing
quantities, or changing the submitted plan too broadly. Deterministic edits keep
the feature auditable, explainable, and aligned with the checker. Requiring a
projected `pass` prevents the UI or API from presenting unverified advice as a
successful repair path.

Consequences:

- Every artifact bundle includes `recommendation.json`.
- Available recommendations include explicit changes, a modified canonical
  plan, and a projected decision document.
- Unavailable recommendations omit modified plans and projected decisions.
- Missing meal structure and unresolved quantities remain user/model-input
  problems; MealCheck does not guess them.
- Initial supported edits are intentionally narrow: prep-safety note addition,
  allergen/excluded-food substitution, and vegetable coverage addition.
- Nutrient rebalancing can be added later only if it can be expressed as a
  bounded deterministic rule and re-verified to `pass`.

## 2026-06-23: Hosted Local Model Fails Gracefully For Non-Meal Inputs

Status: Accepted

Decision:

Hosted `local_model` run creation applies deterministic meal-plan qualification
before queueing a run. Inputs that are clearly not meal plans, are meal-like but
lack quantities or units, or are recipe-like without day/meal decomposition
return `422 meal_plan_not_verifiable` with a structured qualification result.
These requests do not call the local model and do not create queued runs.

If an input passes preflight but local-model normalization later fails, the run
still fails, but the public error is a guidance-oriented message. Sanitized model
output, parser errors, and normalization events remain in
`debug/normalization-failure.json` for debugging.

Reason:

The public hosted product should behave like a meal-plan verifier, not a parser
debugging tool. Obvious non-meal inputs should stop quickly and explain what is
missing. Borderline failures after the model runs should avoid leaking compact
contract, parser, or source-item implementation details into the consumer UI.

Consequences:

- Fast-failed inputs avoid queue and local-model capacity.
- The frontend can reuse the qualification notice for expected refusal states.
- Operators still have redacted debug artifacts for post-model failures.
- The preflight must remain conservative so plausible meal plans are sent to the
  model rather than incorrectly rejected.

## 2026-06-23: Hosted Local Model Allows Seven-Day Unbatched Fallback

Status: Accepted

Decision:

Keep per-day extraction as the preferred path for clear multi-day input, but
raise the fallback capacity for acceptable multi-day text that cannot be
decomposed safely:

```bash
MEALCHECK_LOCAL_MODEL_MAX_INPUT_CHARS='6000'
MEALCHECK_LOCAL_MODEL_MAX_OUTPUT_TOKENS='1536'
MEALCHECK_LOCAL_MODEL_TIMEOUT='240s'
LLAMA_CTX_SIZE='4096'
```

Continue using `Qwen3-0.6B-Q4_K_M.gguf`, CPU-only serving, four threads, one
llama slot, and `512` MB prompt cache.

Reason:

The measured three-day compact inputs were small on the input side but close to
the old output token cap when run unbatched. A concise seven-day, three-meal
plan should fit more reliably with a `4096` context and a `1536` output cap,
while still preserving the stricter per-day path for well-formatted `Day N`
plans.

Consequences:

- The hosted UI can advertise a `6000` character local-model limit.
- Unbatched seven-day fallback attempts may take substantially longer than
  per-day extraction and are allowed up to `240s`.
- The backend should still reject inputs above the configured character limit
  before calling llama.cpp.
- If live seven-day snack-inclusive tests require more room, that should be a
  separate experimental tier rather than the public default.

## 2026-06-23: Local Model Splits Clear Multi-Day Inputs Per Day

Status: Accepted

Decision:

When hosted `local_model` input has unambiguous `Day N` boundaries for every
requested day, MealCheck splits the candidate text into per-day extraction
calls before sending it to llama.cpp. Each section is rewritten as a one-day
prompt, normalized through the compact local row contract, restored to its
original day number, merged into one canonical MealCheck plan, and then checked
through the existing deterministic verifier. Ambiguous or incomplete day
coverage falls back to the existing whole-plan local-model extraction path.

Reason:

Live multi-day tests showed that three-day inputs were not close to the
`2048` context limit, but were close to the local model's output token cap.
Per-day extraction bounds each model call, improves failure isolation, and
makes longer hosted inputs more feasible on the small CPU-only llama.cpp
deployment without immediately raising context or output limits.

Consequences:

- Clear multi-day hosted local-model submissions make one model call per day.
- The backend continues to validate total extracted item count after merging.
- Normalization events include `local_model_decomposed` when this path is used.
- Ambiguous day structure remains supported through the previous whole-plan
  path rather than being rejected solely because decomposition failed.
- With one llama slot, latency may not fall linearly, but output-cap pressure
  and schema drift risk are reduced.

## 2026-06-22: Hosted Website Removes Example Run Navigation

Status: Accepted

Decision:

Remove the seeded example-run block from the hosted website and keep the public
homepage focused on the local-model meal-check workflow. Preserve the seeded
proof as repository material, including a standalone checked-in HTML report at
`docs/seeded-report.html`. Move the seeded artifact bundle out of Vite
`public/` assets so it is not published by the frontend build.

Reason:

The hosted website's job is now clear: paste a meal plan and get a verification
report from the server-owned local model. A visible example-run block adds a
second task and makes the first screen feel like a demo browser instead of a
tool. The seeded proof is still valuable for auditability and development, but
GitHub/repo documentation is a better place for that static artifact.

Consequences:

- The React app no longer loads `demo-runs/index.json` during boot.
- The sidebar has one active action: `New meal check`.
- The seeded bundle lives under
  `examples/seeded-one-day-peanut-allergy/artifacts/demo-runs` for backend
  compatibility and fixture checks.
- Checked-in seeded artifacts can remain for tests, backend compatibility, and
  developer workflows.
- GitHub cannot execute arbitrary app HTML inside README, so the repo artifact
  is a standalone HTML file that can be viewed as source, opened locally, or
  served through GitHub Pages if that is enabled later.

## 2026-06-22: Local Model Prompts Number Inline Source Items

Status: Accepted

Decision:

Before prompting the local model, MealCheck should resolve concise inline meal
lines into explicit numbered source item rows. For example:

```text
Day 1 breakfast: 1 cup oatmeal, 1 cup berries, and 1 cup yogurt.
```

becomes three source rows with stable source item IDs, day, meal code, and
source text. The local model must return one compact row for every source item
ID.

Reason:

The deployed local-model smoke test showed that concise natural meal text could
otherwise produce valid JSON while omitting items. The v3 row contract already
detects missing source IDs, but the backend also needs to give the model an
explicit item inventory when the user enters inline meal descriptions rather
than bullet lists.

Consequences:

- Hosted local-model prompts include a source inventory and exact item-count
  instruction when the backend can resolve inline item phrases.
- The parser defaults count-only phrases such as `1 banana` to `serving` so
  they remain verifiable while still using supported units.
- The inline splitter preserves food names containing `and` unless `and`
  introduces another quantified item.
- Missing or duplicated compact rows still fail closed before checker
  execution.

## 2026-06-22: Hosted Website Uses Server-Owned Local Model

Status: Accepted

Decision:

Make `mealcheck.dev` a no-key hosted local-model verifier. The public website
should not ask for a model provider, API key, custom endpoint, or provider
model id. Users paste concise ingredient-level meal-plan text; the backend
uses its private llama.cpp service to normalize that text into compact rows,
expands those rows into canonical MealCheck JSON, and runs deterministic
verification.

BYOK OpenAI, Anthropic, Gemini, and OpenAI-compatible custom endpoints remain
supported in the codebase as repo/API/CLI and self-hosted capabilities, not as
the primary public website workflow.

Reason:

Requiring users to bring API keys made the hosted UX feel like a technical test
harness instead of a verification product. The local llama.cpp work produced a
usable bounded path on the deployed MacBook, especially after compact row output
and launchd scheduling tuning. A no-key hosted demo is easier to explain,
reduces provider-key handling risk, and still leaves the repo as the higher
control path for users who want custom providers or local agent integration.

Consequences:

- `MEALCHECK_HOSTED_MODE=local_model` is the intended hosted production mode.
- The hosted UI hides BYOK provider/API-key controls in local-model mode.
- Hosted `local_model` requests reject client-supplied `provider` config.
- Public smoke testing should use `scripts/test-deployed-local-model-live.sh`;
  BYOK live scripts are now provider-regression tools.
- Hosted inputs need conservative local-model limits, clear busy/unavailable
  errors, and deletion/retention controls because server CPU is the bounded
  resource.
- BYOK docs should remain available, but framed as repo/API/CLI or self-hosted
  usage.

## 2026-06-22: Local llama Compact Contract Uses Row V3

Status: Accepted

Decision:

Replace the active local llama output schema with a row-oriented v3 contract:

```json
{
  "i": [
    [1, 1, "b", "cooked oatmeal", 1, "cup"],
    [2, 1, "l", "grilled chicken breast", 4, "oz"],
    [3, 2, "d", "baked salmon", 4, "oz"]
  ]
}
```

Each active row is `[source_item_id, day, meal_code, food, quantity, unit]`.
Meal codes are short and bounded: `b` breakfast, `m` morning snack, `l` lunch,
`a` afternoon snack, `d` dinner, `s` snack, and `e` evening snack. MealCheck
numbers resolved source item lines before prompting the local model, then
MealCheck-owned adapter code rejects missing or duplicated source item IDs,
groups rows by day and meal code, orders them deterministically, and expands
them into canonical verifier JSON.

Reason:

The v2 tuple contract was fast but hardcoded one day with breakfast, lunch, and
dinner. Canonical JSON supports multiple days and variable meal counts but is
too token-heavy for the resource-constrained local model path. The row contract
adds compact source item ID, day number, and meal code fields per item while
avoiding repeated canonical keys such as `days`, `meals`, `items`, `food`,
`quantity`, and `unit`. Source item IDs were added after live tests showed that
the small local model could otherwise omit items while preserving valid meal
shape.

Consequences:

- Hosted local model mode can represent multi-day and variable meal-count
  requests without returning to canonical model output.
- The local adapter remains the trusted schema owner and validates row shape,
  day range, meal code, units, and positive quantities before verifier use.
- v2 tuple output and older object-item compact output remain accepted for old
  artifacts and trial comparisons.
- Local model output caps need to be sized for the requested day and meal count;
  one-day runs still finish as soon as compact JSON is complete.

## 2026-06-22: Local llama Runs As A Private launchd Service

Status: Accepted

Decision:

Run the hosted local model as a private system `LaunchDaemon` named
`dev.mealcheck.llama`, bound to `127.0.0.1:11435`. The service uses
`deploy/macos/mealcheck-llama-server.sh` to load a machine-local env file from
`/Users/chranama-server/MealCheck-data/mealcheck-llama.env` before starting
`llama-server`.

Initial production-candidate runtime:

```bash
Qwen3-0.6B-Q4_K_M.gguf
--threads 4
--ctx-size 2048
--gpu-layers 0
--parallel 1
--cache-ram 512
```

Reason:

The MacBook deployment already uses direct macOS services rather than Docker or
orchestration. Local model serving should follow the same operational shape:
private localhost binding, launchd supervision, logs under
`MealCheck-data/logs`, and machine-local env files for tunable values. The
`2048` context default is less aggressive than the fastest short-input smoke
setting and better matches expected heterogeneous web inputs.

Consequences:

- `llama-server` is not public; the backend is the only intended caller.
- Model path and runtime flags are editable without changing the launchd plist.
- CPU-only serving remains the accepted default because Intel Metal/iGPU
  offload was slower and produced corrupted JSON in local trials.
- The launchd service must pass local `/v1/models` and structured-output smoke
  tests before the backend exposes local model verification publicly.

## 2026-06-22: Local llama Compact Contract Uses Tuple V2

Status: Accepted

Decision:

Use a shorter v2 local-only contract for llama.cpp smoke tests:

```json
{
  "b": [["cooked oatmeal", 1, "cup"]],
  "l": [["grilled chicken breast", 4, "oz"]],
  "d": [["baked salmon", 4, "oz"]]
}
```

`b`, `l`, and `d` mean breakfast, lunch, and dinner. Each item tuple is
`[food, quantity, unit]`. MealCheck-owned adapter code expands this into
canonical `schema_version: "0.1"` verifier JSON before validation.

Reason:

CPU-only Qwen3-0.6B Q4 measurements showed stable structured output near
`8-9s`, with token generation already close to the practical ceiling for the
server. The remaining software-side latency lever is reducing the number of
model-emitted structural tokens. Tuple output removes repeated item keys and
shortens meal keys while keeping the deterministic MealCheck adapter as the
schema owner.

Consequences:

- The active local llama schema now uses tuple output.
- The local smoke harness defaults to the v2 tuple prompt and a lower output
  cap.
- `mealcheck local-llama normalize` still accepts the earlier object-item
  compact shape for old artifacts.
- The public BYOK provider contracts remain canonical MealCheck JSON.

## 2026-06-22: Local llama Uses Compact Extraction Plus Trusted Expansion

Status: Accepted

Decision:

Use a local-only compact extraction contract for llama.cpp model trials instead
of asking small local models to emit full canonical MealCheck plan JSON.

The compact model output contains only meal keys and short item fields:

```json
{
  "breakfast": [{"f": "cooked oatmeal", "q": 1, "u": "cup"}],
  "lunch": [{"f": "grilled chicken breast", "q": 4, "u": "oz"}],
  "dinner": [{"f": "baked salmon", "q": 4, "u": "oz"}]
}
```

MealCheck-owned backend code expands this compact output into canonical
`schema_version: "0.1"` verifier JSON before validation. The public BYOK
provider contracts remain canonical MealCheck JSON.

Reason:

MacBook server measurements showed that lower quantization increased tokens per
second but did not materially reduce wall-clock latency when the model still
had to generate canonical wrapper fields. The useful trust boundary is for the
model to extract foods, quantities, and units, while deterministic MealCheck
code owns schema wrappers, day/meal structure, and validation. This reduces the
model token burden without weakening the verifier contract.

Consequences:

- Local llama smoke tests now measure compact extraction plus trusted adapter
  expansion, not direct canonical JSON generation.
- The compact adapter rejects unknown fields, missing meal keys, empty meal
  arrays, nonpositive quantities, and unsupported units before checker
  execution.
- `mealcheck local-llama normalize` is the canonical local CLI path for turning
  compact model output into `normalized-plan.json`.
- `mealcheck local-llama schema` emits the schema used by llama.cpp constrained
  decoding.
- Hosted OpenAI, Anthropic, Gemini, and BYOK custom-provider flows continue to
  use canonical MealCheck JSON until a local model is explicitly exposed as a
  server-owned no-key provider.

## 2026-06-19: Hosted BYOK Uses Public Policy Gates By Default

Status: Accepted

Decision:

The hosted BYOK surface should be able to run without an access code. Public
hosted use should be bounded by admission policies rather than trusted-user
gating alone: request-rate limits, daily run limits, queue limits, body-size and
text-length limits, provider timeouts, short retention, and cleanup. The
existing invite-token system remains available as `invite_required` mode for
private deployments and rollback.

Public hosted mode should disable unrestricted `openai_compatible` custom
endpoints by default. If an operator enables them, the server should still
reject localhost, private-IP, link-local, non-HTTPS, and non-default-port custom
endpoint URLs. Native OpenAI, Anthropic, and Gemini providers remain available.

Reason:

BYOK removes the largest original abuse risk: anonymous users cannot spend
maintainer model budget because they must supply their own provider key.
Access-code friction is therefore less central to product safety. The remaining
risks are service exhaustion, storage abuse, proxy/SSRF behavior through custom
endpoints, and accidental provider-key exposure. Those risks are better handled
with hard public limits and egress restrictions than with a shared access-code
gate.

Consequences:

- Public hosted qualification and run creation can omit access codes.
- `/api/health` exposes access mode and policy metadata so the frontend can
  show or hide access-code entry.
- Private deployments can still require access codes with
  `MEALCHECK_ACCESS_MODE=invite_required`.
- Public hosted custom OpenAI-compatible endpoints are disabled unless
  explicitly enabled by configuration.
- The UI treats access code entry as conditional, not a first-screen default.

## 2026-06-19: API And CLI Use One Reduced Settings Contract

Status: Accepted

Decision:

Hosted API requests and CLI case files should use one explicit `settings`
object containing `nutrition_targets` and `verification_constraints`. The
public contract should no longer accept top-level `profile` and `constraints`
objects, nor the unused demographic/profile fields removed from the hosted UI.
The internal `profile_generation` mode string can remain as a compatibility
mode name for now, but visible product language should describe it as
targets-only generation.

Reason:

Milestone 20 established that MealCheck should ask only for fields that affect
verification or provider meal-plan generation. Keeping a broader API/CLI
contract after removing those fields from the UI would create hidden semantics,
make documentation harder to trust, and encourage clients to send data that the
verifier cannot use. A single settings contract makes the hosted demo, local CLI,
future agent-tool use, and BYOK provider prompts easier to reason about.

Consequences:

- New hosted requests must send `settings.nutrition_targets` and
  `settings.verification_constraints`.
- CLI case files must use the same `settings` object.
- Old top-level `profile` and `constraints` fields are rejected.
- Report artifacts keep existing summary keys for compatibility, but those
  summaries are populated only from reduced settings.
- Future settings should be added only when deterministic verification,
  generation, qualification, or provider prompt construction actually uses them.

## 2026-06-19: Hosted Settings Ask Only For Verifier-Used Fields

Status: Accepted

Decision:

The hosted Verification Settings panel should ask only for fields that
directly affect the verifier or provider meal-plan generation contract:
calorie target, protein target, days, meals per day, allergies, excluded foods,
nutrient thresholds, calorie tolerance, and prep-safety requirement. It should
not ask for general demographic/profile fields or unused switches such as diet
pattern or shopping-list requirement until the product has code paths that use
them.

Reason:

The hosted product is a verifier, not a general nutrition profile intake. Asking
for age, sex, height, weight, activity level, goal, diet pattern, or unused
policy switches creates friction and suggests the system can personalize or
judge more than it actually does. Hidden compatibility defaults may still keep
the backend contract valid, but provider-facing prompts should include only the
same target/check fields the hosted UI exposes.

Consequences:

- Hosted users see a smaller Verification Settings panel.
- The backend request contract remains compatible with existing case files and
  API clients.
- BYOK provider prompts receive filtered settings rather than full profile and
  constraint structs.
- New hosted settings should not be exposed until they have a real verifier,
  generation, or provider-prompt effect.

## 2026-06-19: Hosted UI Is Text-First With Optional Verification Settings

Status: Accepted

Decision:

The hosted MealCheck workspace should lead with the core verification workflow:
access code, pasted meal-plan text, and model provider settings. Target and
constraint inputs should remain available, but they should live behind a
collapsed Verification Settings disclosure and use defaults unless the user
chooses to tune them.

Reason:

The product question is whether candidate meal-plan text qualifies for
verification and then passes deterministic checks. Showing profile and
constraint configuration before the text/provider workflow creates too much
setup friction and makes the hosted surface feel like a form-heavy planner
rather than a verification tool. The fields still matter because guideline
checks and profile-based generation need them, but they should not be imposed
before the user reaches the primary task.

Consequences:

- The first screen should expose Access, Meal Plan Text, and Model Provider
  before optional settings.
- Target and constraint defaults remain part of qualification and generation
  payloads.
- Users can still expand Verification Settings to adjust nutrition targets,
  constraints, and advanced thresholds.
- Backend contracts do not change for this UI decision.
- Tests should assert both the collapsed default state and that edited settings
  still affect payloads.

## 2026-06-19: Hosted Surface Uses BYOK Qualification Instead Of Manual Entry

Status: Accepted

Decision:

The hosted website should expose seeded demos, invite-gated BYOK meal-plan
qualification, and invite-gated BYOK generation. It should not expose hosted
manual structured JSON entry as a primary verifier surface. Structured JSON
verification remains available through CLI/local case files for debugging,
regression tests, and future agent-tool use.

Reason:

The target hosted user is technical enough to manage provider API keys and is
often testing LLM output. The useful hosted preflight question is whether
pasted candidate text qualifies as a verifiable meal plan, not whether a user
can manually fill every normalized JSON field in a browser form. Keeping
structured JSON local also gives users a higher-trust path when provider-key
control matters.

Consequences:

- Hosted `/api/qualify` is the synchronous meal-plan eligibility endpoint.
- Hosted `/api/runs` supports checked-in case compatibility plus BYOK
  `profile_generation` and `prompt_generation`.
- Hosted `/api/runs` rejects `input_mode: "manual_structured"` with guidance to
  use the local CLI/debug workflow.
- CLI/local case files preserve deterministic structured JSON verification.

## 2026-06-18: BYOK Is A Technical Test Surface, Not Key Storage

Status: Accepted

Decision:

Hosted BYOK should be positioned as a convenience test surface for technical
users who can create temporary, scoped, budget-limited provider keys. The
MealCheck backend may transiently handle a key for one run, but it must not
persist provider keys to storage, artifacts, logs, metrics, browser storage, or
runtime case files. Pending BYOK inputs have an expiry and fail closed if the
worker does not claim them in time.

Reason:

API keys are bearer credentials. The hosted path is useful for MVP testing, but
it is not zero-knowledge key handling because the key transits the MealCheck
backend process. Users who need the strongest key-control posture should run
MealCheck locally from the repository and submit BYOK requests to their local
backend.

Consequences:

- UI and docs must disclose the backend trust boundary.
- Custom OpenAI-compatible endpoints are explicit key recipients and must be
  trusted by the user.
- Successful provider output artifacts are redacted defensively, not only
  failure/debug artifacts.
- Hosted deployment must use HTTPS and avoid request-body or provider-header
  logging.

## 2026-06-16: Backend Updates Use A Root Launchd Poller

Status: Accepted

Decision:

Add `dev.mealcheck.autodeploy`, a root-owned system `LaunchDaemon`, to poll
GitHub every five minutes and fast-forward the MacBook checkout from
`origin/main`. The poller runs Git and Go commands as `chranama-server` so the
checkout and binaries keep normal ownership. It restarts
`system/dev.mealcheck.server` only when backend code changed.

Reason:

Cloudflare now deploys the static frontend automatically from GitHub, but the
MacBook-hosted API still needs a safe backend update path. A launchd poller
fits the existing MacBook deployment shape without exposing a public webhook or
granting GitHub SSH access to the server.

Consequences:

- The poller refuses dirty worktrees and non-fast-forward updates.
- Backend code changes run `go test ./...`, rebuild both binaries, restart the
  backend service, and verify local health.
- Documentation-only and frontend-only commits are pulled without backend
  rebuild or restart.
- Installing or removing the poller requires a password-protected `sudo` step
  on the server because it manages `/Library/LaunchDaemons`.

## 2026-06-16: MVP BYOK Supports Native Model Providers

Status: Accepted

Decision:

MealCheck's MVP BYOK generation path will support `openai`, `anthropic`,
`gemini`, and `openai_compatible` provider types.

Reason:

The MVP needs to let testers use common new-user-credit providers directly
without forcing them through an OpenAI-compatible router. Keeping
`openai_compatible` preserves custom endpoint interoperability for later router
or local-provider work.

Consequences:

- Native providers use official provider endpoints and ignore custom
  `base_url` input.
- `openai_compatible` is the only provider type that accepts `base_url`.
- BYOK key handling, redaction, invite gating, artifact writing, and bounded
  JSON repair remain the same across providers.

## 2026-06-15: Replace Direct Upload Pages With GitHub-Integrated Pages

Status: Accepted

Decision:

Replace the Direct Upload `mealcheck` Pages project with a new Cloudflare Pages
project of the same name connected to `chranama/MealCheck`. The active
production deployment now builds automatically from GitHub:

- project: `mealcheck`
- repository: `chranama/MealCheck`
- production branch: `main`
- root directory: `ui`
- build command: `npm ci && npm run build`
- build output: `dist`
- deployment ID: `dd76ce42-4a09-4482-b38e-0ba0a8d3b0f4`
- source commit: `94271e5901938d1ced9dd675c264cf095fbbbac6`
- domains: `mealcheck.pages.dev`, `mealcheck.dev`

Reason:

The Direct Upload project could not be converted to Git integration in place.
Using the same project name after deletion preserves the familiar
`mealcheck.pages.dev` hostname while adding automatic deployments from GitHub.

Consequences:

- Pushing to `main` is now the production frontend deployment path.
- Wrangler Direct Upload is no longer the normal deployment path for the
  frontend.
- The custom domain `mealcheck.dev` is bound to the Git-integrated Pages
  project.
- Production backend and tunnel deployment remain unchanged.

## 2026-06-15: MVP Web Acceptance Keeps Direct Upload Until A Deliberate Git Cutover

Status: Superseded by the 2026-06-15 GitHub-integrated Pages cutover.

Decision:

MealCheck is accepted for first invite-gated public review on the deployed
Cloudflare Pages and Cloudflare Tunnel shape:

- frontend: `https://mealcheck.dev`
- API: `https://api.mealcheck.dev`
- latest Pages deployment: `4615be28-52b7-40c2-8140-f5e12666b573`
- source commit: `d30ae3d5e0d94b229748dcda937dadc500267d65`

Production live runs require per-user access codes, with the legacy shared
invite token disabled in production config. Public smoke testing verified
health, access-code enforcement, manual live-run completion, report/artifact
retrieval, deletion, backup capture, retention posture, CORS behavior, and
fake-key BYOK redaction.

The existing Cloudflare Pages project remains Direct Upload. A 2026-06-15
Cloudflare API attempt to attach `chranama/MealCheck` as the project source was
rejected with `You cannot update the source object in a Direct Uploads
project`. Cloudflare Git integration therefore requires a new Git-integrated
Pages project and a deliberate `mealcheck.dev` custom-domain cutover.

Reason:

The deployed product now satisfies the MVP web acceptance goals without
maintainer-paid inference or router port forwarding. Replacing the Pages
project only to gain push-to-deploy would introduce avoidable cutover risk after
the public URL is already active. Keeping Direct Upload preserves the working
production path while leaving a clear migration plan if automatic Git builds
become operationally important.

Consequences:

- `docs/runbook.md` is the operational source of truth for deploy, health,
  smoke, backup, retention, and failure recovery commands.
- This decision described the Direct Upload interim state. Frontend production
  deploys now use GitHub-integrated Pages.
- A GitHub-connected Pages migration must be treated as a deployment cutover,
  not as an in-place project setting change.
- Access codes remain the MVP public live-run gate; full user accounts remain
  out of scope until product needs justify them.

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

## 2026-06-13: Live Runs Use Per-User Access Codes

Status: Accepted

Decision:

MealCheck's public seeded demos remain open, but live run creation should use
per-user access codes for the MVP web deployment. Access codes are bearer
credentials, not accounts. Operators create them with `mealcheck invite create`
against the Postgres store, share the full code out of band, and can list or
revoke them later. The backend stores only a hash of the secret portion plus a
short label, usage count, expiry, revocation time, and optional max-run limit.

The legacy `MEALCHECK_INVITE_TOKEN` shared-token path remains available for
local tests and migration, but production should set `MEALCHECK_INVITE_REQUIRED`
and use DB-backed per-user access codes.

Reason:

A single shared invite token is too coarse for a public beta because it cannot
be revoked per reviewer and gives no useful usage limit. Full accounts are too
heavy for the current static frontend and MacBook-hosted MVP. Per-user access
codes provide a middle path: they limit live backend abuse and support expiry,
revocation, and run caps without collecting email addresses or creating an
identity system.

Consequences:

- Seeded reports remain usable without any access code.
- Live manual and BYOK runs can be gated without adding accounts.
- Access codes must never be logged, stored in plaintext, committed, or shown
  in frontend config.
- The UI should call the credential an "access code"; `X-MealCheck-Invite-Token`
  remains the wire header for backward compatibility.
- If MealCheck later needs identity, sharing, billing, or persistent user
  history, this access-code model should be replaced with a real auth model.

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

## 2026-06-11: Milestone 7 Local Acceptance Uses Deterministic Smoke Harnesses

Status: Accepted

Decision:

Milestone 7 is accepted for local development/prototyping scope. Local
acceptance is demonstrated by:

- `go run ./cmd/mealcheck local-smoke`
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

## 2026-06-10: Milestone 0 Uses Seed-Scoped Fixtures

Status: Accepted

Decision:

Milestone 0 is complete with a nutrient catalog scoped to the seeded proof case,
not the broader 30 to 60 food target. The first catalog has 17 foods and exists
to exercise the schema, resolver, allergen, sodium, unit, and unresolved-quantity
paths needed by the seeded example.

MealCheck will use a native Go fixture validator:

```bash
go run ./cmd/mealcheck fixture-check
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

## 2026-06-10: Keep Documentation Focused

Status: Accepted

Decision:

MealCheck will use a small current documentation set organized around stable
questions: what the product is, what the public contracts are, how it is built,
how it is operated, what has been decided, and what the next implementation
slice is. `docs/README.md` is the authoritative index for the current document
set.

Reason:

The project is early. Separate planning, RFC, and ADR files add navigation cost
before implementation exists.

Consequences:

- Decisions live in this log by default.
- New docs need to justify their existence against the documentation rule in
  `docs/README.md`.
- Short-lived implementation plans should be folded into
  `docs/implementation-plan.md`, `docs/current-priorities.md`, or the relevant
  focused doc once they are no longer active working documents.
- MVP user flow lives in `docs/user-story.md`.
- Guideline sources and preprocessing live in
  `docs/nutritional-guidelines.md`.

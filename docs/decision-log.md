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
- Guideline sources stay in `docs/contracts.md` until the source-pack process is
  too large for that file.

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
- The initial pack is `us-adult-general-v1`.
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
- FoodData Central lookup is a later enhancement unless needed for MVP.

## 2026-06-10: MacBook Hosting Uses Fixed-Cost Constraints

Status: Accepted

Decision:

The first hosted shape uses a static frontend on Cloudflare Pages and a
MacBook-hosted backend exposed through Cloudflare Tunnel. Public users get
seeded demos. Live generation or parsing uses bring-your-own-key execution.

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

# Product

MealCheck answers one verification question:

`Does this meal plan satisfy declared constraints and source-backed checks well enough to use, or should it be revised?`

For the hosted product, the user pastes a concise ingredient-level meal plan and
MealCheck normalizes it through the server-owned local model before deterministic
verification. BYOK/custom provider flows and structured JSON entry remain
available in the downloaded repository and API/CLI for local verification,
debugging, regression cases, and future agent-tool integration.

## Problem

Meal plans can look plausible while hiding basic failures:

- calories and nutrients do not add up
- allergens or excluded foods appear in the plan
- food portions are vague or impossible to verify
- sodium, added sugar, or saturated fat exceed configured limits
- shopping lists do not match the meals
- meal-prep instructions skip storage or reheating safety
- a new prompt or model produces a worse plan than a previous version

Normal chat tools are optimized for generation and conversation. They are not
optimized for repeatable, evidence-backed checking.

MealCheck should make the check bounded, source-linked, and inspectable.

## Primary Users

- Technical users who already use LLMs or agents to draft meal plans and want
  a bounded verifier before trusting the result.
- People with recurring meal-prep prompts who want to compare prompt, model, or
  provider changes under the same deterministic checks.
- Developers evaluating whether this pattern can generalize to other
  guideline-backed consumer workflows.

## Core Use Cases

- Inspect seeded demo reports without credentials, model keys, or live
  inference.
- Determine whether model output or pasted text qualifies as a verifiable meal
  plan.
- Verify pasted ingredient-level meal-plan text through the hosted local model.
- Use BYOK/custom providers from the repo API/CLI or a self-hosted deployment
  for provider experiments and custom endpoints.
- Verify normalized structured JSON locally through the CLI for debugging and
  regression cases.
- Inspect the seeded proof as a static repository artifact.
- Check declared allergens, exclusions, nutrition targets, and verification
  constraints.
- Check calculated nutrition totals against configured guideline-derived
  thresholds.
- Produce a shareable report with failures, unresolved foods, source references,
  and a final decision.

## Product Shape

MealCheck is:

- verification-first, not chatbot-first
- deterministic-checks-first
- source-pack-driven rather than vague-health-advice-driven
- strict-schema-oriented
- fixed-cost-friendly
- local-model-hosted for the public demo, with BYOK/custom endpoints kept in
  the repo/API/CLI power-user surface
- agent-tool-ready for users who already formulate plans in an LLM environment
- honest about uncertainty and unresolved foods

All input modes converge on the same normalized JSON meal-plan contract before
evaluation. The LLM may generate structured JSON, perform bounded JSON repair,
or help determine whether text can be normalized into a verifiable meal plan.
It is not the authority for nutrition totals or guideline compliance.

## Public Demo Model

The public, no-login surface should use seeded or cached examples. A reviewer
should be able to inspect a complete report and raw artifacts without
credentials, model API keys, or paid inference calls.

The public frontend should be a static site hosted on Cloudflare Pages. Live
backend behavior should be reached through the MacBook-hosted API.

Live hosted normalization uses the server-owned local model and strict resource
limits. BYOK generation or repair belongs in local/API/self-hosted workflows.

The hosted website should expose one primary workflow plus a repo/local path:

- `New Meal Check`: policy-limited local-model normalization and deterministic
  verification of pasted meal plans
- `Run Locally`: documentation links for CLI/backend, BYOK/custom providers,
  seeded proof artifacts, and future agent-tool integration

Structured manual JSON entry is not a primary hosted workflow. It belongs in
the local CLI and development workflow where it supports fixtures, debugging,
regression tests, and agent-generated structured inputs.

Seeded examples should remain checked-in fixtures and repo documentation
artifacts. They should not appear as a separate example-run block in the hosted
website's main workflow.

## In Scope

- Healthy-adult seeded scenarios.
- Static seeded report artifact in the repository.
- Policy-limited public local-model verification of pasted meal-plan text.
- Meal-plan qualification before verification.
- Local CLI structured JSON verification for debugging and regression cases.
- Strict meal-plan schema.
- Small fixture nutrient catalog for the first proof and local/debug paths.
- Versioned guideline packs derived from public sources.
- Deterministic checks for structure, allergens, exclusions, nutrient limits,
  food-group coverage, meal-prep safety, and regression against baseline.
- Evidence artifacts and human-readable reports.
- A constrained hosted local-model service mode.

## Out Of Scope

- Medical nutrition therapy.
- Diagnosis, treatment, or disease-specific meal recommendations.
- Pregnancy, pediatric, renal, diabetic, eating-disorder, or other clinical
  nutrition scenarios in the first version.
- Claims that a meal plan will cause weight loss or improve health outcomes.
- Grocery-store price optimization.
- Local LLM serving as the primary value proposition.
- Anonymous live inference paid for by the maintainer.
- Hosted structured manual-entry as a primary nontechnical workflow.
- Open-ended hosted meal-plan brainstorming chat.
- High-availability production operation.

## Success Criteria

MealCheck is useful when:

- a new user can inspect the hosted verifier quickly
- a developer can inspect the seeded proof from the repository
- failures are tied to concrete evidence
- unresolved foods and uncertain matches are visible
- deterministic checks are separated from LLM-generated explanations
- source-pack versions are visible in reports
- generated artifacts are stable enough for the hosted UI and local CLI
- the hosted public surface is inspectable without secrets or live paid calls
- public hosted users can test whether pasted ingredient-level content
  qualifies as a verifiable meal plan and can be checked
- the Cloudflare Pages frontend remains useful when the MacBook backend is
  offline
- the MacBook backend can run behind Cloudflare Tunnel as a bounded,
  policy-limited live-run service
- a reviewer can reach the deployed frontend and API without local setup
- local users can verify structured JSON through the CLI without provider keys
  or hosted infrastructure

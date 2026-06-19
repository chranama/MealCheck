# Product

MealCheck answers one verification question:

`Does this meal plan satisfy declared constraints and source-backed checks well enough to use, or should it be revised?`

For the hosted product, the plan is generated, normalized, or repaired through
MealCheck's bring-your-own-key LLM flow. Structured JSON entry remains available
in the downloaded repository and CLI for local verification, debugging,
regression cases, and future agent-tool integration.

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
- Generate a structured meal plan from nutrition targets and verification
  constraints through bring-your-own-key execution.
- Generate a structured meal plan from a user prompt plus nutrition targets and
  verification constraints through bring-your-own-key execution.
- Verify normalized structured JSON locally through the CLI for debugging and
  regression cases.
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
- local-first with a hosted demonstration and BYOK verification wrapper
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

Live LLM generation or repair should require bring-your-own-key execution and
strict resource limits.

The hosted website should expose three primary surfaces:

- `Demo Reports`: public seeded reports and artifacts
- `BYOK Verify`: invite-gated qualification, generation or normalization, and
  deterministic verification
- `Run Locally`: instructions for local CLI/backend use and future agent-tool
  integration

Structured manual JSON entry is not a primary hosted workflow. It belongs in
the local CLI and development workflow where it supports fixtures, debugging,
regression tests, and agent-generated structured inputs.

## In Scope

- Healthy-adult seeded scenarios.
- Public seeded report demos.
- Invite-gated BYOK targets-only generation and prompt-based generation.
- Meal-plan qualification before verification.
- Local CLI structured JSON verification for debugging and regression cases.
- Strict meal-plan schema.
- Small fixture nutrient catalog for the first proof and local/debug paths.
- Versioned guideline packs derived from public sources.
- Deterministic checks for structure, allergens, exclusions, nutrient limits,
  food-group coverage, meal-prep safety, and regression against baseline.
- Evidence artifacts and human-readable reports.
- A constrained hosted BYOK service mode.

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

- a new user can inspect a seeded report quickly
- failures are tied to concrete evidence
- unresolved foods and uncertain matches are visible
- deterministic checks are separated from LLM-generated explanations
- source-pack versions are visible in reports
- generated artifacts are stable enough for the hosted UI and local CLI
- the hosted public surface is inspectable without secrets or live paid calls
- invite-gated BYOK users can test whether generated or pasted content
  qualifies as a verifiable meal plan
- the Cloudflare Pages frontend remains useful when the MacBook backend is
  offline
- the MacBook backend can run behind Cloudflare Tunnel as a bounded,
  invite-gated live-run service
- a reviewer can reach the deployed frontend and API without local setup
- local users can verify structured JSON through the CLI without provider keys
  or hosted infrastructure

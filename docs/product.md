# Product

MealCheck answers one verification question:

`Does this LLM-generated meal plan satisfy declared constraints and source-backed checks well enough to use, or should it be revised?`

The plan may come from ChatGPT, Claude, Gemini, another workflow agent, a saved
prompt, or MealCheck's own bring-your-own-key generation flow.

## Problem

LLM-generated meal plans can look plausible while hiding basic failures:

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

- General users who already ask LLMs for meal plans and want a sanity check.
- People with recurring meal-prep prompts who want to compare prompt or model
  changes.
- Portfolio reviewers inspecting a constrained, evidence-based LLM application.
- Developers evaluating whether this pattern can generalize to other
  guideline-backed consumer workflows.

## Core Use Cases

- Validate a pasted meal plan.
- Compare a baseline meal plan against a candidate meal plan.
- Compare two prompt/model outputs using bring-your-own-key execution.
- Check declared allergens, exclusions, and profile constraints.
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
- local-first with a constrained hosted wrapper
- honest about uncertainty and unresolved foods

The LLM may generate, parse, or explain meal plans. It is not the authority for
nutrition totals or guideline compliance.

## Public Demo Model

The public, no-login surface should use seeded or cached examples. A reviewer
should be able to inspect a complete report and raw artifacts without
credentials, model API keys, or paid inference calls.

The public frontend should be a static site hosted on Cloudflare Pages. Live
backend behavior should be reached through the MacBook-hosted API.

Live generation or parsing should require bring-your-own-key execution and
strict resource limits.

## In Scope

- Healthy-adult seeded scenarios.
- Strict meal-plan schema.
- Small fixture nutrient catalog for the first proof.
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

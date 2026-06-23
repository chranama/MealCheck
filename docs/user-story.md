# User Story

This document defines the tightened MVP user story MealCheck should support
first.

## Primary Story

A technically capable user is already using an LLM, agent, or prompt workflow
to draft meal plans. They want MealCheck to answer two bounded questions before
they trust the result:

1. Is this content specific enough to qualify as a verifiable meal plan?
2. If so, does the normalized meal plan violate declared constraints or
   source-backed guideline checks?

Example user:

- adult healthy-user scenario
- no clinical diet requirements
- can paste concise ingredient-level meal-plan text into the hosted site
- may use OpenAI, Anthropic, Gemini, or an OpenAI-compatible endpoint when
  running the repo/API/CLI locally or in a self-hosted deployment
- wants a three-day or seven-day meal plan checked
- wants MealCheck to verify the plan, not provide medical nutrition advice
- may run the repo locally or use MealCheck as a future agent-callable tool

## Product Surface

MealCheck has three intended surfaces:

- Hosted website: public seeded demo reports plus policy-limited local-model
  verification of pasted meal-plan text.
- Downloaded repo: trusted local verifier, local backend, BYOK/custom provider
  surface, and debugging surface.
- CLI: structured JSON validation, fixture regression, artifact inspection, and
  future agent-tool integration.

The hosted website is not the primary structured manual-entry verifier and is
not a general meal-planning chatbot. Structured JSON entry remains preserved in
the CLI and local development workflow for debugging and regression purposes.

## User Goal

The user wants answers to:

```text
Does this text or model output qualify as a meal plan MealCheck can verify?
```

and, once qualified:

```text
Does this meal plan violate my constraints or the selected public-guideline checks?
```

The user should receive a qualification result before verification, and then a
clear `pass`, `warn`, or `block` decision with evidence after verification.

## Preconditions

The MVP assumes:

- the user is an adult
- the user is not asking for disease-specific, pediatric, pregnancy, or
  therapeutic nutrition guidance
- hosted model-backed live work uses the server-owned local model and is bounded
  by public request and run policies
- the selected guideline pack is `dga-2025-2030-us-adult-general-v1`
- all verification modes eventually produce a normalized JSON meal plan
- nutrient values are calculated from MealCheck's resolver, not trusted from the
  LLM
- strong judgments are made only from user constraints, resolved nutrient data,
  guideline-pack rules, or a baseline plan

## What Qualifies As A Meal Plan

MealCheck verifies structured, ingredient-level meal plans. Natural language can
start the process, but natural language is not directly eligible for
verification until it is normalized into the MealCheck meal-plan contract.

A verifiable meal plan must answer:

- what days are covered
- what meals exist on each day
- what food items are in each meal
- what quantities and units are attached to those foods
- which items are unresolved when quantities or foods cannot be normalized

Not eligible:

```text
Eat healthy this week.
```

Not yet eligible because it is too vague:

```text
Breakfast: oatmeal
Lunch: salad
Dinner: chicken bowl
```

Recipe-like but requiring decomposition:

```text
Make a healthy chicken bowl with rice, vegetables, and sauce.
```

Eligible:

```text
Day 1 / Breakfast / cooked oatmeal / 1 / cup
Day 1 / Breakfast / blueberries / 0.5 / cup
Day 1 / Lunch / chicken breast / 6 / oz
Day 1 / Lunch / brown rice / 1 / cup
Day 1 / Lunch / broccoli / 1 / cup
```

Eligible plans may still contain unresolved foods or quantities, but those
uncertainties must be explicit rather than silently guessed.

## Internal Meal Plan Contract

MealCheck evaluates structured JSON, not natural language.

Natural language may appear in:

- the user's custom generation prompt
- pasted source text that a BYOK model attempts to normalize
- user-facing report explanations

The auditable evaluation input is always normalized JSON. The same schema is
used whether the plan was generated from nutrition targets and verification
constraints, generated from a custom prompt, normalized from pasted text, or
supplied locally to the CLI.

Minimal internal shape:

```json
{
  "days": [
    {
      "day": 1,
      "meals": [
        {
          "name": "breakfast",
          "items": [
            {
              "food": "cooked oatmeal",
              "quantity": 1,
              "unit": "cup",
              "preparation": "plain"
            }
          ]
        }
      ]
    }
  ]
}
```

## Hosted Local-Model Flow

1. The user opens the hosted MealCheck verification surface.
2. The user pastes concise ingredient-level meal-plan text.
3. MealCheck uses the server-owned local model to normalize the pasted text into
   compact MealCheck rows, then expands those rows into canonical verifier JSON.
4. If defaults are not sufficient, the user opens Verification Settings and
   adjusts nutrition targets:
   - calorie target
   - protein target
5. If defaults are not sufficient, the user adjusts verification constraints:
   - number of days
   - meals per day
   - allergies
   - foods to avoid
   - sodium limit
   - added sugar limit
   - saturated fat limit
   - calorie tolerance
   - prep-safety-notes requirement
6. MealCheck creates a normalized JSON plan and verifies it with deterministic
   checks.

BYOK provider settings are not part of the hosted website flow. They remain
available through the repo API/CLI or self-hosted deployments for users who want
OpenAI, Anthropic, Gemini, or custom OpenAI-compatible endpoint experiments.
   it deterministically.
9. MealCheck creates an artifact bundle and report for completed runs.
10. The user sees the qualification result, decision, failed checks, unresolved
    foods, calculated totals, and source-pack citations.

## Local CLI Debug Flow

The local CLI preserves structured JSON validation and artifact writing.

The CLI is used for:

- validating fixture cases
- debugging normalized meal-plan JSON
- reproducing checker behavior without hosted infrastructure
- regression testing
- preparing a future agent-tool integration

Structured manual JSON entry is valid in this local/debug context. It is not the
primary hosted website workflow.

## Qualification Outcomes

MealCheck should distinguish qualification from verification.

Target qualification outcomes:

- `not_meal_plan`: the content does not describe a meal plan
- `meal_plan_too_vague`: the content resembles a plan but lacks verification
  details such as days, meals, ingredients, quantities, or units
- `recipe_or_menu_needs_decomposition`: the content includes recipes or menu
  labels that must be decomposed into ingredient-level items
- `eligible_for_verification`: the content has enough structure to verify
- `eligible_with_unresolved_items`: the content can be verified, but some foods
  or quantities remain unresolved and must be reflected in the report

Qualification answers whether verification can proceed. It does not decide
whether the plan passes.

## LLM Role

The LLM may:

- generate a candidate meal plan
- classify whether text appears to be a meal plan
- normalize eligible text into MealCheck JSON
- produce normalized JSON directly from nutrition targets and verification
  constraints
- produce normalized JSON from a custom user prompt
- repair malformed JSON when allowed
- explain failed checks in plain language

The LLM must not:

- decide whether the plan passes
- provide the source of truth for calories or nutrients
- override allergies, excluded foods, or guideline-pack limits
- silently invent missing nutrition-critical quantities
- produce medical claims about health outcomes

If LLM output fails schema validation, MealCheck may make one bounded repair
attempt using the user's BYOK provider key. Repair may fix JSON syntax or minor
schema mismatches. Repair must not invent missing nutrition-critical
information. Missing quantities or units should remain unresolved.

## MealCheck Role

MealCheck owns:

- meal-plan qualification contract
- schema validation
- settings validation
- guideline-pack rule loading
- food and unit resolution
- nutrition calculation
- allergen and exclusion checks
- deterministic check execution
- pass, warn, or block decision
- report and artifact generation

## MVP Checks

The first hosted and local story should support:

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

## Report Outcome

The report should answer:

- What nutrition targets and verification constraints were used?
- Which guideline pack was used?
- Which foods were resolved?
- Which foods were unresolved?
- What were the calculated daily nutrition totals?
- Which checks passed, warned, or blocked?
- What exact meal-plan items caused failures?
- What should the user do next?

Example blocking outcome:

```text
Decision: block

The plan includes peanut sauce despite the declared peanut allergy. Day 2 also
exceeds the configured sodium limit. The plan should be revised before use.
```

## Acceptance Criteria

The tightened MVP user story is supported when:

- a public Cloudflare Pages frontend loads from a stable URL
- the seeded report is inspectable from the public frontend without login,
  network calls to the MacBook backend, model API keys, or paid inference
- hosted navigation exposes demo reports, local-model verification, and local-run
  instructions
- hosted live verification does not require provider API keys and is bounded by
  public request/rate/run policies
- hosted live verification does not present structured manual entry as the
  primary workflow
- a reviewer can build or install the local CLI from a fresh checkout and run
  the seeded proof without network access, provider keys, or hosted services
- the CLI preserves structured JSON validation for debugging and regression
  cases
- optional BYOK targets-only generation can create a plan without storing the
  user's provider key
- optional BYOK prompt-based generation can create a plan without storing the
  user's provider key
- a qualification step can distinguish content that is not a meal plan, too
  vague to verify, recipe-like but undecomposed, eligible, or eligible with
  unresolved items
- policy-limited local-model runs can be created, monitored, viewed, and
  deleted through the web surface or documented API commands
- BYOK flows disclose that provider keys transit the MealCheck backend, and
  that nutrition targets, verification constraints, prompt text, and generated
  meal-plan content are sent to the user's selected provider when those flows
  are used from the repo/API/CLI or self-hosted deployments
- every verification mode produces auditable JSON before deterministic checks
- MealCheck calculates nutrition totals from resolver data
- failed checks include evidence and source references
- unresolved foods are visible rather than silently ignored
- the final decision is reproducible from artifacts
- persisted reports, artifacts, metadata, and logs do not include model provider
  API keys
- the report avoids medical diagnosis, treatment, or outcome claims

## Out Of Scope

The MVP user story does not support:

- disease-specific meal plans
- pregnancy or lactation guidance
- pediatric meal planning
- eating-disorder contexts
- supplement recommendations
- medication interactions
- personalized clinical nutrition advice
- grocery price optimization
- guaranteed weight-loss outcomes
- hosted nontechnical manual meal-plan entry as a primary workflow
- open-ended hosted brainstorming chat for deciding what to eat

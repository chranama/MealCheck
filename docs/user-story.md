# User Story

This document defines the MVP user story MealCheck should support first.

## Primary Story

A healthy adult wants to create or enter a meal plan, then verify whether the
plan satisfies declared constraints and guideline-backed checks before using it.

Example user:

- 35-year-old male
- generally healthy
- no clinical diet requirements
- wants a three-day or seven-day meal plan
- may want LLM generation, prompt-based generation, or manual structured entry
- wants MealCheck to verify the plan, not provide medical nutrition advice

## User Goal

The user wants an answer to:

`Does this meal plan violate my constraints or the selected public-guideline checks?`

The user should receive a clear `pass`, `warn`, or `block` decision with
evidence.

## Preconditions

The MVP assumes:

- the user is an adult
- the user is not asking for disease-specific, pediatric, pregnancy, or
  therapeutic nutrition guidance
- the selected guideline pack is `dga-2025-2030-us-adult-general-v1`
- all input modes eventually produce a normalized JSON meal plan
- nutrient values are calculated from MealCheck's resolver, not trusted from the
  LLM
- strong judgments are made only from user constraints, resolved nutrient data,
  guideline-pack rules, or a baseline plan

## Internal Meal Plan Contract

MealCheck evaluates structured JSON, not natural language.

Natural language may appear in:

- the user's custom generation prompt
- user-facing report explanations

The auditable evaluation input is always normalized JSON. The same schema is
used whether the plan was generated from profile fields, generated from a custom
prompt, or entered manually through a form.

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
              "food": "oatmeal",
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

## User Flow

1. The user starts a new MealCheck run.
2. The user enters profile inputs:
   - age
   - sex
   - height
   - weight
   - activity level
   - goal, such as maintain weight or mild calorie deficit
3. MealCheck selects the default guideline pack for the profile. MVP default:
   `dga-2025-2030-us-adult-general-v1`.
4. The user enters or adjusts plan settings and optional constraints:
   - number of days
   - meals per day
   - allergies
   - foods to avoid
   - diet pattern
   - calorie target override
   - protein target
   - sodium limit
   - added sugar limit
   - saturated fat limit
   - preferred foods
   - disliked foods
   - prep-time or meal-prep requirements
   - shopping-list requirement
5. The user chooses one of three plan input modes:
   - profile-only LLM generation
   - prompt-based LLM generation
   - manual structured entry without an LLM
6. MealCheck produces or receives a normalized JSON meal plan.
7. MealCheck validates the meal-plan schema.
8. MealCheck resolves foods, quantities, and nutrients.
9. MealCheck runs deterministic checks.
10. MealCheck creates an artifact bundle and report.
11. The user sees the decision, failed checks, unresolved foods, calculated
    totals, and source-pack citations.

## Input Modes

### Profile-Only LLM Generation

The user provides profile and constraints through forms. MealCheck builds the
LLM prompt behind the scenes.

Flow:

```text
profile + constraints
  -> MealCheck system prompt + JSON schema
  -> remote BYOK LLM
  -> normalized meal_plan.json
  -> verifier
```

This is the lowest-friction LLM path. The user does not need to write a prompt.

### Prompt-Based LLM Generation

The user writes a custom natural-language prompt after profile and constraints
exist.

Example:

```text
Create a simple three-day high-protein meal-prep plan. I prefer eggs, rice,
chicken, Greek yogurt, and easy leftovers. Avoid peanuts and shellfish.
```

MealCheck combines the custom prompt with its system prompt, schema requirement,
profile, and constraints.

The user's prompt may influence preferences, but it must not override declared
hard constraints such as allergies, excluded foods, or guideline-pack limits.

Flow:

```text
profile + constraints + user prompt
  -> MealCheck system prompt + JSON schema
  -> remote BYOK LLM
  -> normalized meal_plan.json
  -> verifier
```

### Manual Structured Entry

The user enters the meal plan directly through a form. No LLM is required.

Example form rows:

```text
Day 1 / Breakfast / oatmeal / 1 / cup
Day 1 / Breakfast / blueberries / 0.5 / cup
Day 1 / Lunch / chicken breast / 6 / oz
```

Flow:

```text
profile + constraints + manual plan form
  -> normalized meal_plan.json
  -> verifier
```

This path is important because it proves MealCheck is a verifier, not just an
LLM wrapper.

## LLM Role

The LLM may:

- generate a candidate meal plan
- produce normalized JSON directly from profile and constraints
- produce normalized JSON from a custom user prompt
- repair malformed JSON when allowed
- explain failed checks in plain language

The LLM must not:

- decide whether the plan passes
- provide the source of truth for calories or nutrients
- override allergies, excluded foods, or guideline-pack limits
- produce medical claims about health outcomes

If LLM output fails schema validation, MealCheck may make one bounded repair
attempt using the user's BYOK provider key. Repair may fix JSON syntax or minor
schema mismatches. Repair must not invent missing nutrition-critical
information. Missing quantities or units should remain unresolved.

## MealCheck Role

MealCheck owns:

- schema validation
- profile and constraint validation
- form-to-JSON conversion for manual structured entry
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

- What profile and constraints were used?
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

The MVP user story is supported when:

- a seeded example runs without network access or model API keys
- manual structured entry can produce the same normalized meal-plan JSON without
  an LLM
- optional BYOK profile-only generation can create a plan without storing the
  user's provider key
- optional BYOK prompt-based generation can create a plan without storing the
  user's provider key
- every input mode produces auditable JSON before verification
- MealCheck calculates nutrition totals from resolver data
- failed checks include evidence and source references
- unresolved foods are visible rather than silently ignored
- the final decision is reproducible from artifacts
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

# Contracts

MealCheck should expose one product contract across CLI, reports, examples, and
the hosted service. Different surfaces can wrap the contract, but should not
invent different semantics.

## Meal Check Case

The first case format should be JSON. JSONL can be added once batch runs exist.

Initial fields:

- `case_id`: stable identifier.
- `profile`: age, sex, height, weight, activity level, and optional calorie
  target.
- `constraints`: allergies, excluded foods, diet pattern, days, meals per day,
  budget or prep limits, and check thresholds.
- `baseline_plan`: optional baseline plan.
- `candidate_plan`: required plan to evaluate.
- `expectations`: deterministic checks and severity settings.
- `guideline_pack`: versioned pack identifier.
- `tags`: optional grouping metadata.

Example:

```json
{
  "case_id": "seeded-3-day-peanut-allergy",
  "profile": {
    "age": 35,
    "sex": "female",
    "height_cm": 165,
    "weight_kg": 68,
    "activity_level": "moderate",
    "calorie_target_kcal": 2000
  },
  "constraints": {
    "days": 3,
    "meals_per_day": 3,
    "allergies": ["peanut"],
    "excluded_foods": ["shellfish"],
    "max_sodium_mg_per_day": 2300,
    "max_added_sugar_pct_calories": 10,
    "max_saturated_fat_pct_calories": 10
  },
  "guideline_pack": "us-adult-general-v1",
  "candidate_plan": "plans/candidate.json"
}
```

## Meal Plan Schema

LLM-generated or user-pasted plans must be normalized before evaluation.

Minimal normalized shape:

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
              "food": "plain Greek yogurt",
              "quantity": 1,
              "unit": "cup",
              "preparation": "plain"
            }
          ]
        }
      ]
    }
  ],
  "shopping_list": [
    {
      "food": "plain Greek yogurt",
      "quantity": 3,
      "unit": "cups"
    }
  ],
  "prep_notes": [
    "Refrigerate leftovers promptly."
  ]
}
```

Required fields:

- days
- meals
- meal names
- item food names
- item quantities
- item units

Optional fields:

- preparation
- brand
- recipe name
- shopping list
- prep notes
- user-facing explanation

The checker should treat vague quantities such as `some`, `a bit`, or `to
taste` as unresolved unless the check explicitly allows them.

## Guideline Pack Contract

Guideline packs are versioned local data artifacts derived from public sources.
The engine should not scrape government sites during normal runs.

Required fields:

- `pack_id`
- `name`
- `audience`
- `source_documents`
- `derived_rules`
- `limits`
- `citation_labels`
- `last_verified`
- `disclaimer`

Initial source candidates:

- Dietary Guidelines for Americans 2025-2030:
  `https://www.dietaryguidelines.gov/`
- USDA DRI Calculator:
  `https://www.nal.usda.gov/human-nutrition-and-food-safety/dri-calculator`
- USDA FoodData Central:
  `https://fdc.nal.usda.gov/`
- FDA Daily Values for Nutrition Facts labels:
  `https://www.fda.gov/food/nutrition-facts-label/daily-value-new-nutrition-and-supplement-facts-labels`
- FoodSafety.gov:
  `https://www.foodsafety.gov/`

Guideline packs should distinguish:

- directly quoted or source-derived limits
- product-specific thresholds chosen by MealCheck
- user-configured limits
- unresolved areas where the system should not make strong claims

## Nutrition Resolution Contract

MealCheck should not trust nutrient totals supplied by the LLM.

For each food item, the resolver should produce:

- normalized food name
- matched nutrient catalog item
- match confidence
- quantity and normalized unit
- calories
- protein
- carbohydrate
- fat
- saturated fat
- sodium
- added sugar when available
- fiber when available
- unresolved reason when a match fails

MVP behavior:

- Use a small local fixture nutrient catalog for seeded examples.
- Mark unresolved foods explicitly.
- Warn or block when too much of the plan cannot be resolved.
- Add live FoodData Central lookup only after the local fixture path is stable.

## Check Contract

Checks should return:

- `check_id`
- `status`: `pass`, `warn`, `block`, or `not_applicable`
- `severity`
- `message`
- `evidence`
- `source_refs`
- `affected_days`
- `affected_meals`

Initial checks:

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
- `baseline_candidate_regression`

Strong judgments require at least one of:

- user-supplied constraint
- expected answer
- source material
- versioned guideline-pack rule
- trusted baseline

## LLM Secret Contract

Bring-your-own-key execution must follow these rules:

- API keys are never written to Postgres, artifact bundles, reports, logs, or
  metrics.
- Generation and parsing credentials are separate inputs when both are needed.
- Resolved configs stored in artifacts must redact secret material.
- Async hosted runs may hold keys only in memory or short-lived encrypted job
  state.
- Keys must be discarded when a run completes, fails, expires, or is cancelled.

## Decision Contract

The external decision enum is:

- `pass`: no blocking violation detected
- `warn`: review needed, but not an automatic block
- `block`: plan should not be used without revision

Required `decision.json` fields:

- `decision`
- `summary`
- `risk_level`: `low`, `medium`, or `high`
- `failed_checks`
- `unresolved_items`
- `recommended_action`
- `guideline_pack`
- `artifact_paths`

CLI exit behavior:

- exit `0` for `pass`
- exit `0` for `warn` by default
- exit `1` for `block`
- exit `2` for invalid configuration, resolver failure, or unusable artifacts

Strict mode may treat `warn` as a failing decision.

## Artifact Bundle

The shared evidence bundle should be:

```text
artifacts/<run-id>/
  decision.json
  report.html
  report.md
  failures.jsonl
  nutrition-totals.json
  unresolved-foods.json
  metrics.json
  manifest.json
  normalized-plan.json
  configs/
    run.json
    redacted-provider.json
  guideline-pack/
    pack.json
    citations.json
  schemas/
    decision.schema.json
    meal-plan.schema.json
    guideline-pack.schema.json
    report.schema.json
```

Responsibilities:

- `decision.json`: final machine-readable decision.
- `report.html`: public reviewer view.
- `report.md`: lightweight text report for terminals and PRs.
- `failures.jsonl`: failed or review-needed checks.
- `nutrition-totals.json`: calculated daily and aggregate nutrition values.
- `unresolved-foods.json`: foods and quantities the resolver could not verify.
- `metrics.json`: aggregate runtime, resolution, and check metrics.
- `manifest.json`: MealCheck version, timestamps, config hashes, and provenance.
- `normalized-plan.json`: schema-normalized evaluated plan.
- `guideline-pack/`: exact source-pack snapshot used for the run.
- `schemas/`: validation schemas for artifact consumers.

## Hosted API Contract

The initial hosted backend should expose:

- `GET /api/health`: read service health.
- `GET /api/demo-runs`: list seeded public reports.
- `POST /api/runs`: create a validation or BYOK generation run.
- `GET /api/runs/{run_id}`: read run status and summary.
- `GET /api/runs/{run_id}/events`: stream run events over SSE.
- `GET /api/runs/{run_id}/report`: fetch the rendered report.
- `GET /api/runs/{run_id}/artifacts`: list downloadable artifacts.
- `DELETE /api/runs/{run_id}`: delete run metadata and artifacts.

The frontend is a Cloudflare Pages static site. It should consume these
endpoints rather than depending on backend internals.

## Frontend Boundary Contract

The production frontend should be a static site deployed to Cloudflare Pages.
It may receive public build-time configuration such as:

- `VITE_API_BASE_URL=https://api.mealcheck.<domain>`
- display environment labels
- links to seeded public reports

The frontend must not contain:

- model provider API keys
- FoodData Central API keys if later used
- database URLs
- tunnel credentials
- backend admin tokens
- private artifact paths

Backend requirements for the frontend:

- allow the production Pages origin through CORS
- expose SSE from `GET /api/runs/{run_id}/events`
- return machine-readable JSON errors
- support a backend health/status endpoint before live-run UI ships
- keep seeded public report access separate from private BYOK run access

## Hosted Visibility And Retention

Default hosted policy:

- seeded demo runs are public
- live BYOK runs are private by default
- shareable reports require an explicit share setting
- artifacts expire after a short retention window
- expired artifacts and metadata are deleted by a cleanup job

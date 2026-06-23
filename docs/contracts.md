# Contracts

MealCheck should expose one product contract across CLI, reports, examples, and
the hosted service. Different surfaces can wrap the contract, but should not
invent different semantics.

## Meal Check Case

The first case format should be JSON. JSONL can be added once batch runs exist.

Initial fields:

- `case_id`: stable identifier.
- `input_mode`: `manual_structured`, `profile_generation`, or `prompt_generation`.
- `settings`: nutrition targets and verification constraints used by generation,
  qualification, and deterministic checks.
- `baseline_plan`: optional baseline plan.
- `candidate_plan`: required plan to evaluate for manual or seeded cases; created
  by the LLM for generation cases.
- `generation_prompt`: optional user prompt for `prompt_generation`.
- `expectations`: deterministic checks and severity settings.
- `guideline_pack_id`: versioned pack identifier.
- `guideline_pack_path`: path to the local guideline pack artifact.
- `nutrient_catalog_id`: versioned nutrient catalog identifier.
- `nutrient_catalog_path`: path to the local nutrient catalog artifact.
- `tags`: optional grouping metadata.

Example:

```json
{
  "case_id": "seeded-3-day-peanut-allergy",
  "input_mode": "manual_structured",
  "settings": {
    "nutrition_targets": {
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "verification_constraints": {
      "days": 3,
      "meals_per_day": 3,
      "allergies": ["peanut"],
      "excluded_foods": ["shellfish"],
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_prep_safety_notes": true
    }
  },
  "guideline_pack_id": "dga-2025-2030-us-adult-general-v1",
  "guideline_pack_path": "data/guidelines/dga-2025-2030-us-adult-general-v1/guideline-pack.json",
  "nutrient_catalog_id": "fixture-catalog-v1",
  "nutrient_catalog_path": "data/nutrients/fixture-catalog-v1.json",
  "candidate_plan": "plans/candidate.json"
}
```

## Input Modes

MealCheck has four model-backed or case-file input modes. The hosted website
uses `local_model` plus checked-in demo compatibility. BYOK generation modes
remain available for repo/API/local and self-hosted deployments;
`manual_structured` is preserved for CLI/local debugging and regression
fixtures.

- `manual_structured`: a local/debug case supplies normalized meal-plan JSON.
  No LLM is required.
- `local_model`: hosted pasted meal-plan text is normalized by the
  server-owned local llama.cpp model, then verified.
- `profile_generation`: the user supplies nutrition targets and verification
  constraints; MealCheck builds the LLM prompt and requires JSON output.
- `prompt_generation`: the user supplies nutrition targets, verification
  constraints, and a custom natural-language prompt; MealCheck wraps the prompt
  with its system prompt and requires JSON output.

All modes must produce the same normalized meal-plan JSON before evaluation.
The verifier evaluates the JSON artifact, not the prompt or user-facing prose.

## Meal Plan Schema

Every input mode must produce a normalized JSON meal plan before evaluation.

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
- `rules`
- `citation_labels`
- `retrieved_at` or `last_verified`
- `disclaimer`

The source selection and preprocessing pipeline are defined in
`docs/nutritional-guidelines.md`.

Selected initial source set:

- Dietary Guidelines for Americans 2025-2030:
  `https://www.dietaryguidelines.gov/`
- USDA DRI Calculator:
  `https://www.nal.usda.gov/human-nutrition-and-food-safety/dri-calculator`
- USDA FoodData Central:
  `https://fdc.nal.usda.gov/`
- FDA Daily Values for Nutrition Facts labels:
  `https://www.fda.gov/food/food-labeling-nutrition`
- FDA Food Allergies:
  `https://www.fda.gov/food/food-allergensgluten-free-guidance-documents-regulatory-information/food-allergies`
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
- Start with the foods needed by seeded fixtures.
- Expand toward roughly 30 to 60 common foods only when public demos or manual
  entry need broader coverage.
- Resolve foods by exact match plus reviewed aliases only.
- Do not use fuzzy matching in the MVP.
- Normalize quantities to grams internally.
- Accept `g`, `oz`, `cup`, `tbsp`, `tsp`, and `serving` only when the fixture
  defines the conversion for that food.
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

MVP severity defaults:

- block on declared allergen violations
- block on declared excluded-food violations
- block on missing required meal-plan structure
- block when a nutrition-critical food, quantity, or unit cannot be resolved
- warn when sodium exceeds 2,300 mg/day
- warn when saturated fat exceeds 10 percent of calories
- warn when a meal exceeds the guideline-pack added-sugar threshold
- warn when calories are outside the configured target tolerance
- warn when protein is below a configured minimum
- warn on weak food-group coverage or incomplete prep-safety evidence

Protein checks are `not_applicable` when no protein minimum is configured.
Nutrient thresholds are warnings by default unless the case or user marks them
as hard limits.

## LLM Secret Contract

Bring-your-own-key execution must follow these rules:

- API keys are never written to Postgres, artifact bundles, reports, logs, or
  metrics.
- LLM credentials are accepted only for `profile_generation`,
  `prompt_generation`, or bounded JSON repair.
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
- `guideline_pack_id`
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
  report.json
  report.html
  report.md
  failures.jsonl
  daily-totals.json
  resolved-foods.json
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
    nutrient-catalog.schema.json
    report.schema.json
  optional/
    llm-output.json
    normalization-events.json
```

Responsibilities:

- `decision.json`: final machine-readable decision.
- `report.json`: structured report data for UI rendering.
- `report.html`: public reviewer view.
- `report.md`: lightweight text report for terminals and PRs.
- `failures.jsonl`: failed or review-needed checks.
- `daily-totals.json`: calculated daily and aggregate nutrition values.
- `resolved-foods.json`: resolver matches, normalized quantities, and nutrient
  contributions.
- `unresolved-foods.json`: foods and quantities the resolver could not verify.
- `metrics.json`: aggregate runtime, resolution, and check metrics.
- `manifest.json`: MealCheck version, timestamps, config hashes, and provenance.
- `normalized-plan.json`: schema-normalized evaluated plan.
- `optional/llm-output.json`: original LLM output when an LLM was used; omitted
  for manual-only runs.
- `optional/normalization-events.json`: validation, repair, unresolved-field,
  and normalization events.
- `guideline-pack/`: exact source-pack snapshot used for the run.
- `schemas/`: validation schemas for artifact consumers.

## Hosted API Contract

The initial hosted backend should expose:

- `GET /api/health`: read service health.
- `GET /api/demo-runs`: list seeded public reports.
- `POST /api/qualify`: classify pasted candidate text and optionally normalize
  it with BYOK.
- `POST /api/runs`: create a checked-in case, profile-generation, or
  prompt-generation run.
- `GET /api/runs/{run_id}`: read run status and summary.
- `GET /api/runs/{run_id}/events`: stream run events over SSE.
- `GET /api/runs/{run_id}/report`: fetch the rendered report.
- `GET /api/runs/{run_id}/artifacts`: list downloadable artifacts.
- `GET /api/runs/{run_id}/artifacts/{artifact_path}`: fetch one artifact.
- `DELETE /api/runs/{run_id}`: delete run metadata and artifacts.

The frontend is a Cloudflare Pages static site. It should consume these
endpoints rather than depending on backend internals.

`POST /api/qualify` supports the hosted meal-plan qualification contract:

`provider` is optional when candidate text is already normalized MealCheck JSON
or can be rejected deterministically. It is required only when MealCheck needs
BYOK normalization to decide eligibility.

```json
{
  "text": "Day 1 breakfast: 1 cup cooked oatmeal and 1 banana.",
  "settings": {
    "nutrition_targets": {
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "verification_constraints": {
      "days": 1,
      "meals_per_day": 1,
      "allergies": [],
      "excluded_foods": [],
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_prep_safety_notes": true
    }
  },
  "provider": {
    "type": "gemini",
    "model": "gemini-example",
    "api_key": "user-supplied-key"
  }
}
```

Response shape:

```json
{
  "qualification": {
    "schema_version": "0.1",
    "status": "eligible_for_verification",
    "reason": "text was normalized into a MealCheck meal plan",
    "provider_used": true,
    "normalized_plan": {
      "schema_version": "0.1",
      "plan_id": "normalized-from-text",
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
                  "unit": "cup"
                }
              ]
            }
          ]
        }
      ]
    }
  }
}
```

Qualification statuses are `not_meal_plan`, `meal_plan_too_vague`,
`recipe_or_menu_needs_decomposition`, `eligible_for_verification`, and
`eligible_with_unresolved_items`. Qualification does not decide guideline
compliance; it only determines whether content can become normalized meal-plan
JSON for verification.

`POST /api/runs` supports four hosted request shapes.

Checked-in demo or fixture case:

```json
{
  "case_path": "examples/seeded-3-day-peanut-allergy/case.json"
}
```

Hosted local-model verification:

```json
{
  "input_mode": "local_model",
  "candidate_text": "Day 1 breakfast: 1 cup cooked oatmeal, 1 cup blueberries, and 1 cup plain Greek yogurt.",
  "settings": {
    "nutrition_targets": {
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "verification_constraints": {
      "days": 1,
      "meals_per_day": 3,
      "allergies": ["peanuts"],
      "excluded_foods": ["shellfish"],
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_prep_safety_notes": false
    }
  }
}
```

Targets-only BYOK generation:

```json
{
  "input_mode": "profile_generation",
  "settings": {
    "nutrition_targets": {
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "verification_constraints": {
      "days": 3,
      "meals_per_day": 3,
      "allergies": ["peanuts"],
      "excluded_foods": ["shellfish"],
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_prep_safety_notes": true
    }
  },
  "provider": {
    "type": "openai",
    "model": "gpt-example",
    "api_key": "user-supplied-key"
  },
  "repair_json": true
}
```

Prompt-based BYOK generation:

```json
{
  "input_mode": "prompt_generation",
  "settings": {
    "nutrition_targets": {
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "verification_constraints": {
      "days": 3,
      "meals_per_day": 3,
      "allergies": ["peanuts"],
      "excluded_foods": ["shellfish"],
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_prep_safety_notes": true
    }
  },
  "generation_prompt": "Create a simple 3 day high-protein meal plan.",
  "provider": {
    "type": "anthropic",
    "model": "claude-example",
    "api_key": "user-supplied-key"
  },
  "repair_json": true
}
```

Rules:

- `case_path` cannot be combined with `input_mode`.
- Hosted `/api/runs` rejects `input_mode: "manual_structured"`; structured JSON
  verification belongs to CLI/local case files.
- `local_model` requires `candidate_text`, rejects `provider`, and uses the
  configured server-owned local model provider.
- `profile_generation` and `prompt_generation` require a BYOK provider with
  `model` and `api_key`.
- Hosted and CLI case contracts use `settings.nutrition_targets` and
  `settings.verification_constraints`. Old `profile` and `constraints` fields
  are rejected as unknown fields.
- Supported `provider.type` values are `openai`, `anthropic`, `gemini`, and
  `openai_compatible`.
- Native providers use their official endpoints. `base_url` is honored only for
  `openai_compatible` custom endpoints.
- `repair_json` defaults to `true` for generation modes and allows one bounded
  repair attempt.
- Provider API keys are held only in memory while the queued run is pending and
  are never written to run metadata, reports, or artifact bundles.
- Pending BYOK inputs carry expiry metadata and may be discarded before worker
  execution; an expired pending input fails the run closed before provider
  invocation.
- `configs/redacted-provider.json` records provider type, base URL, and model
  with `api_key` set to `redacted`.
- `optional/llm-output.json` and `optional/normalization-events.json` are
  emitted only when a run used provider generation or normalization metadata.
- `optional/llm-output.json` must redact exact provider-key matches before
  persistence.
- `openai_compatible` sends the supplied API key to `base_url`; users must
  trust custom endpoint operators.

## Frontend Boundary Contract

The production frontend should be a static site deployed to Cloudflare Pages.
It may receive public build-time configuration such as:

- `VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.dev`
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
- artifacts expire after a short retention window, initially 7 days for live
  BYOK runs
- expired artifacts and metadata are deleted by a cleanup job
- privacy and safety defaults are defined in `docs/privacy-and-safety.md`

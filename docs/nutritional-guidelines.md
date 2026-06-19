# Nutritional Guidelines

This document defines the public nutrition sources MealCheck will use and how
those sources are normalized into machine-readable JSON.

MealCheck should not interpret public guidance live at runtime. It should use a
versioned guideline pack built from reviewed source material.

## Guiding Rule

MealCheck may only make strong nutrition judgments when a check is grounded in
one of:

- user-supplied constraint
- resolved nutrient data
- versioned guideline-pack rule
- trusted baseline

The product should say "this plan violates the configured sodium limit," not
"this is an unhealthy diet."

## Initial Guideline Pack

The first guideline pack is:

`dga-2025-2030-us-adult-general-v1`

The `schema_version` field remains the artifact-format version. The guideline
source and content version are encoded in `pack_id` so reports can distinguish
data shape changes from source-basis changes.

Audience:

- generally healthy adults
- non-clinical meal planning
- no pregnancy, lactation, pediatric, disease-specific, or therapeutic diet
  claims

Initial check domains:

- daily calorie target tolerance
- sodium limit
- added sugar limit
- saturated fat limit
- protein minimum when configured
- declared allergen exclusion
- declared food exclusion
- basic food-group coverage
- basic meal-prep safety reminders

## Source Set

The MVP source set should be limited to official U.S. public sources.

| Source | Role | Runtime Use |
| --- | --- | --- |
| Dietary Guidelines for Americans 2025-2030, via `https://www.dietaryguidelines.gov/` and `https://cdn.realfood.gov/DGA.pdf` | Primary general adult nutrition guidance and food-pattern framing | Preprocessed into guideline pack |
| USDA DRI Calculator, `https://www.nal.usda.gov/human-nutrition-and-food-safety/dri-calculator` | Profile-based calorie and nutrient recommendation reference | Preprocessed or manually encoded into target rules |
| USDA FoodData Central, `https://fdc.nal.usda.gov/` | Food and nutrient data | Local fixture for seeded runs; optional runtime API later |
| FDA Daily Values / Nutrition Facts label guidance, `https://www.fda.gov/food/food-labeling-nutrition` | General daily-value references for label-style limits | Preprocessed into guideline pack where useful |
| FDA Food Allergies, `https://www.fda.gov/food/food-allergensgluten-free-guidance-documents-regulatory-information/food-allergies` | Major allergen categories and synonym taxonomy | Preprocessed into allergen rules |
| FoodSafety.gov, `https://www.foodsafety.gov/` | Basic clean, separate, cook, chill, storage, and temperature guidance | Preprocessed into prep-safety checks |

Post-MVP optional source:

- Open Food Facts for packaged-food ingredients and allergens. This is useful
  for branded foods, but it is user-contributed data and should not be treated
  as the primary guideline source.

## Source Domains

Each source-derived rule belongs to an explicit domain.

| Domain | Example Checks | Claim Strength |
| --- | --- | --- |
| Nutrient limits | sodium, added sugar, saturated fat | Strong when nutrient data resolves |
| Nutrition targets | configured calories and protein minimum | Strong only when targets are present and assumptions are visible |
| Food exclusions | declared allergies, user-excluded foods | Strong for ingredient/name matches; warn when unresolved |
| Food groups | vegetables, fruit, grains, protein, dairy alternatives | Advisory or warn by default |
| Meal-prep safety | chill, reheat, storage, cross-contamination notes | Warn by default unless a clear unsafe instruction appears |
| Nutrition calculation | calories, protein, sodium, fats, sugars | Strong only for resolved foods and units |

Out of scope for the initial guideline pack:

- diabetes-specific recommendations
- hypertension treatment plans
- kidney disease diets
- pregnancy or lactation guidance
- pediatric guidance
- eating-disorder contexts
- supplements
- medication interactions
- guaranteed weight-loss outcomes

## Preprocessing Pipeline

Guideline preprocessing is a build-time or maintainer task.

1. **Register Sources**
   - Record title, URL, publisher, retrieval date, source version, and checksum
     when a file is downloaded.
   - Store source metadata in the guideline pack.

2. **Extract Candidate Rules**
   - Identify numeric thresholds, food-group guidance, allergen categories,
     storage rules, cooking temperatures, and explicit disclaimers.
   - Ignore broad prose that cannot become a bounded check.

3. **Classify Rule Strength**
   - `hard_check`: objective rule that can block a plan.
   - `soft_check`: useful rule that should warn.
   - `advisory`: visible guidance that should not affect decision by default.
   - `excluded`: source text intentionally not represented in MealCheck.

4. **Normalize Units**
   - Convert all numeric rules into canonical units.
   - Examples: `mg/day`, `g/day`, `%_calories/day`, `kcal/day`, `deg_f`,
     `hours`, and `servings/day`.

5. **Map To Nutrient Fields**
   - Map guideline nutrients to resolver output fields.
   - Example mappings:
     - sodium -> `sodium_mg`
     - saturated fat -> `saturated_fat_g`
     - added sugars -> `added_sugar_g`
     - calories -> `energy_kcal`
     - protein -> `protein_g`

6. **Define Applicability**
   - Record the audience and conditions for each rule.
   - Rules must declare whether they apply to all adults, only configured
     profiles, only specific food categories, or only meal-prep notes.

7. **Create Citations**
   - Assign stable citation IDs to every source-backed rule.
   - Reports cite rule IDs and source labels, not unstructured source snippets.

8. **Write Fixture Tests**
   - Add one passing and one failing fixture for each hard check.
   - Add warning fixtures for unresolved foods, vague units, and advisory
     domains.

9. **Review And Version**
   - Treat the guideline pack as source code.
   - Pack updates require review, changelog notes, and regenerated fixtures when
     rules change.

## Preprocessing Agent Prompt

Use this prompt when regenerating or reviewing a guideline pack with a chat agent
that has internet search. Prefer this over ad hoc source review.

```text
You are preparing a MealCheck guideline pack. MealCheck is a verifier, not a
medical nutrition advisor. Your job is to transform official public source
material into bounded, auditable JSON rules.

Target pack:

- pack_id: dga-2025-2030-us-adult-general-v1
- audience: generally healthy adults, non-clinical, age 18+
- out of scope: pediatric, pregnancy, lactation, disease-specific diets,
  therapeutic diets, eating-disorder contexts, supplements, medication
  interactions, guaranteed weight-loss claims

Use only official U.S. public sources unless explicitly told otherwise:

- Dietary Guidelines for Americans current edition from dietaryguidelines.gov
  or its official linked PDF
- USDA DRI Calculator from nal.usda.gov
- USDA FoodData Central API Guide from fdc.nal.usda.gov
- FDA Food Allergies page from fda.gov
- FoodSafety.gov 4 Steps to Food Safety page

Tasks:

1. Retrieve each source from its official URL.
2. Record source metadata:
   - source_id
   - title
   - publisher
   - URL
   - retrieved_at date
   - source version or content-current-as-of date when available
   - page, section, or table locator for each extracted claim
3. Extract only bounded rules that MealCheck can evaluate from structured meal
   plan JSON, resolved nutrient data, user constraints, or prep notes.
4. For each candidate rule, classify it as:
   - hard_check: objective rule that can block a plan
   - soft_check: source-backed threshold that should warn by default
   - advisory: useful guidance that should not affect final decision by default
   - excluded: source text intentionally not represented in MealCheck
5. Normalize numeric rules into canonical fields and units:
   - sodium -> sodium_mg, mg/day
   - saturated fat -> saturated_fat_pct_calories, pct_calories/day
   - added sugars -> added_sugar_g, g/meal when the current source states a
     per-meal threshold
   - protein -> protein_g_per_kg, g/kg/day, preserving both lower and upper
     bounds when the current source gives a range
   - dairy servings -> dairy, servings/day, including the calorie-pattern basis
   - vegetable servings -> vegetables, servings/day, including the
     calorie-pattern basis
   - fruit servings -> fruits, servings/day, including the calorie-pattern basis
   - whole grain servings -> whole_grains, servings/day, preserving lower and
     upper bounds when the current source gives a range
   - snack added sugar -> added_sugar_g plus the source serving equivalent
   - food safety temperature -> deg_f
   - food safety time -> hours
6. Capture all explicit, bounded limits from the selected source set. For the
   DGA 2025-2030 source, this includes at least:
   - protein target range
   - saturated fat percent-of-calories limit
   - added sugar per-meal limit
   - grain snack added-sugar equivalent limit
   - dairy snack added-sugar equivalent limit
   - sodium daily limit
   - dairy servings for the stated calorie pattern
   - vegetable servings for the stated calorie pattern
   - fruit servings for the stated calorie pattern
   - whole grain serving range
7. Capture all explicit, bounded food-safety limits from FoodSafety.gov that can
   be represented as prep-note, temperature, time, or storage checks. This
   includes at least:
   - 20-second handwashing
   - hot holding at or above 140 degrees F
   - temperature danger zone of 40 to 140 degrees F
   - microwave cooking at or above 165 degrees F
   - refrigerator at or below 40 degrees F
   - freezer at or below 0 degrees F
   - refrigeration within 2 hours
   - refrigeration within 1 hour above 90 degrees F
8. Extract the FDA major allergen taxonomy exactly as the current FDA source
   states it. Do not invent allergen thresholds. If the source says no threshold
   is established, record that as a limitation.
9. Extract only meal-prep safety rules that can be checked from prep notes, such
   as clean/separate/cook/chill framing and prompt refrigeration of leftovers.
10. Do not interpret broad source prose as a deterministic check. Put unsupported
   or vague material in excluded or advisory notes.
11. If a current source conflicts with an older project note, use the current
   source for the generated pack and list the conflict in the review notes.
12. Do not provide medical advice. Do not write disease-specific, pregnancy,
    pediatric, or therapeutic diet rules.

Output:

1. source-registry.json
   - registry_id
   - retrieved_at
   - sources[]
   - claims_used[] for each source, with claim_id, summary, and source_locator

2. guideline-pack.json
   - schema_version
   - pack_id
   - name
   - audience
   - source_documents[]
   - rules[]
   - disclaimer

For each rule, include:

- rule_id
- domain
- status
- strength
- severity
- nutrient, operator, value, unit, and period when numeric
- applicability
- user-facing message
- source_refs
- source_claims

Use these field conventions:

- `domain`: one of `nutrient_limit`, `profile_target`, `food_exclusion`,
  `meal_prep_safety`, `food_group_coverage`, `food_group_servings`,
  `snack_added_sugar_limit`, `food_safety_temperature`, `food_safety_time`, or
  `food_safety_storage`.
- `operator`: one of `<`, `<=`, `>`, `>=`, `=`, `between`, `contains_none`, or
  `requires_any`.
- `value`: number for simple thresholds, array for controlled lists, or object
  with `min` and `max` for ranges.
- `unit`: canonical unit such as `mg/day`, `pct_calories/day`, `g/meal`,
  `g/kg/day`, `servings/day`, `deg_f`, `hours`, `seconds`, or a source-specific
  equivalent such as `g_per_0_75_oz_whole_grain_equivalent`.
- `period`: `meal`, `day`, `snack`, `prep`, `storage`, `after_preparation`,
  `after_cooking`, `microwave_cooking`, or `storage_or_holding` as applicable.
- `applicability`: structured conditions such as age, calorie-pattern basis,
  food group, food category, storage type, temperature condition, or whether
  prep notes are required.
- `source_refs`: source document IDs present in `source_documents`.
- `source_claims`: claim IDs present in `source-registry.json`.

Quality bar:

- Every active source-backed rule must point to at least one source_id.
- Every active source-backed rule should point to at least one source claim ID.
- Every source_id in source_refs must exist in source_documents.
- Strong claims must be grounded in source material, user constraints, resolved
  nutrient data, or a trusted baseline.
- The pack must be reproducible without live web access after generation.
- Keep quotes short. Prefer paraphrased summaries with precise source locators.
```

## Guideline Pack JSON Shape

Initial pack shape:

```json
{
  "pack_id": "dga-2025-2030-us-adult-general-v1",
  "name": "DGA 2025-2030 U.S. Adult General MealCheck Guidelines",
  "audience": {
    "age_min_years": 18,
    "clinical_scope": "non_clinical_general_adult"
  },
  "source_documents": [
    {
      "source_id": "dga-2025-2030",
      "title": "Dietary Guidelines for Americans 2025-2030",
      "publisher": "USDA/HHS",
      "url": "https://cdn.realfood.gov/DGA.pdf",
      "retrieved_at": "2026-06-10"
    }
  ],
  "rules": [
    {
      "rule_id": "sodium_max_default",
      "domain": "nutrient_limit",
      "status": "active",
      "strength": "soft_check",
      "nutrient": "sodium_mg",
      "operator": "<",
      "value": 2300,
      "period": "day",
      "severity": "warn",
      "source_refs": ["dga-2025-2030"],
      "source_claims": ["dga-sodium-mg-per-day"]
    },
    {
      "rule_id": "added_sugar_max_10g_per_meal",
      "domain": "nutrient_limit",
      "status": "active",
      "strength": "soft_check",
      "nutrient": "added_sugar_g",
      "operator": "<=",
      "value": 10,
      "period": "meal",
      "severity": "warn",
      "source_refs": ["dga-2025-2030"],
      "source_claims": ["dga-added-sugar-grams-per-meal"]
    }
  ],
  "disclaimer": "MealCheck checks bounded guideline-derived rules. It does not provide medical nutrition advice."
}
```

## Runtime API Policy

Runtime internet APIs are allowed only for structured data lookup, not for live
guideline interpretation.

MVP:

- Use a checked-in fixture nutrient catalog for public seeded runs.
- Do not require network access for seeded demos or local tests.
- Do not require an API key for the first deterministic proof.

Post-MVP:

- Use FoodData Central API for live food search and nutrient details.
- Cache resolved foods by source ID, serving unit, and source release.
- Record API source IDs in artifacts.
- Treat API outages or unresolved foods as `warn` or `unresolved`, not as proof
  that a plan passes.

FoodData Central supports REST food search and food-details endpoints, requires
a data.gov API key, exposes OpenAPI JSON/YAML, and publishes downloadable data
files. MealCheck should prefer downloads or cached subsets for deterministic
tests and seeded public reports.

## Meal Plan JSON Normalization

Meal-plan normalization is separate from guideline-pack preprocessing.

All input modes must produce the same normalized meal-plan JSON before
evaluation:

- profile-only LLM generation
- prompt-based LLM generation
- manual structured entry

Normalization requirements:

- required day, meal, food, quantity, and unit fields
- canonical meal names where possible
- canonical units where supported
- unresolved reason when food identity, quantity, or unit is unclear
- no trusted nutrient totals from the LLM
- original LLM output retained in artifacts when an LLM was used

The verifier evaluates only normalized JSON. Natural language can be accepted as
prompt input or report output, but it is not the auditable meal-plan contract.

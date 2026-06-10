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

`us-adult-general-v1`

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
| Profile targets | estimated calories, optional protein target | Strong only when profile inputs are present and assumptions are visible |
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

## Guideline Pack JSON Shape

Initial pack shape:

```json
{
  "pack_id": "us-adult-general-v1",
  "name": "U.S. Adult General MealCheck Guidelines",
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
      "strength": "hard_check",
      "nutrient": "sodium_mg",
      "operator": "<=",
      "value": 2300,
      "period": "day",
      "severity": "block",
      "source_refs": ["dga-2025-2030"]
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

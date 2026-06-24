# MealCheck Evaluation Dataset

MealCheck uses evaluation datasets to expand the local nutrient catalog from
measured resolver gaps and reviewed source data rather than from intuition.
There are two checked-in layers:

1. an FNDDS-grounded synthetic regression layer for targeted workflow behavior
2. a WWEIA/NHANES dietary recall layer for real reported intake patterns

The local catalog is still intentionally bounded. It is not trying to replace
FoodData Central. Its job is fast, deterministic coverage for common foods,
CI-safe demos, and a clear basis for deciding which long-tail foods should
remain unresolved or move to a future API-backed lookup path.

## Source Data

FNDDS source:

- USDA FNDDS 2021-2023 At A Glance, Foods and Beverages workbook
- USDA FNDDS 2021-2023 At A Glance, Nutrient Values workbook
- USDA FNDDS 2021-2023 At A Glance, Portions and Weights workbook

WWEIA/NHANES source:

- NHANES August 2021-August 2023 `DR1IFF_L.xpt`, Dietary Interview -
  Individual Foods, First Day
- NHANES August 2021-August 2023 `DR2IFF_L.xpt`, Dietary Interview -
  Individual Foods, Second Day
- NHANES August 2021-August 2023 `DEMO_L.xpt`, Demographics, used for adult
  age filtering

Source pages:

- `https://www.ars.usda.gov/northeast-area/beltsville-md-bhnrc/beltsville-human-nutrition-research-center/food-surveys-research-group/docs/fndds-download-databases/`
- `https://wwwn.cdc.gov/nchs/nhanes/search/datapage.aspx?Component=Dietary`
- `https://fdc.nal.usda.gov/download-datasets/`

FNDDS is appropriate for the local catalog because it is based on foods and
beverages reported in WWEIA/NHANES and includes both nutrient values and
familiar household portions. WWEIA/NHANES is appropriate for the real-recall
evaluation layer because it contains public, deidentified 24-hour dietary
interview rows with participant/day, eating occasion, food code, grams, and
nutrient values.

The source workbooks and XPT files are not checked into the repository.
Regeneration expects them at:

```bash
/tmp/mealcheck-fndds-2021-2023/foods-and-beverages.xlsx
/tmp/mealcheck-fndds-2021-2023/nutrient-values.xlsx
/tmp/mealcheck-fndds-2021-2023/portions-and-weights.xlsx
/tmp/mealcheck-nhanes-2021-2023/DR1IFF_L.xpt
/tmp/mealcheck-nhanes-2021-2023/DR2IFF_L.xpt
/tmp/mealcheck-nhanes-2021-2023/DEMO_L.xpt
```

## Artifacts

- `data/nutrients/fixture-catalog-v1.json`: 151-food reviewed local catalog
  generated from FNDDS source rows.
- `data/evaluation/fndds-grounded-meal-plans-v1.json`: 100 structured
  meal-plan cases with source-like text, normalized plan JSON, settings, tags,
  source refs, and expected outcomes.
- `data/evaluation/wweia-nhanes-real-recalls-v1.json`: 100 real-recall
  evaluation cases generated from adult reliable WWEIA/NHANES dietary
  interview rows. Cases preserve reported gram weights and eating occasions.
- `data/evaluation/results/baseline-17-foods.json`: evaluation of the 100-case
  FNDDS-grounded dataset against the original 17-food local catalog.
- `data/evaluation/results/fndds-grounded-catalog-v1.json`: evaluation of the
  FNDDS-grounded dataset against the expanded FNDDS-grounded catalog.
- `data/evaluation/results/wweia-nhanes-real-recalls-v1.json`: evaluation of
  the WWEIA/NHANES real-recall dataset against the expanded local catalog.
- `cmd/mealcheck-eval`: deterministic runner for case coverage, unresolved food
  frequency, category summaries, and expected-outcome mismatches.
- `scripts/generate-fndds-evaluation.py`: reproducible generator for the
  expanded fixture catalog and FNDDS-grounded evaluation dataset.
- `scripts/generate-wweia-nhanes-evaluation.py`: reproducible generator for the
  WWEIA/NHANES real-recall evaluation dataset.

## Dataset Mix

The FNDDS-grounded dataset contains:

- 20 balanced common-food plans
- 15 vegetarian plans
- 10 vegan plans
- 10 high-sodium plans
- 10 high-added-sugar plans
- 10 allergen-risk plans
- 10 low-protein plans
- 8 long-tail unresolved plans
- 7 vague-quantity plans

Each case contains exactly one day and three meals. This keeps individual cases
inspectable while still creating a 900-item resolver workload.

The WWEIA/NHANES real-recall dataset contains:

- 40 fully resolved real eating-occasion cases
- 30 high-coverage full adult recall days
- 20 high-sodium full adult recall days
- 10 low-protein full adult recall days

The real-recall layer uses adult reliable recalls only. It intentionally mixes
resolved real eating occasions with full-day recall cases that expose catalog
gaps. It should not be described as a meal-plan dataset. It is a structured
dietary recall dataset transformed into MealCheck evaluation cases.

## Current Results

The original 17-food catalog resolves 296 of 900 items, a 32.89% resolver rate,
against the FNDDS-grounded dataset. This baseline intentionally shows the limit
of the MVP fixture catalog.

The expanded FNDDS-grounded catalog resolves 885 of 900 items, a 98.33%
resolver rate, with zero expected-outcome mismatches. The 15 unresolved items
are intentional:

- 8 long-tail foods that should stay unresolved until a FoodData Central
  fallback exists
- 7 vague quantities that should block until the user provides a quantity/unit

Against the WWEIA/NHANES real-recall dataset, the expanded catalog resolves
496 of 815 items, a 60.86% resolver rate, with zero expected-outcome
mismatches. This lower rate is expected: real dietary recalls include tap
water, mixed dishes, alcohol, branded-like snack variants, condiments, and
specific prepared-food forms that the bounded local catalog does not yet cover.
The top gap candidates currently include tap water, white rolls, granulated
sugar, wine, apple juice, instant coffee, saltine crackers, flavored liquid
coffee creamer, and common mixed dishes.

## Catalog Expansion Policy

Expansion rules:

1. Add foods in reviewed batches driven by evaluation coverage and unresolved
   frequency.
2. Keep matching conservative: exact normalized names plus reviewed aliases.
3. Keep units per food explicit; unsupported units remain unresolved.
4. Mark ambiguous, branded, vague, and long-tail foods unresolved instead of
   guessing.
5. Store source references on every generated food.
6. Keep source workbooks out of CI; check in deterministic generated JSON.

FNDDS At A Glance does not publish added-sugar grams. MealCheck therefore uses
a documented proxy in the generated fixture: naturally sweet foods and plain
dairy receive `added_sugar_g = 0`, while explicitly sweetened categories such
as soda, candy, cookies, syrup, sweetened drinks, and higher-sugar cereal use
FNDDS total sugar as a conservative added-sugar proxy. This is adequate for
resolver and workflow evaluation, but it should be replaced by FPED or another
reviewed added-sugar source before making broader nutrition claims.

## Running Evaluation

Regenerate the catalog and dataset after downloading the FNDDS workbooks:

```bash
python3 scripts/generate-fndds-evaluation.py
```

Regenerate the WWEIA/NHANES real-recall layer after downloading the NHANES XPT
files and the FNDDS foods workbook:

```bash
python3 scripts/generate-wweia-nhanes-evaluation.py
```

Run the expanded catalog evaluation:

```bash
go run ./cmd/mealcheck-eval
```

Write a result artifact:

```bash
go run ./cmd/mealcheck-eval \
  -out data/evaluation/results/fndds-grounded-catalog-v1.json
```

Write the WWEIA/NHANES result artifact:

```bash
go run ./cmd/mealcheck-eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -out data/evaluation/results/wweia-nhanes-real-recalls-v1.json
```

Run a comparison against another catalog:

```bash
go run ./cmd/mealcheck-eval \
  -catalog /path/to/catalog.json \
  -out data/evaluation/results/custom.json \
  -allow-mismatch
```

`-allow-mismatch` is useful for baseline or exploratory runs where expected
outcomes are not supposed to pass yet.

## CI Role

`go run ./cmd/mealcheck-fixture-check` verifies that:

- the expanded catalog has at least 100 foods
- catalog food IDs, names, and aliases do not collide
- allergens and food groups use reviewed labels
- every food has a positive gram conversion
- each checked-in evaluation dataset contains exactly 100 cases
- required evaluation categories are present in each dataset
- each case has expected outcomes and food items
- WWEIA/NHANES cases retain source refs and source metrics

The strict evaluation can be used as a release gate:

```bash
go run ./cmd/mealcheck-eval
```

The WWEIA/NHANES layer can also be run as a release or catalog-expansion gate:

```bash
go run ./cmd/mealcheck-eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json
```

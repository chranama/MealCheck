# MealCheck Evaluation Dataset

MealCheck uses evaluation datasets to expand the local nutrient catalog from
measured resolver gaps and reviewed source data rather than from intuition.
There are two checked-in layers:

1. an FNDDS-grounded synthetic regression layer for targeted workflow behavior
2. a WWEIA/NHANES dietary recall layer for real reported intake patterns

The local catalog is still intentionally bounded. It is not trying to replace
FoodData Central. Its job is fast, deterministic coverage for common foods,
CI-safe demos, and a clear basis for deciding which long-tail foods should
remain unresolved, resolve through the conservative FNDDS fallback, or move to
a future API-backed lookup path.

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
- `data/evaluation/results/wweia-nhanes-real-recalls-with-fndds-fallback-v1.json`:
  coverage run for the same real-recall dataset with the optional FNDDS SQLite
  fallback enabled.
- `mealcheck eval`: deterministic runner for case coverage, unresolved food
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

With the optional FNDDS SQLite fallback enabled, the same WWEIA/NHANES
real-recall dataset resolves 688 of 815 items, an 84.42% resolver rate. This is
a coverage run, not a strict expected-outcome regression, because the checked-in
WWEIA expected unresolved counts describe the no-fallback catalog mode. The
fallback removes common source-backed gaps such as water, 100% juice, instant
coffee, white rolls, granulated sugar, no-added-fat vegetables and rice, and
sandwich vegetable components while leaving ambiguous, composed,
restaurant/product-style, review-required, and unsupported-unit rows unresolved.

## Catalog Expansion Policy

Expansion rules:

1. Add foods in reviewed batches driven by evaluation coverage and unresolved
   frequency.
2. Keep matching conservative: exact normalized names plus preprocessed,
   auto-approved match keys.
3. Keep units per food explicit and source-backed; unsupported units remain
   unresolved.
4. Mark ambiguous, branded, vague, and long-tail foods unresolved instead of
   guessing.
5. Store source references on every generated food.
6. Keep source workbooks out of CI; check in deterministic generated JSON.

Runtime lookup order:

1. Use the reviewed local catalog first.
2. If no reviewed catalog match exists, pass the item through the fallback
   resolver gate. The gate only allows quantified foods with units supported by
   the fallback surface and names that appear specific enough for automatic
   lookup.
3. The gate blocks broad one-word foods, mixed dishes that need ingredient
   decomposition, restaurant or branded foods, unclear preparation, non-food
   text, and unsupported fallback units.
4. If the gate allows lookup, optionally check the FNDDS SQLite fallback.
5. The fallback only resolves preprocessed FNDDS match keys whose
   `resolver_status` is `auto` and whose match is unique.
6. The fallback supports gram units plus source-backed food-specific unit
   conversions derived from FNDDS Portions and Weights.
7. Explicit `unknown_food` items with quantities may retry the fallback.
   Explicit vague quantities, unsupported units, and other unresolved reasons
   remain unresolved.
8. Quarantined or review-required FNDDS rows are never runtime resolver hits
   unless a specific generated match key is explicitly marked `auto`; broad keys
   that map to multiple foods fail closed.

Unresolved verification policy:

- By default, any unresolved food or quantity remains in `unresolved-foods.json`
  and blocks `quantities_resolvable`.
- When `settings.verification_constraints.unresolved_policy.de_minimis_enabled`
  is explicitly enabled, MealCheck may exclude tiny unresolved mass items from
  nutrition totals only when the item has deterministic `g`, `gram`, `grams`,
  `oz`, `ounce`, or `ounces` quantity, has `unknown_food` or
  `missing_conversion:<unit>` as its unresolved reason, and stays within
  per-item, per-day, and per-day-count caps.
- De minimis exclusion is disabled when allergy or excluded-food constraints
  are configured.
- Excluded unresolved items are written to `excluded-unresolved-foods.json` and
  make `quantities_resolvable` warn rather than pass. They are never counted in
  nutrition totals.

FNDDS At A Glance does not publish added-sugar grams. MealCheck therefore uses
a documented proxy in the generated fixture: naturally sweet foods and plain
dairy receive `added_sugar_g = 0`, while explicitly sweetened categories such
as soda, candy, cookies, syrup, sweetened drinks, and higher-sugar cereal use
FNDDS total sugar as a conservative added-sugar proxy. This is adequate for
resolver and workflow evaluation, but it should be replaced by FPED or another
reviewed added-sugar source before making broader nutrition claims.

The SQLite fallback uses FNDDS total sugar as a conservative `added_sugar_g`
proxy for all fallback rows because FNDDS At A Glance does not publish
added-sugar grams.

## FNDDS Reference Database

MealCheck also keeps a full local FNDDS 2021-2023 reference layer. This is not
the reviewed resolver catalog. Its job is to preserve all source rows and
precompute which foods are plausible future resolver candidates.

Artifacts:

- `data/reference/fndds/source-manifest.json`: source URLs, expected raw file
  paths, and generation command.
- `data/reference/fndds-2021-2023/foods.jsonl`: all imported FNDDS food rows
  with descriptions, category metadata, nutrients, portions, source refs, and
  candidate classification.
- `data/reference/fndds-2021-2023/nutrients.jsonl`: normalized nutrient rows.
- `data/reference/fndds-2021-2023/portions.jsonl`: normalized portion rows.
- `data/reference/fndds-2021-2023/resolver-candidates.jsonl`: eligible
  source-backed foods for future review.
- `data/reference/fndds-2021-2023/resolver-match-keys.jsonl`: canonical and
  alias match keys with resolver status, confidence, and block reason.
- `data/reference/fndds-2021-2023/unit-conversions.jsonl`: source-backed
  food-specific gram conversions derived from FNDDS portions.
- `data/reference/fndds-2021-2023/quarantined-foods.jsonl`: ambiguous,
  composed, restaurant/product-style, or unclear-preparation foods.
- `data/reference/fndds-2021-2023/review-required-foods.jsonl`: source rows
  that need manual handling before candidate use.
- `data/reference/fndds-2021-2023/classification-summary.json`: counts by
  status and ambiguity flag.
- `data/reference/fndds-2021-2023/fndds.sqlite`: indexed read-only runtime
  fallback database generated from the same reference rows.

Current FNDDS reference import:

- 5,432 food rows preserved
- 3,056 eligible resolver candidates
- 2,375 quarantined rows
- 6,201 resolver match keys
- 25,928 source-backed unit conversions
- 1 review-required row

The preprocessing classifier quarantines rows with signals such as `NFS`,
not-specified descriptions, mixed dishes, sandwiches, pizza, burritos,
restaurant/product-style wording, unclear added-fat preparation, and
multi-component allergen risk. Quarantined rows are not deleted; they remain
available for frequency mining and manual review pressure.

Regenerate the FNDDS reference layer after downloading the FNDDS workbooks:

```bash
python3 scripts/import-fndds-reference.py
```

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
go run ./cmd/mealcheck eval
```

Write a result artifact:

```bash
go run ./cmd/mealcheck eval \
  -out data/evaluation/results/fndds-grounded-catalog-v1.json
```

Write the WWEIA/NHANES result artifact:

```bash
go run ./cmd/mealcheck eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -out data/evaluation/results/wweia-nhanes-real-recalls-v1.json
```

Write the WWEIA/NHANES FNDDS fallback coverage artifact:

```bash
go run ./cmd/mealcheck eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite \
  -skip-expected \
  -out data/evaluation/results/wweia-nhanes-real-recalls-with-fndds-fallback-v1.json
```

Run a comparison against another catalog:

```bash
go run ./cmd/mealcheck eval \
  -catalog /path/to/catalog.json \
  -out data/evaluation/results/custom.json \
  -allow-mismatch
```

`-allow-mismatch` is useful for baseline or exploratory runs where expected
outcomes are not supposed to pass yet.

## CI Role

`go run ./cmd/mealcheck fixture-check` verifies that:

- the expanded catalog has at least 100 foods
- catalog food IDs, names, and aliases do not collide
- allergens and food groups use reviewed labels
- every food has a positive gram conversion
- each checked-in evaluation dataset contains exactly 100 cases
- required evaluation categories are present in each dataset
- each case has expected outcomes and food items
- WWEIA/NHANES cases retain source refs and source metrics
- FNDDS reference artifacts exist and have internally consistent counts
- the FNDDS SQLite fallback has table counts, indexes, known statuses, and
  resolver examples consistent with the generated JSONL reference artifacts
- eligible FNDDS resolver candidates do not carry hard quarantine flags
- known ambiguous and allowlisted FNDDS examples classify as expected

The strict evaluation can be used as a release gate:

```bash
go run ./cmd/mealcheck eval
```

The WWEIA/NHANES layer can also be run as a release or catalog-expansion gate:

```bash
go run ./cmd/mealcheck eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json
```

The FNDDS fallback coverage run is useful for catalog expansion analysis but is
not a strict release gate:

```bash
go run ./cmd/mealcheck eval \
  -dataset data/evaluation/wweia-nhanes-real-recalls-v1.json \
  -fndds-fallback data/reference/fndds-2021-2023/fndds.sqlite \
  -skip-expected
```

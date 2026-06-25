# Meal Plan Input Robustness

This document defines the acceptable pasted-meal-plan input boundary and the
first synthetic dataset for normalization robustness testing.

## Term

Use `input robustness testing` for product discussion. For implementation and
test artifacts, prefer the more specific term `meal-plan normalization
evaluation`.

This is not the same as adversarial testing. Adversarial and invalid inputs
belong in qualification tests. This document covers acceptable inputs that
should be normalized successfully.

## Link To User Story

The primary hosted user pastes a concise ingredient-level meal plan and asks
MealCheck to verify it. The user story depends on two contracts:

1. Qualification decides whether text is specific enough to verify.
2. Normalization turns acceptable text into canonical MealCheck meal-plan JSON.

This robustness work tests the second contract. It assumes the input has already
crossed the "good meal plan" boundary.

## Good Meal Plan Definition

A good MealCheck meal-plan input is natural language or semi-structured text
that contains enough concrete information to normalize without guessing
nutrition-critical details.

Required:

- day coverage, either explicit day labels or an unambiguous one-day plan
- meal labels for each day
- food or ingredient names for each meal
- numeric quantities for each food item
- units that can be normalized to MealCheck-supported units
- enough line, sentence, or delimiter structure to associate each food item
  with the correct day and meal

Ideal multi-day format:

- start each day with an explicit label such as `Day 1`, `Day 2`, and `Day 3`
- keep day labels contiguous and in order for the requested verification window
- group breakfast, lunch, dinner, and any snacks under the day they belong to
- keep each food item close to its quantity and unit
- avoid cross-day shorthand such as "repeat yesterday's lunch" or "same snacks"

Clear `Day N` sections let the hosted local-model path decompose a multi-day
input into one model call per day, then merge the normalized days back into one
canonical plan. If the day boundaries are ambiguous or incomplete, the backend
uses the unbatched whole-plan fallback instead of silently guessing.

Supported unit vocabulary for the first robustness dataset:

- `g` or grams
- `oz` or ounces
- `cup` or cups
- `tbsp` or tablespoons
- `tsp` or teaspoons
- `slice` or slices
- `serving` or servings

Acceptable variability:

- bullets, numbered lists, or short inline sentences
- fractional or decimal quantities
- one to seven days
- three meals per day, or snack-inclusive plans up to six meals per day
- repeated foods across days
- common preparation adjectives such as grilled, cooked, steamed, baked, plain,
  roasted, or mixed

Not acceptable for this dataset:

- vague meal names without ingredients or quantities
- recipes that need decomposition into ingredients
- nutrition totals without food items
- foods with no quantity
- quantities with unsupported units such as handful, bowl, scoop, or
  small/medium/large
- clinical, pediatric, pregnancy, or disease-specific scenarios
- inputs longer than the hosted character limit

Inputs outside this boundary should be tested as qualification failures or
`eligible_with_unresolved_items` cases, not as successful normalization cases.

Qualification failure classes:

- `not_meal_plan`: text does not describe days, meals, recipes, or
  ingredient-level meal-plan content.
- `meal_plan_too_vague`: text resembles a meal plan, but lacks numeric
  quantities or supported units needed for verification.
- `recipe_or_menu_needs_decomposition`: text is recipe-like, but is not
  decomposed into day, meal, ingredient, quantity, and unit fields.

Hosted local-model run creation should fast-fail these cases before queueing a
run or calling the model. If an input passes preflight but model normalization
still fails, the public run error should stay user-facing and guidance-oriented;
debug artifacts may retain sanitized model and parser details for operators.

## Dataset

The first synthetic acceptable-input dataset lives at:

```text
examples/meal-plan-input-robustness/
```

It includes:

- a manifest with expected day counts, meal-code coverage, item counts, and
  coverage tags
- one-day canonical bullet input
- one-day inline sentence input
- one-day numbered-list input
- two-day bullet input
- one-day snack-inclusive input
- three-day compact inline input

The dataset intentionally avoids invalid examples. It should remain small
enough to read and diagnose by hand.

A companion qualification-failure set lives at:

```text
examples/meal-plan-input-robustness/failure-manifest.json
```

That set is for pre-model refusal behavior, not successful normalization.

## Evaluation Expectations

For each acceptable input case, a normalization run should:

- return valid compact row JSON
- preserve each expected source item exactly once
- preserve the expected day and meal association
- preserve numeric quantities as numbers
- normalize unit strings into supported units
- expand through the MealCheck adapter into canonical plan JSON
- run through deterministic verification without schema or loader failure

Run-level pass/fail should be based on normalization correctness, not on whether
the meal plan receives a `pass`, `warn`, or `block` decision. A synthetically
valid meal plan may still fail nutrition, allergen, or prep-safety checks.

## Public Copy Alignment

Public copy should stay non-technical. It should say that MealCheck works best
with plans that list days, meals, foods or ingredients, and approximate amounts.
For multi-day plans, it should encourage clear `Day 1`, `Day 2`, and `Day 3`
style labels. It should not mention schemas, compact rows, model contracts,
APIs, batching, fallback paths, or internal adapters.

## Future Dataset Slices

After the acceptable-input set is stable, add separate datasets for:

- vague-but-meal-like text
- recipe-like text that needs decomposition
- unsupported or ambiguous quantities
- borderline character-limit inputs
- allergen and excluded-food stress cases
- manually reviewed real-world examples, if privacy and permission allow it

# Expected Evidence

This fixture is the seeded one-day proof case. It should produce a final
`block` decision while matching the current hosted product shape: one day of
meal-plan content with three meals.

Expected block evidence:

- `allergens_absent`: day 1 lunch contains `peanut sauce`; the user declared a
  peanut allergy.
- `quantities_resolvable`: day 1 lunch includes `seasoning blend` with
  `quantity_text` set to `some` and `unresolved_reason` set to
  `vague_quantity`.

Expected warning evidence:

- `sodium_under_limit`: day 1 should exceed 2,300 mg sodium after resolving
  `instant ramen`, `soy sauce`, and other day 1 items.
- `calories_within_tolerance`: day 1 should fall below the configured
  15 percent calorie tolerance.
- `protein_minimum_met`: day 1 should fall below the configured 98 g protein
  minimum after resolving the FNDDS-grounded catalog values.
- `prep_safety_mentions_present`: the candidate prep notes do not mention
  prompt refrigeration or the 2-hour leftover window.

Expected pass or non-block evidence:

- The candidate has one day and three meals.
- The candidate does not contain shellfish.
- The candidate uses the same normalized meal-plan contract as the baseline.

The nutrient catalog is a deterministic fixture. It is not a broad nutrition
database and should not be treated as a FoodData Central replacement.

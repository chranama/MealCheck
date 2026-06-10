# Expected Evidence

This fixture is the first Milestone 0 proof case. It should produce a final
`block` decision once the checker exists.

Expected block evidence:

- `allergens_absent`: day 2 lunch contains `peanut sauce`; the user declared a
  peanut allergy.
- `quantities_resolvable`: day 2 lunch includes `seasoning blend` with
  `quantity_text` set to `some` and `unresolved_reason` set to
  `vague_quantity`.

Expected warning evidence:

- `sodium_under_limit`: day 2 should exceed 2,300 mg sodium after resolving
  `instant ramen`, `soy sauce`, and other day 2 items.
- `calories_within_tolerance`: days 1 and 3 should fall below the configured
  15 percent calorie tolerance.
- `prep_safety_mentions_present`: the candidate prep notes do not mention
  prompt refrigeration or the 2-hour leftover window.

Expected pass or non-block evidence:

- The candidate has three days and three meals per day.
- The candidate does not contain shellfish.
- The candidate uses the same normalized meal-plan contract as the baseline.

The nutrient catalog is a deterministic fixture. It is not a broad nutrition
database and should not be treated as a FoodData Central replacement.

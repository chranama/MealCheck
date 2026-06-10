# MealCheck Report

- Case: `seeded-3-day-peanut-allergy`
- Decision: `block`
- Guideline pack: `dga-2025-2030-us-adult-general-v1`

## Summary

The candidate plan has blocking issues and should be revised before use.

## Checks Requiring Attention

- `quantities_resolvable` block: The candidate includes unresolved food quantities or units.
- `allergens_absent` block: The candidate includes a declared allergen.
- `calories_within_tolerance` warn: One or more days are outside the configured calorie tolerance.
- `sodium_under_limit` warn: One or more days exceed the configured sodium limit.
- `prep_safety_mentions_present` warn: Prep notes do not mention prompt refrigeration or leftover handling.

## Unresolved Foods

- Day 2 lunch: `seasoning blend` (vague_quantity)

## Daily Totals

- Day 1: 1572.6 kcal, 132.9 g protein, 465.0 mg sodium
- Day 2: 1746.6 kcal, 101.6 g protein, 5558.2 mg sodium
- Day 3: 1301.0 kcal, 108.9 g protein, 347.3 mg sodium

## Disclaimer

MealCheck checks bounded guideline-derived rules. It does not provide medical nutrition advice.

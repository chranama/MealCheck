# MealCheck Report

- Case: `seeded-one-day-peanut-allergy`
- Decision: `block`
- Guideline pack: `dga-2025-2030-us-adult-general-v1`

## Summary

The candidate plan has blocking issues and should be revised before use.

## Checks Requiring Attention

- `quantities_resolvable` block: The candidate includes unresolved food quantities or units.
- `allergens_absent` block: The candidate includes a declared allergen.
- `calories_within_tolerance` warn: One or more days are outside the configured calorie tolerance.
- `sodium_under_limit` warn: One or more days exceed the configured sodium limit.
- `protein_minimum_met` warn: One or more days are below the configured protein minimum.
- `prep_safety_mentions_present` warn: Prep notes do not mention prompt refrigeration or leftover handling.

## Unresolved Foods

- Day 1 lunch: `seasoning blend` (vague_quantity)

## Excluded Unresolved Foods

None.

## Estimated And Decomposed Foods

None.

## Daily Totals

- Day 1: 1275.0 kcal, 90.5 g protein, 5052.9 mg sodium

## Disclaimer

MealCheck checks bounded guideline-derived rules. It does not provide medical nutrition advice.

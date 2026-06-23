# Meal Plan Input Robustness Dataset

This directory contains synthetic meal-plan text inputs for normalization
robustness testing.

The dataset is intentionally consumer-text-shaped rather than canonical JSON.
Each case should satisfy the internal "good meal plan" boundary documented in
`docs/meal-plan-input-robustness.md`: it names days, meals, foods, quantities,
and units clearly enough that MealCheck can normalize the text before running
deterministic verification.

## Files

- `manifest.json`: case metadata, expected item counts, day counts, and coverage
  tags.
- `cases/*.txt`: synthetic user-submitted meal-plan text.

## Use

Use this set after the single local llama smoke test passes. The smoke test
proves the basic compact JSON contract; this dataset is for broader extraction
coverage.

Suggested progression:

1. Run the existing one-case smoke test.
2. Run each one-day, three-meal case through the existing local llama smoke
   script by setting `MEALCHECK_LLAMA_PROMPT_FILE`, or use
   `scripts/test-meal-plan-input-robustness.sh`.
3. Use the multi-day and snack cases for hosted local-model regression once the
   harness supports per-case expected days and meal-code sets.

The dataset does not include intentionally invalid inputs. Invalid and vague
inputs belong in qualification tests, not in this acceptable-input normalization
set.

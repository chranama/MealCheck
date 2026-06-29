# Meal Plan Input Robustness Dataset

This directory contains synthetic meal-plan text inputs for normalization
robustness testing.

The dataset is intentionally consumer-text-shaped rather than canonical JSON.
Each case should satisfy the internal "good meal plan" boundary documented in
`docs/meal-plan-input-robustness.md`: it names days, meals, foods, quantities,
and units clearly enough that MealCheck can normalize the text before running
deterministic verification.

For multi-day cases, prefer explicit `Day 1`, `Day 2`, and `Day 3` labels with
meals and ingredient quantities grouped under the matching day. That shape
exercises the hosted local-model per-day decomposition path. Ambiguous but still
acceptable multi-day text should be tracked separately because it exercises the
unbatched whole-plan fallback.

## Files

- `manifest.json`: case metadata, expected item counts, day counts, and coverage
  tags.
- `cases/*.txt`: synthetic user-submitted meal-plan text.
- `failure-manifest.json`: failure metadata for inputs that should be refused
  during qualification or deterministic normalization.
- `failure-cases/*.txt`: synthetic invalid, vague, or recipe-like user inputs.

## Use

Use this set after the single local llama smoke test passes. The smoke test
proves the basic compact JSON contract; this dataset is for broader extraction
coverage.

Suggested progression:

1. Run the existing one-case smoke test.
2. Run each one-day, three-meal case through the existing local llama smoke
   script by setting `MEALCHECK_LLAMA_PROMPT_FILE`, or use
   `scripts/test-meal-plan-input-robustness.sh`.
3. Use the multi-day and snack cases for hosted local-model regression,
   especially after changes to day-section splitting or the unbatched fallback.

The Go hosted test suite also reads the manifest and checks the deterministic
source-item inventory for every acceptable case. That coverage is intentionally
model-free: it verifies the item count, day coverage, meal-code coverage, and
prompt item-count instruction before a local model is involved.

The acceptable-input manifest does not include intentionally invalid inputs.
Invalid and vague inputs belong in the failure set, not in the successful
normalization set. The separate `failure-manifest.json` tracks representative
fast-fail cases and whether they should fail at qualification or deterministic
normalization.

#!/usr/bin/env bash
#
# Run the MealCheck acceptable-input robustness cases that are compatible with
# the current one-day, three-meal local llama smoke harness.

set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="${MEALCHECK_INPUT_ROBUSTNESS_MANIFEST:-$ROOT/examples/meal-plan-input-robustness/manifest.json}"
OUTPUT_ROOT="${MEALCHECK_INPUT_ROBUSTNESS_OUTPUT_DIR:-/tmp/mealcheck-input-robustness-$(date +%Y%m%d-%H%M%S)}"
REPEATS="${MEALCHECK_INPUT_ROBUSTNESS_REPEATS:-1}"

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "error: required command not found: $name" >&2
    exit 1
  fi
}

require_command jq
require_command bash

if [[ ! -f "$MANIFEST" ]]; then
  echo "error: manifest not found: $MANIFEST" >&2
  exit 1
fi

mkdir -p "$OUTPUT_ROOT"

mapfile -t cases < <(jq -r '
  .cases[]
  | select(.smoke_harness_compatible == true)
  | [.id, .file, .expected_item_count]
  | @tsv
' "$MANIFEST")

if [[ "${#cases[@]}" -eq 0 ]]; then
  echo "error: no smoke-compatible robustness cases found in $MANIFEST" >&2
  exit 1
fi

failures=0

printf "manifest=%s\n" "$MANIFEST"
printf "output_root=%s\n" "$OUTPUT_ROOT"
printf "repeats=%s\n" "$REPEATS"

for row in "${cases[@]}"; do
  IFS=$'\t' read -r id file expected_item_count <<<"$row"
  prompt_file="$ROOT/examples/meal-plan-input-robustness/$file"
  case_output_dir="$OUTPUT_ROOT/$id"

  printf "\n=== robustness case: %s ===\n" "$id"
  printf "prompt_file=%s\n" "$prompt_file"
  printf "expected_item_count=%s\n" "$expected_item_count"

  if [[ ! -f "$prompt_file" ]]; then
    echo "FAIL: prompt file missing: $prompt_file" >&2
    failures=$((failures + 1))
    continue
  fi

  if ! MEALCHECK_LLAMA_PROMPT_FILE="$prompt_file" \
    MEALCHECK_LLAMA_EXPECTED_ITEM_COUNT="$expected_item_count" \
    MEALCHECK_LLAMA_OUTPUT_DIR="$case_output_dir" \
    MEALCHECK_LLAMA_REPEATS="$REPEATS" \
    "$ROOT/scripts/test-local-llama-structured-json.sh"; then
    failures=$((failures + 1))
  fi
done

if [[ "$failures" -gt 0 ]]; then
  echo "FAIL: $failures robustness case(s) failed"
  exit "$failures"
fi

echo "PASS: smoke-compatible meal-plan input robustness cases passed"

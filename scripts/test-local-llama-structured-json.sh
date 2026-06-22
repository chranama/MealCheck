#!/usr/bin/env bash
#
# Run a local llama.cpp compact JSON smoke test for MealCheck.
#
# This script assumes llama-server is already running locally, usually on the
# deployed MacBook server at http://127.0.0.1:11435/v1. It does not download
# models or start llama-server; the trial matrix should start one candidate
# GGUF at a time and then run this script from the MealCheck repository root.

set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BASE_URL="${LLAMA_BASE_URL:-http://127.0.0.1:11435/v1}"
BASE_URL="${BASE_URL%/}"
MODEL="${LLAMA_MODEL:-local-llama}"
PROMPT_FILE="${MEALCHECK_LLAMA_PROMPT_FILE:-$ROOT/examples/local-llama/synthetic-meal-plan.txt}"
SCHEMA_PATH="${MEALCHECK_LLAMA_SCHEMA_PATH:-$ROOT/examples/local-llama/compact-meal-plan-response.schema.json}"
OUTPUT_DIR="${MEALCHECK_LLAMA_OUTPUT_DIR:-/tmp/mealcheck-local-llama-$(date +%Y%m%d-%H%M%S)}"
REPEATS="${MEALCHECK_LLAMA_REPEATS:-3}"
MAX_TOKENS="${MEALCHECK_LLAMA_MAX_TOKENS:-220}"
CURL_MAX_TIME_SECONDS="${MEALCHECK_LLAMA_CURL_MAX_TIME_SECONDS:-300}"
RUN_CHECKER="${MEALCHECK_LLAMA_RUN_CHECKER:-1}"

failures=0

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "error: required command not found: $name" >&2
    exit 1
  fi
}

print_section() {
  printf "\n=== %s ===\n" "$1"
}

curl_llama() {
  curl -sS --fail-with-body --max-time "$CURL_MAX_TIME_SECONDS" "$@"
}

record_summary() {
  local status="$1"
  local run_index="$2"
  local seconds="$3"
  local message="$4"
  jq -cn \
    --arg status "$status" \
    --arg model "$MODEL" \
    --arg base_url "$BASE_URL" \
    --arg run_index "$run_index" \
    --argjson duration_seconds "$seconds" \
    --arg message "$message" \
    '{
      status: $status,
      model: $model,
      base_url: $base_url,
      run_index: ($run_index | tonumber),
      duration_seconds: $duration_seconds,
      message: $message
    }' >>"$OUTPUT_DIR/summary.jsonl"
}

build_request() {
  jq -n \
    --arg model "$MODEL" \
    --argjson max_tokens "$MAX_TOKENS" \
    --rawfile meal_plan_text "$PROMPT_FILE" \
    --slurpfile schema "$SCHEMA_PATH" \
    '{
      model: $model,
      temperature: 0,
      max_tokens: $max_tokens,
      stream: false,
      chat_template_kwargs: {
        enable_thinking: false
      },
      response_format: {
        type: "json_schema",
        json_schema: {
          name: "mealcheck_compact_meal_plan",
          strict: true,
          schema: $schema[0]
        }
      },
      messages: [
        {
          role: "system",
          content: "You extract meal-plan ingredients into compact MealCheck local JSON only. Return exactly one minified JSON object. Do not use Markdown. Do not include line breaks, indentation, or spaces outside string values. The only top-level keys are breakfast, lunch, and dinner. Each meal is an array of item objects. Each item uses only f for food, q for numeric quantity, and u for unit. Allowed units are g, oz, cup, tbsp, tsp, and serving."
        },
        {
          role: "user",
          content: ("Extract this one-day meal plan into the smallest valid compact JSON object. Include exactly breakfast, lunch, and dinner. Include only resolved food items with numeric quantity plus unit. Do not include schema_version, plan_id, days, day, meals, name, items, food, quantity, unit, prep notes, or optional fields.\n\n" + $meal_plan_text)
        }
      ]
    }'
}

validate_plan_shape() {
  local plan_path="$1"
  jq -e '
    .schema_version == "0.1" and
    (.plan_id | type == "string" and length > 0) and
    (.days | type == "array" and length == 1) and
    (.days[0].day == 1) and
    (.days[0].meals | type == "array" and length == 3) and
    ([.days[0].meals[].name | ascii_downcase] | sort == ["breakfast", "dinner", "lunch"]) and
    ([.days[0].meals[].items[]] | length >= 6) and
    ((has("description") | not) or .description == "") and
    ((has("shopping_list") | not) or .shopping_list == null or (.shopping_list | type == "array" and length == 0)) and
    ((has("prep_notes") | not) or .prep_notes == null or (.prep_notes | type == "array" and length == 0)) and
    ([.. | objects | select(has("food")) | .food] | length >= 6) and
    ([.days[0].meals[].items[] | select((has("quantity") | not) or (has("unit") | not))] | length == 0) and
    ([.days[0].meals[].items[] | .quantity] |
      all(type == "number")) and
    ([.days[0].meals[].items[] | select(has("quantity_text"))] | length == 0) and
    ([.days[0].meals[].items[] | .unit] |
      all(. as $unit | ["g", "oz", "cup", "tbsp", "tsp", "serving"] | index($unit) != null))
  ' "$plan_path" >/dev/null
}

write_checker_case() {
  local case_path="$1"
  local plan_path="$2"
  jq -n \
    --arg plan_path "$plan_path" \
    '{
      schema_version: "0.1",
      case_id: "local-llama-synthetic-smoke",
      input_mode: "local_llama_trial",
      settings: {
        nutrition_targets: {
          calorie_target_kcal: 2000,
          protein_target_g: 98
        },
        verification_constraints: {
          days: 1,
          meals_per_day: 3,
          allergies: ["peanuts"],
          excluded_foods: ["shellfish"],
          max_sodium_mg_per_day: 2300,
          max_added_sugar_g_per_meal: 10,
          max_saturated_fat_pct_calories: 10,
          calorie_tolerance_pct: 15,
          requires_prep_safety_notes: false
        }
      },
      guideline_pack_id: "dga-2025-2030-us-adult-general-v1",
      guideline_pack_path: "data/guidelines/dga-2025-2030-us-adult-general-v1/guideline-pack.json",
      nutrient_catalog_id: "fixture-catalog-v1",
      nutrient_catalog_path: "data/nutrients/fixture-catalog-v1.json",
      candidate_plan: $plan_path,
      expectations: {
        expected_decision: "block",
        expected_block_checks: [],
        expected_warn_checks: []
      },
      tags: ["local-llama", "synthetic", "structured-json-smoke"]
    }' >"$case_path"
}

run_checker() {
  local run_dir="$1"
  local plan_path="$2"
  local case_path="$run_dir/case.json"
  local artifacts_dir="$run_dir/artifacts"
  local checker_output checker_code

  write_checker_case "$case_path" "$plan_path"

  set +e
  checker_output="$(cd "$ROOT" && go run ./cmd/mealcheck validate --root "$ROOT" --case "$case_path" --out "$artifacts_dir" 2>&1)"
  checker_code="$?"
  set -e

  printf "%s\n" "$checker_output" >"$run_dir/checker-output.txt"
  if [[ "$checker_code" -gt 1 ]]; then
    echo "checker failed to load/evaluate model output; see $run_dir/checker-output.txt" >&2
    return 1
  fi

  return 0
}

run_trial() {
  local index="$1"
  local run_dir="$OUTPUT_DIR/run-$index"
  local request_path="$run_dir/request.json"
  local response_path="$run_dir/response.json"
  local content_path="$run_dir/content.txt"
  local compact_path="$run_dir/compact-plan.json"
  local plan_path="$run_dir/normalized-plan.json"
  local response start end duration content_bytes completion_tokens predicted_tokens predicted_per_second

  mkdir -p "$run_dir"
  build_request >"$request_path"

  print_section "local llama trial $index"
  start="$(date +%s)"
  if ! response="$(curl_llama -X POST "$BASE_URL/chat/completions" -H "Content-Type: application/json" --data-binary @"$request_path" 2>&1)"; then
    end="$(date +%s)"
    duration=$((end - start))
    printf "%s\n" "$response" >"$run_dir/error.txt"
    echo "FAIL: llama.cpp request failed; see $run_dir/error.txt"
    record_summary "fail" "$index" "$duration" "llama.cpp request failed"
    failures=$((failures + 1))
    return 0
  fi
  end="$(date +%s)"
  duration=$((end - start))

  printf "%s\n" "$response" >"$response_path"
  if ! jq -er '.choices[0].message.content // empty' "$response_path" >"$content_path"; then
    echo "FAIL: response did not include choices[0].message.content"
    record_summary "fail" "$index" "$duration" "missing message content"
    failures=$((failures + 1))
    return 0
  fi
  content_bytes="$(wc -c <"$content_path" | tr -d ' ')"
  completion_tokens="$(jq -r '.usage.completion_tokens // empty' "$response_path")"
  predicted_tokens="$(jq -r '.timings.predicted_n // empty' "$response_path")"
  predicted_per_second="$(jq -r '.timings.predicted_per_second // empty' "$response_path")"

  if ! jq . "$content_path" >"$compact_path" 2>"$run_dir/json-parse-error.txt"; then
    echo "FAIL: model content was not JSON; see $content_path"
    record_summary "fail" "$index" "$duration" "message content was not JSON"
    failures=$((failures + 1))
    return 0
  fi

  if ! (cd "$ROOT" && go run ./cmd/mealcheck local-llama normalize --input "$compact_path" --out "$plan_path" --plan-id "local-llama-smoke") >"$run_dir/adapter-output.txt" 2>&1; then
    echo "FAIL: compact JSON could not be adapted to MealCheck JSON; see $run_dir/adapter-output.txt"
    record_summary "fail" "$index" "$duration" "compact adapter failed"
    failures=$((failures + 1))
    return 0
  fi

  if ! validate_plan_shape "$plan_path"; then
    echo "FAIL: model JSON did not meet MealCheck smoke shape; see $plan_path"
    record_summary "fail" "$index" "$duration" "meal plan smoke shape failed"
    failures=$((failures + 1))
    return 0
  fi

  if [[ "$RUN_CHECKER" == "1" ]]; then
    if ! run_checker "$run_dir" "$plan_path"; then
      record_summary "fail" "$index" "$duration" "MealCheck checker could not evaluate output"
      failures=$((failures + 1))
      return 0
    fi
  fi

  jq '{schema_version, plan_id, meals: [.days[0].meals[].name], item_count: ([.days[0].meals[].items[]] | length)}' "$plan_path"
  printf "duration_seconds=%s\n" "$duration"
  printf "content_bytes=%s\n" "$content_bytes"
  if [[ -n "$completion_tokens" ]]; then
    printf "completion_tokens=%s\n" "$completion_tokens"
  fi
  if [[ -n "$predicted_tokens" && -n "$predicted_per_second" ]]; then
    printf "predicted_tokens=%s\n" "$predicted_tokens"
    printf "predicted_tokens_per_second=%s\n" "$predicted_per_second"
  fi
  record_summary "pass" "$index" "$duration" "structured JSON smoke passed"
}

require_command curl
require_command jq
require_command go

if [[ ! -f "$PROMPT_FILE" ]]; then
  echo "error: prompt file not found: $PROMPT_FILE" >&2
  exit 1
fi
if [[ ! -f "$SCHEMA_PATH" ]]; then
  echo "error: schema file not found: $SCHEMA_PATH" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
: >"$OUTPUT_DIR/summary.jsonl"

print_section "local llama health"
health="$(curl_llama "$BASE_URL/models" 2>&1)" || {
  echo "error: llama.cpp health/model check failed at $BASE_URL/models" >&2
  printf "%s\n" "$health" >&2
  exit 1
}
printf "%s\n" "$health" | jq .

printf "base_url=%s\n" "$BASE_URL"
printf "model=%s\n" "$MODEL"
printf "prompt_file=%s\n" "$PROMPT_FILE"
printf "schema_path=%s\n" "$SCHEMA_PATH"
printf "output_dir=%s\n" "$OUTPUT_DIR"
printf "repeats=%s\n" "$REPEATS"
printf "run_checker=%s\n" "$RUN_CHECKER"

for index in $(seq 1 "$REPEATS"); do
  run_trial "$index"
done

print_section "summary"
jq -s '
  {
    total: length,
    passed: ([.[] | select(.status == "pass")] | length),
    failed: ([.[] | select(.status == "fail")] | length),
    durations_seconds: [.[] | .duration_seconds]
  }
' "$OUTPUT_DIR/summary.jsonl"

if [[ "$failures" -gt 0 ]]; then
  echo "FAIL: $failures local llama compact JSON trial(s) failed"
  exit "$failures"
fi

echo "PASS: local llama compact JSON trials passed"

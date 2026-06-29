#!/usr/bin/env bash
#
# Run the P0 normalization regimen against a live local llama.cpp-compatible
# model endpoint on a development laptop.
#
# The script does not start llama-server or download models. It assumes an
# OpenAI-compatible /v1 endpoint is already running.

set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${MEALCHECK_P0_LOCAL_MODEL_BASE_URL:-${MEALCHECK_LOCAL_MODEL_BASE_URL:-http://127.0.0.1:11435/v1}}"
BASE_URL="${BASE_URL%/}"
MODEL="${MEALCHECK_P0_LOCAL_MODEL_NAME:-${MEALCHECK_LOCAL_MODEL_NAME:-}}"
MAX_OUTPUT_TOKENS="${MEALCHECK_P0_MAX_OUTPUT_TOKENS:-${MEALCHECK_LOCAL_MODEL_MAX_OUTPUT_TOKENS:-1536}}"
TIMEOUT="${MEALCHECK_P0_LOCAL_MODEL_TIMEOUT:-${MEALCHECK_LOCAL_MODEL_TIMEOUT:-240s}}"
REPEATS="${MEALCHECK_P0_REPEATS:-3}"
OUTPUT_DIR="${MEALCHECK_P0_OUTPUT_DIR:-/tmp/mealcheck-p0-local-model-$(date +%Y%m%d-%H%M%S)}"
MIN_ROW_MATCH_RATE="${MEALCHECK_P0_MIN_ROW_MATCH_RATE:-1}"
CURL_MAX_TIME_SECONDS="${MEALCHECK_P0_CURL_MAX_TIME_SECONDS:-20}"
ALLOW_MISMATCH="${MEALCHECK_P0_ALLOW_MISMATCH:-0}"
REQUIRE_CLEAN_WORKTREE="${MEALCHECK_P0_REQUIRE_CLEAN_WORKTREE:-0}"
MODEL_SHA="${MEALCHECK_P0_MODEL_SHA:-}"
LLAMA_BUILD="${MEALCHECK_P0_LLAMA_BUILD:-}"
GOCACHE_DIR="${GOCACHE:-/tmp/mealcheck-go-build}"

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

write_json_string_file() {
  local path="$1"
  local value="$2"
  jq -n --arg value "$value" '$value' >"$path"
}

if ! [[ "$REPEATS" =~ ^[0-9]+$ ]] || ((REPEATS < 1)); then
  echo "error: MEALCHECK_P0_REPEATS must be a positive integer" >&2
  exit 2
fi

require_command curl
require_command git
require_command go
require_command jq

mkdir -p "$OUTPUT_DIR"

if [[ "$REQUIRE_CLEAN_WORKTREE" == "1" ]] && [[ -n "$(git -C "$ROOT" status --porcelain)" ]]; then
  echo "error: worktree has uncommitted changes; set MEALCHECK_P0_REQUIRE_CLEAN_WORKTREE=0 to allow" >&2
  git -C "$ROOT" status --short >&2
  exit 2
fi

print_section "Model endpoint"
if ! curl -fsS --max-time "$CURL_MAX_TIME_SECONDS" "$BASE_URL/models" >"$OUTPUT_DIR/models-response.json"; then
  echo "error: local model endpoint is not healthy at $BASE_URL/models" >&2
  exit 1
fi
if [[ -z "$MODEL" ]]; then
  MODEL="$(jq -r '.data[0].id // empty' "$OUTPUT_DIR/models-response.json")"
fi
if [[ -z "$MODEL" ]]; then
  echo "error: no model configured and $BASE_URL/models did not return .data[0].id" >&2
  exit 1
fi
echo "base_url: $BASE_URL"
echo "model: $MODEL"

git -C "$ROOT" status --short >"$OUTPUT_DIR/git-status.txt"
write_json_string_file "$OUTPUT_DIR/go-version.json" "$(go version)"
write_json_string_file "$OUTPUT_DIR/uname.json" "$(uname -a)"
write_json_string_file "$OUTPUT_DIR/cpu.json" "$(sysctl -n machdep.cpu.brand_string 2>/dev/null || true)"
write_json_string_file "$OUTPUT_DIR/memory-bytes.json" "$(sysctl -n hw.memsize 2>/dev/null || true)"

jq -n \
  --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg root "$ROOT" \
  --arg git_commit "$(git -C "$ROOT" rev-parse HEAD)" \
  --arg git_branch "$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)" \
  --arg git_dirty "$(if [[ -s "$OUTPUT_DIR/git-status.txt" ]]; then echo true; else echo false; fi)" \
  --arg base_url "$BASE_URL" \
  --arg model "$MODEL" \
  --arg model_sha "$MODEL_SHA" \
  --arg llama_build "$LLAMA_BUILD" \
  --arg max_output_tokens "$MAX_OUTPUT_TOKENS" \
  --arg timeout "$TIMEOUT" \
  --arg curl_max_time_seconds "$CURL_MAX_TIME_SECONDS" \
  --arg repeats "$REPEATS" \
  --arg min_row_match_rate "$MIN_ROW_MATCH_RATE" \
  --arg output_dir "$OUTPUT_DIR" \
  --rawfile models_response "$OUTPUT_DIR/models-response.json" \
  --rawfile git_status "$OUTPUT_DIR/git-status.txt" \
  --slurpfile go_version "$OUTPUT_DIR/go-version.json" \
  --slurpfile uname "$OUTPUT_DIR/uname.json" \
  --slurpfile cpu "$OUTPUT_DIR/cpu.json" \
  --slurpfile memory_bytes "$OUTPUT_DIR/memory-bytes.json" \
  '{
    schema_version: "0.1",
    regimen: "p0-live-local-model",
    generated_at: $generated_at,
    repository: {
      root: $root,
      git_commit: $git_commit,
      git_branch: $git_branch,
      git_dirty: ($git_dirty == "true"),
      git_status_short: $git_status
    },
    local_model: {
      base_url: $base_url,
      model: $model,
      model_sha: $model_sha,
      llama_build: $llama_build,
      max_output_tokens: ($max_output_tokens | tonumber),
      timeout: $timeout,
      curl_max_time_seconds: ($curl_max_time_seconds | tonumber),
      models_response: ($models_response | fromjson)
    },
    machine: {
      go_version: $go_version[0],
      uname: $uname[0],
      cpu: $cpu[0],
      memory_bytes: $memory_bytes[0]
    },
    evaluation: {
      repeats: ($repeats | tonumber),
      min_row_match_rate: ($min_row_match_rate | tonumber),
      output_dir: $output_dir
    }
  }' >"$OUTPUT_DIR/metadata.json"

print_section "Deterministic P0 baseline"
if ! (
  cd "$ROOT"
  GOCACHE="$GOCACHE_DIR" go run ./cmd/mealcheck eval-normalization \
    -out "$OUTPUT_DIR/deterministic-result.json"
) >"$OUTPUT_DIR/deterministic.stdout" 2>"$OUTPUT_DIR/deterministic.stderr"; then
  echo "error: deterministic P0 evaluation failed" >&2
  cat "$OUTPUT_DIR/deterministic.stderr" >&2
  exit 1
fi
jq '{dataset_id, mode, total_cases, cases_passed, cases_with_mismatches, source_item_preservation_rate}' \
  "$OUTPUT_DIR/deterministic-result.json"

print_section "Live local-model P0 repeats"
: >"$OUTPUT_DIR/live-summary.jsonl"
for run_index in $(seq 1 "$REPEATS"); do
  result_path="$OUTPUT_DIR/live-run-$run_index.json"
  stdout_path="$OUTPUT_DIR/live-run-$run_index.stdout"
  stderr_path="$OUTPUT_DIR/live-run-$run_index.stderr"
  started="$(date +%s)"

  set +e
  (
    cd "$ROOT"
    GOCACHE="$GOCACHE_DIR" go run ./cmd/mealcheck eval-normalization \
      -mode local-llama \
      -local-model-base-url "$BASE_URL" \
      -local-model-name "$MODEL" \
      -local-model-max-output-tokens "$MAX_OUTPUT_TOKENS" \
      -local-model-timeout "$TIMEOUT" \
      -out "$result_path" \
      -allow-mismatch
  ) >"$stdout_path" 2>"$stderr_path"
  exit_code=$?
  set -e

  ended="$(date +%s)"
  duration_seconds=$((ended - started))
  if [[ -s "$result_path" ]] && jq -e . "$result_path" >/dev/null 2>&1; then
    jq -cn \
      --argjson run_index "$run_index" \
      --argjson exit_code "$exit_code" \
      --argjson duration_seconds "$duration_seconds" \
      --slurpfile result "$result_path" \
      '{
        run_index: $run_index,
        exit_code: $exit_code,
        duration_seconds: $duration_seconds,
        result_loaded: true,
        total_cases: $result[0].total_cases,
        cases_passed: $result[0].cases_passed,
        cases_with_mismatches: $result[0].cases_with_mismatches,
        local_model_success_cases_run: $result[0].local_model_success_cases_run,
        local_model_success_cases_pass: $result[0].local_model_success_cases_pass,
        local_model_expected_items: $result[0].local_model_expected_items,
        local_model_rows_matched: $result[0].local_model_rows_matched,
        local_model_row_match_rate: $result[0].local_model_row_match_rate,
        local_model_day_accuracy: ($result[0].local_model_day_accuracy // 0),
        local_model_meal_accuracy: ($result[0].local_model_meal_accuracy // 0),
        local_model_food_accuracy: ($result[0].local_model_food_accuracy // 0),
        local_model_quantity_accuracy: ($result[0].local_model_quantity_accuracy // 0),
        local_model_unit_accuracy: ($result[0].local_model_unit_accuracy // 0),
        local_model_source_repairs: ($result[0].local_model_source_repairs // 0),
        local_model_repair_cases: ($result[0].local_model_repair_cases // 0),
        local_model_provider_failures: ($result[0].local_model_provider_failures // 0),
        local_model_decode_failures: ($result[0].local_model_decode_failures // 0),
        mismatch_case_ids: [($result[0].mismatches // [])[].case_id]
      }' >>"$OUTPUT_DIR/live-summary.jsonl"
  else
    jq -cn \
      --argjson run_index "$run_index" \
      --argjson exit_code "$exit_code" \
      --argjson duration_seconds "$duration_seconds" \
      '{
        run_index: $run_index,
        exit_code: $exit_code,
        duration_seconds: $duration_seconds,
        result_loaded: false,
        cases_with_mismatches: 1,
        local_model_row_match_rate: 0,
        local_model_provider_failures: 1,
        local_model_decode_failures: 0,
        mismatch_case_ids: ["command_failed"]
      }' >>"$OUTPUT_DIR/live-summary.jsonl"
  fi

  jq -r '"run \(.run_index): exit=\(.exit_code) mismatches=\(.cases_with_mismatches) row_match=\(.local_model_row_match_rate) repairs=\(.local_model_source_repairs // 0)"' \
    "$OUTPUT_DIR/live-summary.jsonl" | tail -n 1
done

jq -s \
  --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg output_dir "$OUTPUT_DIR" \
  --argjson repeats "$REPEATS" \
  --argjson min_row_match_rate "$MIN_ROW_MATCH_RATE" \
  '
  def min_or_zero(xs): if (xs | length) == 0 then 0 else xs | min end;
  def max_or_zero(xs): if (xs | length) == 0 then 0 else xs | max end;
  {
    schema_version: "0.1",
    regimen: "p0-live-local-model",
    generated_at: $generated_at,
    output_dir: $output_dir,
    repeats_requested: $repeats,
    repeats_completed: length,
    command_failures: ([.[] | select(.exit_code != 0)] | length),
    repeats_with_mismatches: ([.[] | select((.cases_with_mismatches // 1) > 0)] | length),
    min_local_model_row_match_rate: min_or_zero([.[] | (.local_model_row_match_rate // 0)]),
    min_local_model_food_accuracy: min_or_zero([.[] | (.local_model_food_accuracy // 0)]),
    min_local_model_quantity_accuracy: min_or_zero([.[] | (.local_model_quantity_accuracy // 0)]),
    min_local_model_unit_accuracy: min_or_zero([.[] | (.local_model_unit_accuracy // 0)]),
    max_duration_seconds: max_or_zero([.[] | (.duration_seconds // 0)]),
    total_source_repairs: ([.[] | (.local_model_source_repairs // 0)] | add // 0),
    total_repair_cases: ([.[] | (.local_model_repair_cases // 0)] | add // 0),
    total_provider_failures: ([.[] | (.local_model_provider_failures // 0)] | add // 0),
    total_decode_failures: ([.[] | (.local_model_decode_failures // 0)] | add // 0),
    mismatch_case_ids: ([.[] | (.mismatch_case_ids // [])[]] | unique)
  } as $summary
  | $summary + {
      gate: {
        min_row_match_rate: $min_row_match_rate,
        passed: (
          $summary.repeats_completed == $repeats and
          $summary.command_failures == 0 and
          $summary.repeats_with_mismatches == 0 and
          $summary.total_provider_failures == 0 and
          $summary.total_decode_failures == 0 and
          $summary.min_local_model_row_match_rate >= $min_row_match_rate
        )
      }
    }' "$OUTPUT_DIR/live-summary.jsonl" >"$OUTPUT_DIR/summary.json"

print_section "Aggregate"
jq . "$OUTPUT_DIR/summary.json"
echo "output_dir: $OUTPUT_DIR"

if [[ "$ALLOW_MISMATCH" != "1" ]] && [[ "$(jq -r '.gate.passed' "$OUTPUT_DIR/summary.json")" != "true" ]]; then
  echo "error: P0 live local-model regimen gate failed" >&2
  echo "set MEALCHECK_P0_ALLOW_MISMATCH=1 to keep exploratory runs from failing" >&2
  exit 1
fi

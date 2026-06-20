#!/usr/bin/env bash
#
# Run live BYOK tests against a deployed MealCheck API server.
#
# Public script contract:
# - This script assumes provider API keys are already loaded into the local
#   terminal environment.
# - It does not retrieve keys; load them through your preferred secure workflow.
# - It requires:
#     OPENAI_API_KEY
#     ANTHROPIC_API_KEY
#     GEMINI_API_KEY
#
# The script does not print provider keys and does not write request bodies to
# disk. It fetches deployed run artifacts into a local temp directory and scans
# those fetched artifacts for provider keys. That scan proves keys are not
# exposed through retrievable artifacts; it does not inspect the deployed
# server's filesystem directly.
#
# Usage:
#   OPENAI_API_KEY=... ANTHROPIC_API_KEY=... GEMINI_API_KEY=... \
#   MEALCHECK_DEPLOYED_API_URL=https://api.mealcheck.dev \
#     scripts/test-deployed-byok-live.sh
#
# Optional:
#   MEALCHECK_DEPLOYED_INVITE_TOKEN=... # for invite_required deployments
#   MEALCHECK_DELETE_RUNS=0             # keep deployed runs after the test
#   MEALCHECK_DEPLOYED_OUTPUT_DIR=...   # local directory for fetched artifacts
#   GEMINI_MODEL=gemini-2.5-flash-lite
#   ANTHROPIC_MODEL=claude-haiku-4-5
#   OPENAI_MODEL=gpt-5.4-mini

set -Eeuo pipefail

API_URL="${MEALCHECK_DEPLOYED_API_URL:-https://api.mealcheck.dev}"
API_URL="${API_URL%/}"
INVITE_TOKEN="${MEALCHECK_DEPLOYED_INVITE_TOKEN:-}"
DELETE_RUNS="${MEALCHECK_DELETE_RUNS:-1}"
OUTPUT_DIR="${MEALCHECK_DEPLOYED_OUTPUT_DIR:-/tmp/mealcheck-deployed-byok-$(date +%Y%m%d-%H%M%S)}"
POLL_ATTEMPTS="${MEALCHECK_POLL_ATTEMPTS:-60}"
POLL_SLEEP_SECONDS="${MEALCHECK_POLL_SLEEP_SECONDS:-2}"

GEMINI_MODEL="${GEMINI_MODEL:-gemini-2.5-flash-lite}"
ANTHROPIC_MODEL="${ANTHROPIC_MODEL:-claude-haiku-4-5}"
OPENAI_MODEL="${OPENAI_MODEL:-gpt-5.4-mini}"

created_runs=()
failures=0
SUBMITTED_RUN_ID=""

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "error: required command not found: $name" >&2
    exit 1
  fi
}

require_env_secret() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "error: $name must be set in the local environment" >&2
    exit 1
  fi
}

sanitize_output() {
  local text
  text="$(cat)"
  for key in "${OPENAI_API_KEY:-}" "${ANTHROPIC_API_KEY:-}" "${GEMINI_API_KEY:-}"; do
    if [[ -n "$key" ]]; then
      text="${text//$key/[redacted]}"
    fi
  done
  printf "%s\n" "$text"
}

curl_api_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local args

  args=(-sS --fail-with-body -X "$method" "$API_URL$path")
  if [[ -n "$INVITE_TOKEN" ]]; then
    args+=(-H "X-MealCheck-Invite-Token: $INVITE_TOKEN")
  fi

  if [[ -n "$body" ]]; then
    curl "${args[@]}" -H "Content-Type: application/json" --data-binary "$body"
  else
    curl "${args[@]}"
  fi
}

cleanup_runs() {
  if [[ "$DELETE_RUNS" != "1" ]]; then
    return 0
  fi
  if ((${#created_runs[@]} == 0)); then
    return 0
  fi

  local run_id
  for run_id in "${created_runs[@]}"; do
    if [[ -n "$run_id" ]]; then
      curl_api_json DELETE "/api/runs/$run_id" >/dev/null 2>&1 || true
    fi
  done
}
trap cleanup_runs EXIT

print_section() {
  printf "\n=== %s ===\n" "$1"
}

submit_byok_run() {
  local provider_type="$1"
  local model="$2"
  local api_key="$3"
  local body response run_id

  body="$(MEALCHECK_CURRENT_PROVIDER_KEY="$api_key" jq -n \
    --arg provider_type "$provider_type" \
    --arg model "$model" \
    '{
      input_mode: "prompt_generation",
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
          requires_prep_safety_notes: true
        }
      },
      generation_prompt: "Create a simple one-day meal plan with breakfast, lunch, and dinner. Include item quantities and units. Avoid peanuts and shellfish. Include brief prep safety notes.",
      provider: {
        type: $provider_type,
        model: $model,
        api_key: env.MEALCHECK_CURRENT_PROVIDER_KEY
      },
      repair_json: true
    }')"

  if ! response="$(curl_api_json POST "/api/runs" "$body" 2>&1)"; then
    echo "submit failed for $provider_type/$model" >&2
    printf "%s\n" "$response" | sanitize_output >&2
    return 1
  fi

  run_id="$(printf "%s\n" "$response" | jq -r '.run_id // empty')"
  if [[ -z "$run_id" ]]; then
    echo "submit failed for $provider_type/$model: response did not include run_id" >&2
    printf "%s\n" "$response" | sanitize_output >&2
    return 1
  fi

  created_runs+=("$run_id")
  SUBMITTED_RUN_ID="$run_id"
}

poll_run() {
  local run_id="$1"
  local response status attempt

  for attempt in $(seq 1 "$POLL_ATTEMPTS"); do
    if ! response="$(curl_api_json GET "/api/runs/$run_id" 2>&1)"; then
      echo "poll failed for $run_id" >&2
      printf "%s\n" "$response" | sanitize_output >&2
      return 1
    fi

    printf "%s\n" "$response" |
      jq '{id:.run.id,status:.run.status,decision:.run.decision,risk_level:.run.risk_level,error:.run.error}'

    status="$(printf "%s\n" "$response" | jq -r '.run.status // empty')"
    case "$status" in
      completed)
        return 0
        ;;
      failed)
        return 1
        ;;
    esac

    sleep "$POLL_SLEEP_SECONDS"
  done

  echo "run $run_id did not finish after $POLL_ATTEMPTS polling attempts" >&2
  return 1
}

fetch_artifact() {
  local run_id="$1"
  local artifact_path="$2"
  local output_path="$3"
  local response

  if ! response="$(curl_api_json GET "/api/runs/$run_id/artifacts/$artifact_path" 2>&1)"; then
    echo "artifact fetch failed for $run_id: $artifact_path" >&2
    printf "%s\n" "$response" | sanitize_output >&2
    return 1
  fi

  printf "%s\n" "$response" >"$output_path"
}

inspect_completed_run() {
  local provider="$1"
  local run_id="$2"
  local run_dir="$OUTPUT_DIR/$provider-$run_id"

  mkdir -p "$run_dir"

  fetch_artifact "$run_id" "optional/normalization-events.json" "$run_dir/normalization-events.json"
  fetch_artifact "$run_id" "normalized-plan.json" "$run_dir/normalized-plan.json"
  fetch_artifact "$run_id" "decision.json" "$run_dir/decision.json"

  printf "normalization events:\n"
  jq . "$run_dir/normalization-events.json"

  printf "normalized plan summary:\n"
  jq '{schema_version, plan_id, meals: [.days[0].meals[].name]}' "$run_dir/normalized-plan.json"

  printf "decision summary:\n"
  jq '{decision, risk_level, summary}' "$run_dir/decision.json"
}

run_provider() {
  local provider="$1"
  local model="$2"
  local api_key="$3"
  local run_id

  print_section "$provider via deployed MealCheck"

  SUBMITTED_RUN_ID=""
  if ! submit_byok_run "$provider" "$model" "$api_key"; then
    failures=$((failures + 1))
    return 0
  fi
  run_id="$SUBMITTED_RUN_ID"
  printf "run_id=%s\n" "$run_id"

  if ! poll_run "$run_id"; then
    failures=$((failures + 1))
    return 0
  fi

  if ! inspect_completed_run "$provider" "$run_id"; then
    failures=$((failures + 1))
    return 0
  fi
}

check_local_outputs_for_keys() {
  local key_name key_value matches

  print_section "local fetched artifact key scan"
  for key_name in OPENAI_API_KEY ANTHROPIC_API_KEY GEMINI_API_KEY; do
    key_value="${!key_name}"
    matches="$(grep -R -l -F "$key_value" "$OUTPUT_DIR" 2>/dev/null || true)"
    if [[ -n "$matches" ]]; then
      echo "LEAK FOUND in fetched artifacts for $key_name"
      printf "%s\n" "$matches"
      failures=$((failures + 1))
    else
      echo "OK: no $key_name found in fetched artifacts"
    fi
  done
}

require_command curl
require_command jq
require_env_secret OPENAI_API_KEY
require_env_secret ANTHROPIC_API_KEY
require_env_secret GEMINI_API_KEY

mkdir -p "$OUTPUT_DIR"

print_section "deployed health"
health="$(curl_api_json GET "/api/health" 2>&1)" || {
  echo "health check failed for $API_URL/api/health" >&2
  printf "%s\n" "$health" | sanitize_output >&2
  exit 1
}
printf "%s\n" "$health" | jq .

access_mode="$(printf "%s\n" "$health" | jq -r '.access_mode // "unknown"')"
if [[ "$access_mode" == "invite_required" && -z "$INVITE_TOKEN" ]]; then
  echo "error: deployed API requires an invite token; set MEALCHECK_DEPLOYED_INVITE_TOKEN" >&2
  exit 1
fi

printf "api_url=%s\n" "$API_URL"
printf "output_dir=%s\n" "$OUTPUT_DIR"
printf "delete_runs=%s\n" "$DELETE_RUNS"

run_provider "gemini" "$GEMINI_MODEL" "$GEMINI_API_KEY"
run_provider "anthropic" "$ANTHROPIC_MODEL" "$ANTHROPIC_API_KEY"
run_provider "openai" "$OPENAI_MODEL" "$OPENAI_API_KEY"

check_local_outputs_for_keys

if [[ "$failures" -gt 0 ]]; then
  print_section "result"
  echo "FAIL: $failures deployed BYOK check(s) failed"
  exit "$failures"
fi

print_section "result"
echo "PASS: deployed BYOK live checks passed"

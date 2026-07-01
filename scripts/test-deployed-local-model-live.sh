#!/usr/bin/env bash
#
# Run live local-model tests against a deployed MealCheck API server.
#
# Public script contract:
# - This script does not require model provider API keys.
# - It assumes the deployed backend is configured with:
#     MEALCHECK_HOSTED_MODE=local_model
#     MEALCHECK_LOCAL_MODEL_ENABLED=true
# - It writes fetched public run artifacts to a local temp directory for
#   inspection, then deletes the deployed run by default.
# - It expects hosted local-model runs to pause in awaiting_review, fetches the
#   normalized-plan review artifact, confirms the review, and then verifies the
#   completed report artifacts.
#
# Usage:
#   MEALCHECK_DEPLOYED_API_URL=https://api.mealcheck.dev \
#     scripts/test-deployed-local-model-live.sh
#
# Optional:
#   MEALCHECK_DEPLOYED_INVITE_TOKEN=... # for invite_required deployments
#   MEALCHECK_DELETE_RUNS=0             # keep deployed runs after the test
#   MEALCHECK_DEPLOYED_OUTPUT_DIR=...   # local directory for fetched artifacts
#   MEALCHECK_POLL_ATTEMPTS=60
#   MEALCHECK_POLL_SLEEP_SECONDS=2

set -Eeuo pipefail

API_URL="${MEALCHECK_DEPLOYED_API_URL:-https://api.mealcheck.dev}"
API_URL="${API_URL%/}"
INVITE_TOKEN="${MEALCHECK_DEPLOYED_INVITE_TOKEN:-}"
DELETE_RUNS="${MEALCHECK_DELETE_RUNS:-1}"
OUTPUT_DIR="${MEALCHECK_DEPLOYED_OUTPUT_DIR:-/tmp/mealcheck-deployed-local-model-$(date +%Y%m%d-%H%M%S)}"
POLL_ATTEMPTS="${MEALCHECK_POLL_ATTEMPTS:-60}"
POLL_SLEEP_SECONDS="${MEALCHECK_POLL_SLEEP_SECONDS:-2}"

created_runs=()
failures=0
SUBMITTED_RUN_ID=""
HEALTH_JSON=""
LAST_RUN_STATUS=""

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "error: required command not found: $name" >&2
    exit 1
  fi
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

curl_api_expect_error() {
  local method="$1"
  local path="$2"
  local expected_status="$3"
  local body="${4:-}"
  local tmp_body tmp_status
  local args

  tmp_body="$(mktemp)"
  tmp_status="$(mktemp)"
  args=(-sS -o "$tmp_body" -w "%{http_code}" -X "$method" "$API_URL$path")
  if [[ -n "$INVITE_TOKEN" ]]; then
    args+=(-H "X-MealCheck-Invite-Token: $INVITE_TOKEN")
  fi
  if [[ -n "$body" ]]; then
    args+=(-H "Content-Type: application/json" --data-binary "$body")
  fi
  if ! curl "${args[@]}" >"$tmp_status"; then
    echo "request failed for $method $path" >&2
    cat "$tmp_body" >&2
    rm -f "$tmp_body" "$tmp_status"
    return 1
  fi

  local status
  status="$(cat "$tmp_status")"
  if [[ "$status" != "$expected_status" ]]; then
    echo "expected HTTP $expected_status from $method $path, got $status" >&2
    cat "$tmp_body" >&2
    rm -f "$tmp_body" "$tmp_status"
    return 1
  fi
  cat "$tmp_body"
  rm -f "$tmp_body" "$tmp_status"
}

run_output_dir() {
  printf "%s/local-model-%s" "$OUTPUT_DIR" "$1"
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

settings_json() {
  jq -n '{
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
  }'
}

candidate_text() {
  cat <<'EOF'
Day 1 breakfast: 1 cup cooked oatmeal, 1 cup blueberries, and 1 cup plain Greek yogurt.
Day 1 lunch: 6 oz chicken breast, 1 cup brown rice, and 1 cup broccoli.
Day 1 dinner: 6 oz salmon, 1 cup sweet potato, and 1 cup spinach.
EOF
}

local_model_payload() {
  jq -n \
    --arg candidate_text "$(candidate_text)" \
    --argjson settings "$(settings_json)" \
    '{
      input_mode: "local_model",
      candidate_text: $candidate_text,
      settings: $settings
    }'
}

submit_local_model_run() {
  local body response run_id

  body="$(local_model_payload)"
  if ! response="$(curl_api_json POST "/api/runs" "$body" 2>&1)"; then
    echo "submit failed for deployed local model" >&2
    printf "%s\n" "$response" >&2
    return 1
  fi

  run_id="$(printf "%s\n" "$response" | jq -r '.run_id // empty')"
  if [[ -z "$run_id" ]]; then
    echo "submit failed: response did not include run_id" >&2
    printf "%s\n" "$response" >&2
    return 1
  fi

  created_runs+=("$run_id")
  SUBMITTED_RUN_ID="$run_id"
}

poll_run_until_review() {
  local run_id="$1"
  local response status attempt

  for attempt in $(seq 1 "$POLL_ATTEMPTS"); do
    if ! response="$(curl_api_json GET "/api/runs/$run_id" 2>&1)"; then
      echo "poll failed for $run_id" >&2
      printf "%s\n" "$response" >&2
      return 1
    fi

    printf "%s\n" "$response" |
      jq '{id:.run.id,status:.run.status,decision:.run.decision,risk_level:.run.risk_level,error:.run.error}'

    status="$(printf "%s\n" "$response" | jq -r '.run.status // empty')"
    LAST_RUN_STATUS="$status"
    case "$status" in
      awaiting_review)
        return 0
        ;;
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

poll_run_until_completed() {
  local run_id="$1"
  local response status attempt

  for attempt in $(seq 1 "$POLL_ATTEMPTS"); do
    if ! response="$(curl_api_json GET "/api/runs/$run_id" 2>&1)"; then
      echo "poll failed for $run_id" >&2
      printf "%s\n" "$response" >&2
      return 1
    fi

    printf "%s\n" "$response" |
      jq '{id:.run.id,status:.run.status,decision:.run.decision,risk_level:.run.risk_level,error:.run.error}'

    status="$(printf "%s\n" "$response" | jq -r '.run.status // empty')"
    LAST_RUN_STATUS="$status"
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

  echo "run $run_id did not complete after $POLL_ATTEMPTS polling attempts" >&2
  return 1
}

fetch_artifact() {
  local run_id="$1"
  local artifact_path="$2"
  local output_path="$3"
  local response

  if ! response="$(curl_api_json GET "/api/runs/$run_id/artifacts/$artifact_path" 2>&1)"; then
    echo "artifact fetch failed for $run_id: $artifact_path" >&2
    printf "%s\n" "$response" >&2
    return 1
  fi

  mkdir -p "$(dirname "$output_path")"
  printf "%s\n" "$response" >"$output_path"
}

inspect_review_and_confirm() {
  local run_id="$1"
  local run_dir review_path confirm_path response

  run_dir="$(run_output_dir "$run_id")"
  review_path="$run_dir/review/normalized-plan-review.before-confirm.json"
  confirm_path="$run_dir/review/confirm-response.json"
  mkdir -p "$run_dir/review"

  if ! response="$(curl_api_json GET "/api/runs/$run_id/review" 2>&1)"; then
    echo "review fetch failed for $run_id" >&2
    printf "%s\n" "$response" >&2
    return 1
  fi
  printf "%s\n" "$response" >"$review_path"

  printf "normalized-plan review summary:\n"
  jq '{
    status,
    requires_confirmation,
    trust_signals,
    rows: [.rows[] | {
      day,
      meal_code,
      source_item_id,
      source_text,
      normalized_food,
      quantity,
      unit,
      quantity_text,
      unresolved_reason
    }]
  }' "$review_path"

  if ! jq -e '
    .status == "awaiting_confirmation" and
    .trust_signals.source_item_count == 9 and
    .trust_signals.normalized_row_count == 9 and
    ((.rows // []) | length) == 9 and
    ([.rows[] | select(
      ((.source_item_id // 0) > 0) and
      (((.source_text // "") | length) > 0) and
      (((.normalized_food // "") | length) > 0)
    )] | length) == 9
  ' "$review_path" >/dev/null; then
    echo "normalized-plan review did not include expected source-linked rows" >&2
    jq . "$review_path" >&2
    return 1
  fi

  if ! response="$(curl_api_json POST "/api/runs/$run_id/review/confirm" '{"reason":"Deployed operator walkthrough confirmed normalized plan."}' 2>&1)"; then
    echo "review confirmation failed for $run_id" >&2
    printf "%s\n" "$response" >&2
    return 1
  fi
  printf "%s\n" "$response" >"$confirm_path"

  printf "review confirmation response:\n"
  jq '{id:.run.id,status:.run.status,progress:.progress}' "$confirm_path"
}

inspect_completed_run() {
  local run_id="$1"
  local run_dir
  local response

  run_dir="$(run_output_dir "$run_id")"
  mkdir -p "$run_dir"

  fetch_artifact "$run_id" "manifest.json" "$run_dir/manifest.json"
  fetch_artifact "$run_id" "report.json" "$run_dir/report.json"
  fetch_artifact "$run_id" "recommendation.json" "$run_dir/recommendation.json"
  fetch_artifact "$run_id" "optional/normalization-events.json" "$run_dir/normalization-events.json"
  fetch_artifact "$run_id" "optional/local-model-chunks.json" "$run_dir/local-model-chunks.json"
  fetch_artifact "$run_id" "normalized-plan.json" "$run_dir/normalized-plan.json"
  fetch_artifact "$run_id" "decision.json" "$run_dir/decision.json"
  fetch_artifact "$run_id" "review/normalized-plan-review.json" "$run_dir/review/normalized-plan-review.json"
  fetch_artifact "$run_id" "review/review-actions.jsonl" "$run_dir/review/review-actions.jsonl"

  if ! response="$(curl_api_json GET "/api/runs/$run_id/artifacts" 2>&1)"; then
    echo "artifact list fetch failed for $run_id" >&2
    printf "%s\n" "$response" >&2
    return 1
  fi
  printf "%s\n" "$response" >"$run_dir/artifacts.json"

  printf "artifact list summary:\n"
  jq '{count: (.artifacts | length), artifacts: [.artifacts[].path]}' "$run_dir/artifacts.json"

  printf "manifest review and recommendation artifacts:\n"
  jq '.artifacts | map(select(. == "recommendation.json" or . == "review/normalized-plan-review.json" or . == "review/review-actions.jsonl"))' "$run_dir/manifest.json"

  printf "normalization events:\n"
  jq . "$run_dir/normalization-events.json"

  printf "local model chunk summary:\n"
  jq '{
    chunk_count,
    source_item_count,
    chunks: [.chunks[] | {
      index,
      meal_code,
      source_item_ids,
      decoded_rows: ((.decoded_rows // []) | length),
      repair_count: .reconciliation.repair_count,
      total_ms: .stage_timings.total_ms
    }]
  }' "$run_dir/local-model-chunks.json"

  printf "normalized plan summary:\n"
  jq '{
    schema_version,
    plan_id,
    meals: [.days[0].meals[].name],
    item_count: ([.days[].meals[].items[]] | length)
  }' "$run_dir/normalized-plan.json"

  local item_count meals
  item_count="$(jq '([.days[].meals[].items[]] | length)' "$run_dir/normalized-plan.json")"
  meals="$(jq -r '[.days[0].meals[].name] | sort | join(",")' "$run_dir/normalized-plan.json")"
  if [[ "$item_count" != "9" ]]; then
    echo "expected 9 normalized items, got $item_count" >&2
    return 1
  fi
  if [[ "$meals" != "breakfast,dinner,lunch" ]]; then
    echo "expected breakfast,lunch,dinner meals, got $meals" >&2
    return 1
  fi
  if ! jq -e '
    .chunk_count == 3 and
    .source_item_count == 9 and
    ([.chunks[] | select(
      .prompt.message_count == 2 and
      ((.raw_compact_output // "") | length) > 0 and
      (((.decoded_rows // []) | length) == (.source_item_ids | length))
    )] | length) == 3
  ' "$run_dir/local-model-chunks.json" >/dev/null; then
    echo "local-model chunk artifact did not include expected prompt/output/decoded-row evidence" >&2
    jq . "$run_dir/local-model-chunks.json" >&2
    return 1
  fi

  printf "decision summary:\n"
  jq '{decision, risk_level, summary}' "$run_dir/decision.json"

  printf "recommendation summary:\n"
  jq '{status, reason, source_decision, change_count: ((.changes // []) | length), projected_decision: .projected_decision.decision}' "$run_dir/recommendation.json"

  printf "review action summary:\n"
  jq -s 'map({action, reason, created_at})' "$run_dir/review/review-actions.jsonl"

  if ! jq -e '
    (.artifacts | index("recommendation.json")) and
    (.artifacts | index("review/normalized-plan-review.json")) and
    (.artifacts | index("review/review-actions.jsonl"))
  ' "$run_dir/manifest.json" >/dev/null; then
    echo "manifest did not include recommendation and review artifacts" >&2
    jq . "$run_dir/manifest.json" >&2
    return 1
  fi
  if ! jq -e -s 'any(.[]; .action == "confirmed")' "$run_dir/review/review-actions.jsonl" >/dev/null; then
    echo "review actions did not include confirmation" >&2
    cat "$run_dir/review/review-actions.jsonl" >&2
    return 1
  fi
}

check_health() {
  print_section "deployed health"
  HEALTH_JSON="$(curl_api_json GET "/api/health" 2>&1)" || {
    echo "health check failed for $API_URL/api/health" >&2
    printf "%s\n" "$HEALTH_JSON" >&2
    exit 1
  }
  printf "%s\n" "$HEALTH_JSON" | jq .

  local access_mode hosted_mode enabled ready
  access_mode="$(printf "%s\n" "$HEALTH_JSON" | jq -r '.access_mode // "unknown"')"
  hosted_mode="$(printf "%s\n" "$HEALTH_JSON" | jq -r '.hosted_mode // "unknown"')"
  enabled="$(printf "%s\n" "$HEALTH_JSON" | jq -r '.local_model.enabled // false')"
  ready="$(printf "%s\n" "$HEALTH_JSON" | jq -r '.local_model.ready // false')"

  if [[ "$access_mode" == "invite_required" && -z "$INVITE_TOKEN" ]]; then
    echo "error: deployed API requires an invite token; set MEALCHECK_DEPLOYED_INVITE_TOKEN" >&2
    exit 1
  fi
  if [[ "$hosted_mode" != "local_model" ]]; then
    echo "error: deployed API hosted_mode=$hosted_mode, want local_model" >&2
    exit 1
  fi
  if [[ "$enabled" != "true" || "$ready" != "true" ]]; then
    echo "error: deployed local model is not enabled and ready" >&2
    exit 1
  fi
}

check_rejections() {
  local provider_body provider_response max_input oversized_text oversized_body oversized_response

  print_section "provider config rejection"
  provider_body="$(local_model_payload | jq '.provider = {type:"openai", model:"gpt-test", api_key:"secret"}')"
  if ! provider_response="$(curl_api_expect_error POST "/api/runs" 400 "$provider_body")"; then
    failures=$((failures + 1))
  else
    printf "%s\n" "$provider_response" | jq .
  fi

  print_section "oversized input rejection"
  max_input="$(printf "%s\n" "$HEALTH_JSON" | jq -r '.local_model.max_input_chars // 6000')"
  oversized_text="$(printf 'x%.0s' $(seq 1 $((max_input + 1))))"
  oversized_body="$(jq -n \
    --arg candidate_text "$oversized_text" \
    --argjson settings "$(settings_json)" \
    '{input_mode:"local_model", candidate_text:$candidate_text, settings:$settings}')"
  if ! oversized_response="$(curl_api_expect_error POST "/api/runs" 400 "$oversized_body")"; then
    failures=$((failures + 1))
  else
    printf "%s\n" "$oversized_response" | jq .
  fi
}

delete_and_verify_runs() {
  if [[ "$DELETE_RUNS" != "1" ]]; then
    print_section "delete deployed runs"
    echo "deletion skipped because MEALCHECK_DELETE_RUNS=$DELETE_RUNS"
    return 0
  fi
  if ((${#created_runs[@]} == 0)); then
    return 0
  fi

  print_section "delete deployed runs"
  local run_id response verify_response
  for run_id in "${created_runs[@]}"; do
    if ! response="$(curl_api_json DELETE "/api/runs/$run_id" 2>&1)"; then
      echo "delete failed for $run_id" >&2
      printf "%s\n" "$response" >&2
      failures=$((failures + 1))
      continue
    fi
    printf "%s\n" "$response" | jq .

    if ! verify_response="$(curl_api_expect_error GET "/api/runs/$run_id" 404 2>&1)"; then
      echo "delete verification failed for $run_id" >&2
      printf "%s\n" "$verify_response" >&2
      failures=$((failures + 1))
      continue
    fi
    printf "delete verified for %s:\n" "$run_id"
    printf "%s\n" "$verify_response" | jq .
  done
  created_runs=()
}

require_command curl
require_command jq
mkdir -p "$OUTPUT_DIR"

check_health

printf "api_url=%s\n" "$API_URL"
printf "output_dir=%s\n" "$OUTPUT_DIR"
printf "delete_runs=%s\n" "$DELETE_RUNS"

print_section "local model via deployed MealCheck"
if ! submit_local_model_run; then
  failures=$((failures + 1))
else
  run_id="$SUBMITTED_RUN_ID"
  printf "run_id=%s\n" "$run_id"
  if ! poll_run_until_review "$run_id"; then
    failures=$((failures + 1))
  elif [[ "$LAST_RUN_STATUS" != "awaiting_review" ]]; then
    echo "expected local-model run to pause in awaiting_review, got $LAST_RUN_STATUS" >&2
    failures=$((failures + 1))
  elif ! inspect_review_and_confirm "$run_id"; then
    failures=$((failures + 1))
  elif ! poll_run_until_completed "$run_id"; then
    failures=$((failures + 1))
  elif ! inspect_completed_run "$run_id"; then
    failures=$((failures + 1))
  fi
fi

check_rejections
delete_and_verify_runs

if [[ "$failures" -gt 0 ]]; then
  print_section "result"
  echo "FAIL: $failures deployed local-model check(s) failed"
  exit "$failures"
fi

print_section "result"
echo "PASS: deployed local-model live checks passed"

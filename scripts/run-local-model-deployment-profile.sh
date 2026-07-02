#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${MEALCHECK_PROFILE_ENV_FILE:-$ROOT_DIR/deploy/local-model/mealcheck-server.env.example}"
BUILD_SERVER="${MEALCHECK_PROFILE_BUILD:-1}"
WAIT_FOR_POSTGRES="${MEALCHECK_PROFILE_WAIT_FOR_POSTGRES:-1}"
REQUIRE_LOCAL_MODEL="${MEALCHECK_PROFILE_REQUIRE_LOCAL_MODEL:-1}"
READY_TIMEOUT_SECONDS="${MEALCHECK_PROFILE_READY_TIMEOUT_SECONDS:-90}"
READY_INTERVAL_SECONDS="${MEALCHECK_PROFILE_READY_INTERVAL_SECONDS:-2}"

log() {
  printf '%s mealcheck-local-model-profile: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >&2
}

fail() {
  log "$*"
  exit 1
}

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    fail "missing required command: $name"
  fi
}

abs_path() {
  local path="$1"
  case "$path" in
    /*) printf '%s\n' "$path" ;;
    *) printf '%s/%s\n' "$ROOT_DIR" "$path" ;;
  esac
}

trim_trailing_slash() {
  local value="$1"
  printf '%s\n' "${value%/}"
}

wait_for_http() {
  local label="$1"
  local url="$2"
  local end
  end="$(($(date +%s) + READY_TIMEOUT_SECONDS))"
  log "waiting for $label at $url"
  while ! curl -fsS "$url" >/dev/null 2>&1; do
    if [ "$(date +%s)" -ge "$end" ]; then
      fail "timed out waiting for $label after ${READY_TIMEOUT_SECONDS}s"
    fi
    sleep "$READY_INTERVAL_SECONDS"
  done
  log "$label is ready"
}

wait_for_postgres() {
  if [ "${MEALCHECK_STORE:-postgres}" != "postgres" ]; then
    return 0
  fi
  if [ -z "${DATABASE_URL:-}" ]; then
    fail "DATABASE_URL is required for postgres profile runs"
  fi
  if [ "$WAIT_FOR_POSTGRES" != "1" ]; then
    return 0
  fi
  require_command pg_isready
  local end
  end="$(($(date +%s) + READY_TIMEOUT_SECONDS))"
  log "waiting for Postgres from DATABASE_URL"
  while ! pg_isready -d "$DATABASE_URL" >/dev/null 2>&1; do
    if [ "$(date +%s)" -ge "$end" ]; then
      fail "timed out waiting for Postgres after ${READY_TIMEOUT_SECONDS}s"
    fi
    sleep "$READY_INTERVAL_SECONDS"
  done
  log "Postgres is ready"
}

resolve_local_model_name() {
  if [ "${MEALCHECK_HOSTED_MODE:-local_model}" != "local_model" ]; then
    return 0
  fi
  if [ "${MEALCHECK_LOCAL_MODEL_NAME:-}" != "auto" ]; then
    return 0
  fi
  if [ "$REQUIRE_LOCAL_MODEL" != "1" ]; then
    fail "MEALCHECK_LOCAL_MODEL_NAME=auto requires MEALCHECK_PROFILE_REQUIRE_LOCAL_MODEL=1"
  fi
  require_command jq
  local base_url models_url response model_id
  base_url="$(trim_trailing_slash "${MEALCHECK_LOCAL_MODEL_BASE_URL:-http://127.0.0.1:11435/v1}")"
  models_url="$base_url/models"
  response="$(curl -fsS "$models_url")" || fail "could not fetch local model list from $models_url"
  model_id="$(printf '%s\n' "$response" | jq -r '.data[0].id // empty')"
  if [ -z "$model_id" ]; then
    fail "local model list did not include data[0].id"
  fi
  export MEALCHECK_LOCAL_MODEL_NAME="$model_id"
  log "resolved local model: $MEALCHECK_LOCAL_MODEL_NAME"
}

check_local_model_endpoint() {
  if [ "${MEALCHECK_HOSTED_MODE:-local_model}" != "local_model" ]; then
    return 0
  fi
  if [ "$REQUIRE_LOCAL_MODEL" != "1" ]; then
    return 0
  fi
  case "${MEALCHECK_LOCAL_MODEL_ENABLED:-true}" in
    1|true|TRUE|yes|YES|on|ON) ;;
    *) fail "local-model profile requires MEALCHECK_LOCAL_MODEL_ENABLED=true" ;;
  esac
  require_command curl
  local base_url
  base_url="$(trim_trailing_slash "${MEALCHECK_LOCAL_MODEL_BASE_URL:-http://127.0.0.1:11435/v1}")"
  wait_for_http "local model endpoint" "$base_url/models"
  export MEALCHECK_LOCAL_MODEL_BASE_URL="$base_url"
}

if [ ! -f "$ENV_FILE" ]; then
  fail "environment file not found: $ENV_FILE"
fi

cd "$ROOT_DIR"
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

: "${MEALCHECK_ADDR:=127.0.0.1:8080}"
: "${MEALCHECK_STORE:=postgres}"
: "${MEALCHECK_DATA_DIR:=.mealcheck-local-model}"
: "${MEALCHECK_ARTIFACT_DIR:=$MEALCHECK_DATA_DIR/artifacts}"
: "${MEALCHECK_HOSTED_MODE:=local_model}"
: "${MEALCHECK_LOCAL_MODEL_ENABLED:=true}"
: "${MEALCHECK_LOCAL_MODEL_BASE_URL:=http://127.0.0.1:11435/v1}"

export MEALCHECK_ADDR
export MEALCHECK_STORE
export MEALCHECK_HOSTED_MODE
export MEALCHECK_LOCAL_MODEL_ENABLED

MEALCHECK_DATA_DIR="$(abs_path "$MEALCHECK_DATA_DIR")"
MEALCHECK_ARTIFACT_DIR="$(abs_path "$MEALCHECK_ARTIFACT_DIR")"
export MEALCHECK_DATA_DIR
export MEALCHECK_ARTIFACT_DIR

mkdir -p "$MEALCHECK_DATA_DIR" "$MEALCHECK_ARTIFACT_DIR"

check_local_model_endpoint
resolve_local_model_name
if [ "$REQUIRE_LOCAL_MODEL" = "1" ] && [ "${MEALCHECK_HOSTED_MODE:-local_model}" = "local_model" ] && [ -z "${MEALCHECK_LOCAL_MODEL_NAME:-}" ]; then
  fail "local-model profile requires MEALCHECK_LOCAL_MODEL_NAME or MEALCHECK_LOCAL_MODEL_NAME=auto"
fi
wait_for_postgres

if [ "$BUILD_SERVER" = "1" ]; then
  require_command go
  mkdir -p "$ROOT_DIR/bin"
  log "building bin/mealcheck-server"
  go build -o "$ROOT_DIR/bin/mealcheck-server" ./cmd/mealcheck-server
fi

SERVER_BIN="${MEALCHECK_PROFILE_SERVER_BIN:-$ROOT_DIR/bin/mealcheck-server}"
if [ ! -x "$SERVER_BIN" ]; then
  fail "server binary is not executable: $SERVER_BIN"
fi

log "starting API addr=$MEALCHECK_ADDR store=$MEALCHECK_STORE data_dir=$MEALCHECK_DATA_DIR artifact_dir=$MEALCHECK_ARTIFACT_DIR hosted_mode=$MEALCHECK_HOSTED_MODE"
exec "$SERVER_BIN" \
  -root "$ROOT_DIR" \
  -addr "$MEALCHECK_ADDR" \
  -data-dir "$MEALCHECK_DATA_DIR" \
  -artifact-dir "$MEALCHECK_ARTIFACT_DIR" \
  -store "$MEALCHECK_STORE"

#!/bin/sh
set -u

PATH="/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin:${PATH:-}"

POSTGRES_HOST="${MEALCHECK_POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${MEALCHECK_POSTGRES_PORT:-5432}"
HEALTH_URL="${MEALCHECK_HEALTH_URL:-http://127.0.0.1:8080/api/health}"
TIMEOUT_SECONDS="${MEALCHECK_READY_TIMEOUT_SECONDS:-180}"
INTERVAL_SECONDS="${MEALCHECK_READY_INTERVAL_SECONDS:-2}"

log() {
  printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "missing required command: $1"
    exit 127
  fi
}

deadline() {
  expr "$(date +%s)" + "$TIMEOUT_SECONDS"
}

timed_out() {
  [ "$(date +%s)" -ge "$1" ]
}

print_diagnostics() {
  log "diagnostics: Postgres launchd state"
  launchctl print system/com.mealcheck.postgres 2>/dev/null | sed -n '1,80p' || true

  log "diagnostics: MealCheck launchd state"
  launchctl print system/com.mealcheck.server 2>/dev/null | sed -n '1,80p' || true

  log "diagnostics: Postgres log tail"
  tail -n 40 /usr/local/var/log/postgresql@17.log 2>/dev/null || true

  log "diagnostics: MealCheck error log tail"
  tail -n 40 /Users/chranama-server/MealCheck-data/logs/mealcheck-server.err.log 2>/dev/null || true
}

wait_for_postgres() {
  end="$(deadline)"
  log "waiting for Postgres at ${POSTGRES_HOST}:${POSTGRES_PORT}"
  while ! pg_isready -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" >/dev/null 2>&1; do
    if timed_out "$end"; then
      log "timed out waiting for Postgres after ${TIMEOUT_SECONDS}s"
      print_diagnostics
      exit 1
    fi
    sleep "$INTERVAL_SECONDS"
  done
  log "Postgres is ready"
}

wait_for_mealcheck() {
  end="$(deadline)"
  log "waiting for MealCheck health at ${HEALTH_URL}"
  while ! curl -fsS "$HEALTH_URL" >/dev/null 2>&1; do
    if timed_out "$end"; then
      log "timed out waiting for MealCheck after ${TIMEOUT_SECONDS}s"
      print_diagnostics
      exit 1
    fi
    sleep "$INTERVAL_SECONDS"
  done
  log "MealCheck is ready"
}

print_health() {
  if command -v jq >/dev/null 2>&1; then
    curl -fsS "$HEALTH_URL" | jq .
  else
    curl -fsS "$HEALTH_URL"
    printf '\n'
  fi
}

require_command pg_isready
require_command curl
require_command launchctl
require_command tail
require_command sed

wait_for_postgres
wait_for_mealcheck
print_health

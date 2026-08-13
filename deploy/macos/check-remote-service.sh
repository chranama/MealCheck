#!/bin/bash

set -euo pipefail

PATH="/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin:${PATH:-}"
export PATH
export LANG=C
export LC_ALL=C

fail() {
  printf 'check-remote-service: %s\n' "$*" >&2
  exit 1
}

usage() {
  printf 'usage: %s [--connect] mealcheck-server|mealcheck-server-cf\n' "$0" >&2
  exit 64
}

CONNECT=0
case "$#:${1:-}" in
  1:mealcheck-server|1:mealcheck-server-cf)
    REMOTE_HOST=$1
    ;;
  2:--connect)
    REMOTE_HOST=$2
    CONNECT=1
    ;;
  *) usage ;;
esac
case "$REMOTE_HOST" in
  mealcheck-server|mealcheck-server-cf) ;;
  *) fail "unsupported administration alias: $REMOTE_HOST" ;;
esac

SSH_BIN=${MEALCHECK_REMOTE_SSH_BIN:-}
if [ -z "$SSH_BIN" ]; then
  SSH_BIN=$(command -v ssh) || fail "ssh is unavailable"
fi
[ -x "$SSH_BIN" ] || fail "SSH executable is unavailable"

effective=$("$SSH_BIN" -G -T "$REMOTE_HOST") || fail "cannot resolve SSH configuration for $REMOTE_HOST"
user=$(printf '%s\n' "$effective" | /usr/bin/awk '$1 == "user" {print $2; exit}')
host_key_alias=$(printf '%s\n' "$effective" | /usr/bin/awk '$1 == "hostkeyalias" {print $2; exit}')
[ "$user" = "chranama-server" ] || fail "effective SSH user is not chranama-server"
[ -n "$host_key_alias" ] || fail "effective SSH configuration lacks HostKeyAlias"

printf 'Selected MealCheck administration alias: %s\n' "$REMOTE_HOST"
if [ "$CONNECT" -eq 0 ]; then
  printf 'Configuration-only check passed; no network connection was attempted.\n'
  exit 0
fi

printf 'Starting read-only MealCheck checks; no fallback alias will be attempted.\n'
"$SSH_BIN" -o BatchMode=yes -o ConnectTimeout=20 "$REMOTE_HOST" /bin/bash -s <<'REMOTE'
set -euo pipefail

for service in \
  system/dev.mealcheck.server \
  system/dev.mealcheck.llama \
  system/dev.mealcheck.postgres; do
  /bin/launchctl print "$service" >/dev/null
  printf 'ok service %s\n' "$service"
done

/usr/bin/curl -fsS --connect-timeout 5 --max-time 10 \
  http://127.0.0.1:8080/api/health | /usr/bin/python3 -c '
import json
import sys

value = json.load(sys.stdin)
if value.get("status") != "ok":
    raise SystemExit("MealCheck health is not ok")
local_model = value.get("local_model") or {}
if value.get("hosted_mode") != "local_model" or not local_model.get("ready"):
    raise SystemExit("MealCheck local model is not ready")
'
printf 'ok MealCheck local health and model readiness\n'
REMOTE

printf 'Read-only MealCheck check passed through %s.\n' "$REMOTE_HOST"

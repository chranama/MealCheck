#!/bin/bash
set -euo pipefail

PATH="/usr/local/bin:/opt/homebrew/bin:/usr/local/go/bin:/usr/bin:/bin:/usr/sbin:/sbin:${PATH:-}"

LABEL="dev.mealcheck.llama"
DEPLOY_USER="${MEALCHECK_LLAMA_USER:-chranama-server}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PLIST_SRC="$SCRIPT_DIR/dev.mealcheck.llama.plist.template"
PLIST_DST="/Library/LaunchDaemons/$LABEL.plist"
ENV_SRC="$SCRIPT_DIR/mealcheck-llama.env.example"
ENV_DST="/Users/chranama-server/MealCheck-data/mealcheck-llama.env"
LOG_DIR="/Users/chranama-server/MealCheck-data/logs"

usage() {
  cat <<EOF
usage: $0 install|restart|stop|status|logs

install  Copy env template when missing, install the launchd plist, and start llama-server.
restart  Restart system/$LABEL.
stop     Stop system/$LABEL.
status   Print launchd status.
logs     Tail llama service logs.
EOF
}

as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  else
    sudo "$@"
  fi
}

install_service() {
  as_root mkdir -p "$LOG_DIR"
  as_root chown "$DEPLOY_USER" "$LOG_DIR"

  if [ ! -f "$ENV_DST" ]; then
    cp "$ENV_SRC" "$ENV_DST"
    chmod 600 "$ENV_DST"
    as_root chown "$DEPLOY_USER" "$ENV_DST"
    echo "created $ENV_DST from template; review it before production rollout"
  else
    echo "keeping existing $ENV_DST"
  fi
  as_root chown "$DEPLOY_USER" "$ENV_DST"
  chmod 600 "$ENV_DST"

  as_root cp "$PLIST_SRC" "$PLIST_DST"
  as_root chown root:wheel "$PLIST_DST"
  as_root chmod 644 "$PLIST_DST"
  as_root plutil -lint "$PLIST_DST"

  as_root launchctl bootout "system/$LABEL" >/dev/null 2>&1 || true
  as_root launchctl bootstrap system "$PLIST_DST"
  as_root launchctl kickstart -k "system/$LABEL"
  as_root launchctl print "system/$LABEL"
}

case "${1:-}" in
  install)
    cd "$REPO_DIR"
    install_service
    ;;
  restart)
    as_root launchctl kickstart -k "system/$LABEL"
    ;;
  stop)
    as_root launchctl bootout "system/$LABEL"
    ;;
  status)
    as_root launchctl print "system/$LABEL"
    ;;
  logs)
    tail -n 100 "$LOG_DIR/mealcheck-llama.out.log" "$LOG_DIR/mealcheck-llama.err.log"
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

#!/bin/bash
set -euo pipefail

PATH="/usr/local/bin:/opt/homebrew/bin:/usr/local/go/bin:/usr/bin:/bin:/usr/sbin:/sbin:${PATH:-}"

DEPLOY_USER="${MEALCHECK_AUTODEPLOY_USER:-chranama-server}"
REPO_DIR="${MEALCHECK_AUTODEPLOY_REPO_DIR:-/Users/chranama-server/MealCheck}"
REMOTE="${MEALCHECK_AUTODEPLOY_REMOTE:-origin}"
BRANCH="${MEALCHECK_AUTODEPLOY_BRANCH:-main}"
SERVER_LABEL="${MEALCHECK_AUTODEPLOY_SERVER_LABEL:-dev.mealcheck.server}"
HEALTH_URL="${MEALCHECK_AUTODEPLOY_HEALTH_URL:-http://127.0.0.1:8080/api/health}"
LOG_DIR="${MEALCHECK_AUTODEPLOY_LOG_DIR:-/Users/chranama-server/MealCheck-data/logs}"
LOCK_DIR="${MEALCHECK_AUTODEPLOY_LOCK_DIR:-/tmp/dev.mealcheck.autodeploy.lock}"
GO_TEST="${MEALCHECK_AUTODEPLOY_GO_TEST:-1}"
HEALTH_TIMEOUT_SECONDS="${MEALCHECK_AUTODEPLOY_HEALTH_TIMEOUT_SECONDS:-90}"
HEALTH_INTERVAL_SECONDS="${MEALCHECK_AUTODEPLOY_HEALTH_INTERVAL_SECONDS:-3}"

BUILD_DIR=""

log() {
  printf '%s mealcheck-autodeploy: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

quote() {
  printf '%q' "$1"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "missing required command: $1"
    exit 127
  fi
}

cleanup() {
  if [ -n "$BUILD_DIR" ]; then
    build_dir_q="$(quote "$BUILD_DIR")"
    repo_q="$(quote "$REPO_DIR")"
    user_shell "cd $repo_q && git worktree remove --force $build_dir_q >/dev/null 2>&1 || rm -rf $build_dir_q" >/dev/null 2>&1 || true
  fi
  rmdir "$LOCK_DIR" >/dev/null 2>&1 || true
}

user_shell() {
  command_text="$1"
  path_q="$(quote "$PATH")"
  if [ "$(id -un)" = "$DEPLOY_USER" ]; then
    /bin/bash -lc "export PATH=$path_q; $command_text"
  else
    /usr/bin/sudo -H -u "$DEPLOY_USER" /bin/bash -lc "export PATH=$path_q; $command_text"
  fi
}

wait_for_health() {
  deadline=$(( $(date +%s) + HEALTH_TIMEOUT_SECONDS ))
  while ! curl -fsS "$HEALTH_URL" >/dev/null 2>&1; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
      log "health check failed after restart: $HEALTH_URL"
      return 1
    fi
    sleep "$HEALTH_INTERVAL_SECONDS"
  done
}

needs_backend_rebuild() {
  changed_paths="$1"
  while IFS= read -r path; do
    case "$path" in
      cmd/*|internal/*|go.mod|go.sum)
        return 0
        ;;
    esac
  done <<EOF
$changed_paths
EOF
  return 1
}

build_backend_binaries() {
  target="$1"
  repo_q="$(quote "$REPO_DIR")"
  target_q="$(quote "$target")"
  build_parent="${TMPDIR:-/tmp}"
  BUILD_DIR="${build_parent%/}/mealcheck-autodeploy-build-${target}"
  build_dir_q="$(quote "$BUILD_DIR")"

  log "building backend binaries from $target"
  user_shell "cd $repo_q && git worktree remove --force $build_dir_q >/dev/null 2>&1 || true"
  user_shell "rm -rf $build_dir_q && cd $repo_q && git worktree add --detach $build_dir_q $target_q"

  if [ "$GO_TEST" = "1" ]; then
    log "running Go tests"
    user_shell "cd $build_dir_q && go test ./..."
  fi

  user_shell "mkdir -p $repo_q/bin"
  user_shell "cd $build_dir_q && go build -o $repo_q/bin/mealcheck.next ./cmd/mealcheck"
  user_shell "cd $build_dir_q && go build -o $repo_q/bin/mealcheck-server.next ./cmd/mealcheck-server"
}

install_backend_binaries() {
  repo_q="$(quote "$REPO_DIR")"
  log "installing backend binaries"
  user_shell "cd $repo_q && mv bin/mealcheck.next bin/mealcheck && mv bin/mealcheck-server.next bin/mealcheck-server"
}

restart_backend() {
  log "restarting $SERVER_LABEL"
  /bin/launchctl kickstart -k "system/$SERVER_LABEL"
  wait_for_health
  log "backend health OK"
}

main() {
  require_command git
  require_command curl
  require_command sudo
  require_command launchctl

  if [ "$(id -u)" -ne 0 ]; then
    log "this script must run as root so it can restart system/$SERVER_LABEL"
    exit 1
  fi

  mkdir -p "$LOG_DIR"

  if ! mkdir "$LOCK_DIR" >/dev/null 2>&1; then
    log "another autodeploy run is already active; exiting"
    exit 0
  fi
  trap cleanup EXIT INT TERM

  if [ ! -d "$REPO_DIR/.git" ]; then
    log "repository not found: $REPO_DIR"
    exit 1
  fi

  repo_q="$(quote "$REPO_DIR")"
  remote_q="$(quote "$REMOTE")"
  branch_q="$(quote "$BRANCH")"
  upstream_ref="$REMOTE/$BRANCH"
  upstream_q="$(quote "$upstream_ref")"
  refspec_q="$(quote "refs/heads/$BRANCH:refs/remotes/$REMOTE/$BRANCH")"

  user_shell "command -v git >/dev/null"
  user_shell "command -v go >/dev/null"

  dirty="$(user_shell "cd $repo_q && git status --porcelain")"
  if [ -n "$dirty" ]; then
    log "refusing to autodeploy because the worktree is dirty"
    printf '%s\n' "$dirty"
    exit 1
  fi

  log "fetching $REMOTE $BRANCH"
  user_shell "cd $repo_q && git fetch $remote_q $refspec_q"

  current="$(user_shell "cd $repo_q && git rev-parse HEAD")"
  target="$(user_shell "cd $repo_q && git rev-parse $upstream_q")"

  if [ "$current" = "$target" ]; then
    log "already at $target"
    exit 0
  fi

  merge_base="$(user_shell "cd $repo_q && git merge-base HEAD $upstream_q")"
  if [ "$merge_base" != "$current" ]; then
    log "refusing to autodeploy because HEAD is not an ancestor of $upstream_ref"
    log "current=$current target=$target merge_base=$merge_base"
    exit 1
  fi

  changed_paths="$(user_shell "cd $repo_q && git diff --name-only HEAD $upstream_q")"
  log "updating $current -> $target"
  printf '%s\n' "$changed_paths" | sed 's/^/  changed: /'

  if needs_backend_rebuild "$changed_paths"; then
    build_backend_binaries "$target"
    user_shell "cd $repo_q && git merge --ff-only $upstream_q"
    install_backend_binaries
    restart_backend
  else
    user_shell "cd $repo_q && git merge --ff-only $upstream_q"
    log "no backend code changed; skipped rebuild and restart"
  fi

  log "autodeploy complete at $target"
}

main "$@"

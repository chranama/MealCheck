#!/usr/bin/env bash
set -Eeuo pipefail
DEST=${MEALCHECK_LAUNCHAGENT_PATH:-$HOME/Library/LaunchAgents/dev.mealcheck.controller.lab.plist}
LABEL=dev.mealcheck.controller.lab
DOMAIN=gui/$(id -u)
[[ $(uname -s) == Darwin ]] || { echo 'macOS required' >&2; exit 1; }
if [[ -e "$DEST" ]]; then
  actual=$(/usr/libexec/PlistBuddy -c 'Print :Label' "$DEST")
  [[ "$actual" == "$LABEL" ]] || { echo 'refusing unrelated plist' >&2; exit 1; }
fi
if launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; then launchctl bootout "$DOMAIN/$LABEL"; fi
if [[ -e "$DEST" ]]; then rm "$DEST"; fi
echo 'Lab LaunchAgent removed. Controller state, containers, and persistent volumes retained.'

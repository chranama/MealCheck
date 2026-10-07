#!/usr/bin/env bash
# Installs only the dedicated lab controller in the current GUI user session.
set -Eeuo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
LAB_ROOT=${MEALCHECK_LAB_ROOT:-/tmp/controller-lab}
BINARY=${MEALCHECK_CONTROLLER_BINARY:-$LAB_ROOT/bin/mealcheck-controller}
STATE=${MEALCHECK_CONTROLLER_STATE:-$LAB_ROOT/controller-state}
SOCKET=${MEALCHECK_CONTROLLER_SOCKET:-$STATE/controller.sock}
ENGINE=${MEALCHECK_CONTROLLER_ENGINE:-unix://$HOME/.docker/run/docker.sock}
SECRETS=${MEALCHECK_CONTROLLER_SECRETS:-$LAB_ROOT/secrets}
MODELS=${MEALCHECK_CONTROLLER_MODELS:-$LAB_ROOT/models}
REGISTRIES=${MEALCHECK_CONTROLLER_REGISTRIES:-localhost:15000/mealcheck,postgres,ghcr.io/ggml-org/llama.cpp}
DEST=${MEALCHECK_LAUNCHAGENT_PATH:-$HOME/Library/LaunchAgents/dev.mealcheck.controller.lab.plist}
LABEL=dev.mealcheck.controller.lab
DOMAIN=gui/$(id -u)
[[ $(uname -s) == Darwin ]] || { echo 'macOS required' >&2; exit 1; }
[[ -x "$BINARY" && -d "$SECRETS" && -d "$MODELS" ]] || { echo 'build binary and prepare lab first' >&2; exit 1; }
[[ "$BINARY" == /* && "$STATE" == /* && "$SOCKET" == /* && "$SECRETS" == /* && "$MODELS" == /* ]] || { echo 'paths must be absolute' >&2; exit 1; }
if launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; then
  echo 'lab LaunchAgent already loaded; inspect or uninstall first' >&2; exit 1
fi
[[ ! -e "$DEST" ]] || { echo 'refusing to overwrite existing plist' >&2; exit 1; }
umask 077
mkdir -p "$STATE" "$LAB_ROOT/logs" "$(dirname "$DEST")"
chmod 700 "$STATE" "$LAB_ROOT/logs"
python3 - "$ROOT/deploy/controller/dev.mealcheck.controller.lab.plist.template" "$DEST" "$BINARY" "$STATE" "$SOCKET" "$ENGINE" "$SECRETS" "$MODELS" "$REGISTRIES" "$LAB_ROOT/logs" <<'PY'
import plistlib,sys
from xml.sax.saxutils import escape
src,dest,*values=sys.argv[1:]
keys=['BINARY','STATE','SOCKET','ENGINE','SECRETS','MODELS','REGISTRIES','LOGS']
text=open(src).read()
for key,value in zip(keys,values):text=text.replace('@'+key+'@',escape(value))
# Parse before writing so malformed templates cannot be installed.
parsed=plistlib.loads(text.encode())
with open(dest,'wb') as f:plistlib.dump(parsed,f)
PY
chmod 600 "$DEST"
plutil -lint "$DEST"
launchctl bootstrap "$DOMAIN" "$DEST"
echo "Installed $DOMAIN/$LABEL; state and data are retained on uninstall."

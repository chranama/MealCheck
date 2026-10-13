# Mac controller operations

Use the current account's LaunchAgent, not a root LaunchDaemon: Docker Desktop's
engine belongs to the GUI user session. launchd supervises the native controller;
the controller alone owns workload-container recovery. No automatic Docker Engine
restart policy is configured, and no script starts/restarts Docker Desktop.

## Prerequisites

Prepare the model and secret files using the workload contract. Build and preload
the pinned API/PostgreSQL/model images. Verify the Docker endpoint is a local Unix
socket. Supply explicit image allowlist prefixes. Keep state outside managed data
volumes, mode0700, with no existing daemon holding its lock.

For a disposable demonstration:

```bash
mkdir -p /tmp/controller-lab/bin /tmp/controller-lab/secrets/mealcheck-lab /tmp/controller-lab/systems
cp deploy/controller/systems/mealcheck-cpu-arm64--v1.json /tmp/controller-lab/systems/
chmod 700 /tmp/controller-lab/systems
chmod 600 /tmp/controller-lab/systems/*.json
chmod 700 /tmp/controller-lab/secrets/mealcheck-lab
cp /tmp/controller-lab/secrets/postgres-password /tmp/controller-lab/secrets/mealcheck-lab/
cp /tmp/controller-lab/secrets/database-url /tmp/controller-lab/secrets/mealcheck-lab/
go build -o /tmp/controller-lab/bin/mealcheck-controller ./cmd/mealcheck-controller
MEALCHECK_LAUNCHAGENT_PATH=/tmp/controller-lab/dev.mealcheck.controller.lab.plist \
  ./deploy/controller/macos/install-launchagent.sh
launchctl print gui/$(id -u)/dev.mealcheck.controller.lab
/tmp/controller-lab/bin/mealcheck-controller get --state-dir /tmp/controller-lab/controller-state
```

For Intel Docker Desktop, stage `mealcheck-cpu-amd64--v1.json` instead of the ARM64
file in these examples and preload its native pins. The system declares smaller
CPU/RAM limits; the daemon checks the actual Engine capacity before acceptance.
Never change Docker Desktop allocation to make a fixed profile fit.

The provider resolves `secretProfile=mealcheck-lab` beneath the configured secret
root; the copy step stages the packaging credentials into that private profile
directory. Files remain mode0600. Never stage production credentials.

The installer refuses an existing plist or loaded lab agent. Override documented
`MEALCHECK_CONTROLLER_*` variables in the script to select persistent account-owned
paths. `/tmp` is suitable for the current-session demonstration only: macOS may
remove it at reboot. For continued operation, move binary, models, secrets,
state and logs to private directories under the account's home and set overrides
before installation. The plist records absolute paths and needs no shell startup
file or ambient PATH.

## Generate and apply desired state

After pushing/preloading the API image in the loopback registry, obtain its
immutable reference explicitly. Reject an empty/tag-only result; the renderer
validates the digest and copies no secrets:

```bash
./deploy/controller/packaging/package-lab.sh build
# Run a fresh build-only registry; Docker refuses any existing name collision.
docker run -d --name mealcheck-lab-registry -p 127.0.0.1:15000:5000 \
  registry@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373
docker tag mealcheck-controller-lab:build localhost:15000/mealcheck:lab
docker push localhost:15000/mealcheck:lab
API_DIGEST=$(docker image inspect localhost:15000/mealcheck:lab --format '{{index .RepoDigests 0}}')
# Engineer: review the system API image matches the native preloaded API_DIGEST.
# If different, edit the trusted system and issue a new version/content digest.
python3 deploy/controller/tools/render-spec.py \
  --system-file /tmp/controller-lab/systems/mealcheck-cpu-arm64--v1.json \
  --controller-binary /tmp/controller-lab/bin/mealcheck-controller \
  --output /tmp/controller-lab/desired.json
/tmp/controller-lab/bin/mealcheck-controller plan \
  --state-dir /tmp/controller-lab/controller-state --file /tmp/controller-lab/desired.json
/tmp/controller-lab/bin/mealcheck-controller apply \
  --state-dir /tmp/controller-lab/controller-state --file /tmp/controller-lab/desired.json
```

The loopback registry is a packaging prerequisite, outside managed workload resources.
If the named lab registry already exists, inspect its ownership and loopback port
before reusing it; the command above refuses to replace it. Preloaded digest images
remain usable without the registry while present in the Engine image store.

The renderer refuses to overwrite an existing output. Choose a new filename when
one exists and review it before apply. It references the approved engineer system
by name, version, and canonical digest,
with a resolved read-only lab model path and private `mealcheck-lab` secret profile,
loopback port18080, and Retain policy. Build/preload all images first.

Accepted deployment and resolved system snapshots persist in SQLite; the source
files are not watched. Catalog edits do not change existing resources. Workload
configuration remains immutable after acceptance; changed references or parameter
values are rejected. Lifecycle intent changes remain supported. Registry, model,
secret-root, Engine endpoint, and trusted-catalog policies belong to the daemon
and cannot be widened by a deployment input.

## Persistent installation example

After completing disposable tests, use private durable paths and the default
account LaunchAgents directory. This example creates a fresh controller
installation; delete/uninstall the disposable instance first, and do not adopt
its retained resources with the new installation UUID.

```bash
export MEALCHECK_LAB_ROOT="$HOME/MealCheck-controller-lab"
export MEALCHECK_CONTROLLER_BINARY="$MEALCHECK_LAB_ROOT/bin/mealcheck-controller"
export MEALCHECK_CONTROLLER_STATE="$MEALCHECK_LAB_ROOT/controller-state"
export MEALCHECK_CONTROLLER_SOCKET="$MEALCHECK_CONTROLLER_STATE/controller.sock"
export MEALCHECK_CONTROLLER_ENGINE="unix://$HOME/.docker/run/docker.sock"
export MEALCHECK_CONTROLLER_SECRETS="$MEALCHECK_LAB_ROOT/secrets"
export MEALCHECK_CONTROLLER_MODELS="$MEALCHECK_LAB_ROOT/models"
export MEALCHECK_CONTROLLER_REGISTRIES='localhost:15000/mealcheck,docker.io/library,postgres,ghcr.io/ggml-org/llama.cpp'
export MEALCHECK_CONTROLLER_CATALOG="$MEALCHECK_LAB_ROOT/systems"
unset MEALCHECK_LAUNCHAGENT_PATH
umask 077
mkdir -p "$MEALCHECK_LAB_ROOT/bin" "$MEALCHECK_CONTROLLER_SECRETS/mealcheck-lab" "$MEALCHECK_CONTROLLER_CATALOG"
cp deploy/controller/systems/mealcheck-cpu-arm64--v1.json "$MEALCHECK_CONTROLLER_CATALOG/"
chmod 700 "$MEALCHECK_CONTROLLER_CATALOG"
chmod 600 "$MEALCHECK_CONTROLLER_CATALOG"/*.json
./deploy/controller/packaging/package-lab.sh prepare
cp "$MEALCHECK_CONTROLLER_SECRETS/postgres-password" "$MEALCHECK_CONTROLLER_SECRETS/mealcheck-lab/"
cp "$MEALCHECK_CONTROLLER_SECRETS/database-url" "$MEALCHECK_CONTROLLER_SECRETS/mealcheck-lab/"
go build -o "$MEALCHECK_CONTROLLER_BINARY" ./cmd/mealcheck-controller
./deploy/controller/macos/install-launchagent.sh
```

Generate a fresh desired-state document using `python3 deploy/controller/tools/render-spec.py --lab-root
"$MEALCHECK_LAB_ROOT"`, `--system-file` and `--controller-binary`, then plan/apply with this
state directory. The installer writes the default
`~/Library/LaunchAgents/dev.mealcheck.controller.lab.plist`. Login discovery is
expected from this placement; login/reboot remain unverified until exercised.

## Lifecycle walkthrough

1. Apply a digest-pinned lab spec; inspect `get` and `events` until current-generation
   readiness. Run `./deploy/controller/packaging/package-lab.sh smoke` (explicit loopback URL) against the controller
   stack; this command runs only the smoke workflow, not packaging orchestration.
2. Kill only the inventory-listed API container. Record detection, restart, health,
   and successful report retrieval separately. Completed reports must remain.
3. Kill the controller PID listed by `launchctl print`; KeepAlive must launch a new
   PID. Inspect the unchanged accepted generation and owned resource identities.
4. Exercise engine unavailability using a controlled unreachable endpoint in a
   separate test instance, or deliberately stop Docker Desktop only when interruption
   of all other local containers is acceptable. Unknown observation must not create
   replacements. Restore engine access, inspect convergence and resource identities.
5. Stop/start through the CLI, re-fetch the report, then delete twice. Containers
   and network disappear; volumes and tombstone remain.

Do not run `./deploy/controller/packaging/package-lab.sh up` while the controller stack uses port18080. Packaging
and controller resource names/labels are distinct.

## Inspection and removal

```bash
launchctl print gui/$(id -u)/dev.mealcheck.controller.lab
cat /tmp/controller-lab/logs/stderr.log
/tmp/controller-lab/bin/mealcheck-controller events --state-dir /tmp/controller-lab/controller-state
MEALCHECK_LAUNCHAGENT_PATH=/tmp/controller-lab/dev.mealcheck.controller.lab.plist \
  ./deploy/controller/macos/uninstall-launchagent.sh
```

Uninstall stops only the lab supervisor and removes its matching plist. It does
not remove workload resources or state. To stop/remove workloads, send the desired
CLI transition while the daemon is available before uninstalling. Retained-volume
purge is manual, outside the controller: inspect installation/deployment ownership
and backup needs first. Never use broad Docker pruning.

## Login/reboot boundary

The LaunchAgent starts at GUI login once installed under `~/Library/LaunchAgents`.
It may start before Docker Desktop is ready: the controller must report degraded
engine observation and back off until access returns. A user-session agent does
not promise pre-login availability. A current-session restart test does not prove
logout/login or reboot behavior. Reboot/login tests require an operator-controlled
window and are not performed automatically on this workstation. Record them as
unverified until actually exercised.

## Verification evidence

See [recorded supervision and outage evidence](../evidence/supervision.md) and the [verification ledger](../evidence/README.md) for completed checks and outstanding runtime gates.

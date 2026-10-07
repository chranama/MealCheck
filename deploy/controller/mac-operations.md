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
mkdir -p /tmp/controller-lab/bin /tmp/controller-lab/secrets/mealcheck-lab
chmod 700 /tmp/controller-lab/secrets/mealcheck-lab
cp /tmp/controller-lab/secrets/postgres-password /tmp/controller-lab/secrets/mealcheck-lab/
cp /tmp/controller-lab/secrets/database-url /tmp/controller-lab/secrets/mealcheck-lab/
go build -o /tmp/controller-lab/bin/mealcheck-controller ./cmd/mealcheck-controller
MEALCHECK_LAUNCHAGENT_PATH=/tmp/controller-lab/dev.mealcheck.controller.lab.plist \
  ./deploy/controller/install-launchagent.sh
launchctl print gui/$(id -u)/dev.mealcheck.controller.lab
/tmp/controller-lab/bin/mealcheck-controller get --state-dir /tmp/controller-lab/controller-state
```

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
./deploy/controller/package-lab.sh build
# Run a fresh build-only registry; Docker refuses any existing name collision.
docker run -d --name mealcheck-lab-registry -p 127.0.0.1:15000:5000 \
  registry@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373
docker tag mealcheck-controller-lab:build localhost:15000/mealcheck:lab
docker push localhost:15000/mealcheck:lab
API_DIGEST=$(docker image inspect localhost:15000/mealcheck:lab --format '{{index .RepoDigests 0}}')
python3 deploy/controller/render-spec.py --api-image "$API_DIGEST" \
  --output /tmp/controller-lab/desired.json
/tmp/controller-lab/bin/mealcheck-controller apply \
  --state-dir /tmp/controller-lab/controller-state --file /tmp/controller-lab/desired.json
```

The loopback registry is a packaging prerequisite, outside managed workload resources.
If the named lab registry already exists, inspect its ownership and loopback port
before reusing it; the command above refuses to replace it. Preloaded digest images
remain usable without the registry while present in the Engine image store.

The renderer refuses to overwrite an existing output. Choose a new filename when
one exists and review it before apply. It uses the committed PostgreSQL/model
pins, resolved read-only lab model path, private `mealcheck-lab` secret profile,
loopback port18080, and Retain policy. Build/preload all images first.

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
export MEALCHECK_CONTROLLER_REGISTRIES='localhost:15000/mealcheck,postgres,ghcr.io/ggml-org/llama.cpp'
unset MEALCHECK_LAUNCHAGENT_PATH
umask 077
mkdir -p "$MEALCHECK_LAB_ROOT/bin" "$MEALCHECK_CONTROLLER_SECRETS/mealcheck-lab"
./deploy/controller/package-lab.sh prepare
cp "$MEALCHECK_CONTROLLER_SECRETS/postgres-password" "$MEALCHECK_CONTROLLER_SECRETS/mealcheck-lab/"
cp "$MEALCHECK_CONTROLLER_SECRETS/database-url" "$MEALCHECK_CONTROLLER_SECRETS/mealcheck-lab/"
go build -o "$MEALCHECK_CONTROLLER_BINARY" ./cmd/mealcheck-controller
./deploy/controller/install-launchagent.sh
```

Generate a fresh desired-state document using `render-spec.py --lab-root
"$MEALCHECK_LAB_ROOT"` and the preloaded pinned API image, then apply with this
state directory. The installer writes the default
`~/Library/LaunchAgents/dev.mealcheck.controller.lab.plist`. Login discovery is
expected from this placement; login/reboot remain unverified until exercised.

## Lifecycle walkthrough

1. Apply a digest-pinned lab spec; inspect `get` and `events` until current-generation
   readiness. Run `package-lab.sh smoke` (explicit loopback URL) against the controller
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

Do not run `package-lab.sh up` while the controller stack uses port18080. Packaging
and controller resource names/labels are distinct.

## Inspection and removal

```bash
launchctl print gui/$(id -u)/dev.mealcheck.controller.lab
cat /tmp/controller-lab/logs/stderr.log
/tmp/controller-lab/bin/mealcheck-controller events --state-dir /tmp/controller-lab/controller-state
MEALCHECK_LAUNCHAGENT_PATH=/tmp/controller-lab/dev.mealcheck.controller.lab.plist \
  ./deploy/controller/uninstall-launchagent.sh
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

## Current-session supervision evidence (2026-10-07)

A real LaunchAgent was bootstrapped in `gui/501` using
`MEALCHECK_LAUNCHAGENT_PATH=/tmp/controller-lab/dev.mealcheck.controller.lab.plist`.
The process ran natively with the Docker Desktop Unix endpoint and private
controller-state directory. This temporary plist was deliberately not installed
under `~/Library/LaunchAgents`; it proves current-session bootstrap only and
will not be automatically discovered at the next login. Persistent installation
requires the default account LaunchAgents path and durable paths described above.

A SIGKILL of the managed controller changed PID101 to PID787. The socket served
persisted `Ready` status after 0.138 seconds; this measures supervisor/process
availability, not fresh workload observation. A later status check confirmed
fresh observation at `2026-10-07T15:37:18.794362Z`, generation1, Ready. All six
managed resource IDs (three containers, two volumes, one network) stayed identical.
The synthetic completed run `run_552b21f7b10c5eaf60b9ae80` remained completed.
The daemon was left running for subsequent failure scenarios. No Docker Desktop
shutdown, login/logout, reboot, native production service change, or shared ingress
change was performed.

## Scoped engine-connectivity outage

`python3 deploy/controller/test-engine-outage.py` runs a reproducible lab-only
outage without stopping Docker Desktop or other workloads. It temporarily routes
the dedicated LaunchAgent through `engine-outage.py`, closes its active forwarding
streams with SIGUSR1, and restores forwarding with SIGUSR2. The `finally` cleanup
restores the original direct socket endpoint. Run only with the lab stack Ready.

Measured 2026-10-07: loss detected in 5.033s, phase `Degraded`, resource/dependency/API
conditions `Unknown` with reason `EngineObservationUnavailable`. Direct Docker Engine
29.8.2 remained healthy. After restoring forwarding, fresh Ready observation took
4.825s; generation 3 and all managed resource IDs stayed unchanged. The original
LaunchAgent endpoint was restored and fresh Ready was verified afterward. This
proves controller handling of engine connectivity loss; it does not prove actual
Docker Desktop shutdown/restart or VM reboot behavior.

## Final teardown evidence

Deletion converged to Deleted, tombstone true, generation4. No managed containers
or network remained; only the original database/artifact volumes were retained
with matching incarnations. Repeated delete retained generation4. The temporary
LaunchAgent was then uninstalled; the controller state and workload data remain.
The local build registry remains available, outside the controller lifecycle.

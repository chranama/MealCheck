# Controller failure policy and reproducible core proof

The controller owns workload recovery. Docker-managed containers use no automatic
restart policy, and launchd supervises only the controller process. Its durable
SQLite state is independent of the managed PostgreSQL database.

## Bounds

| Policy | Default | Persistence and reset |
| --- | --- | --- |
| Engine call deadline | 15 seconds | Context deadline for each observation or mutation. |
| Reconciliation poll | 5 seconds | Accepted changes also enqueue a coalesced wake-up. |
| Startup/degraded readiness window | 120 seconds per service | First unready observation/start time persists across controller restarts. Exhaustion requires explicit retry. |
| Transient failure retry | Exponential 1–30 seconds with jitter | Count and next eligible time persist. Successful mutation or complete readiness resets transient backoff. |
| Container start budget | At most 5 starts in 10 minutes, including initial start | Attempt timestamps persist before calls. Old attempts expire after 10 minutes; explicit retry resets the budget. |
| Event retention | Latest 1,000 events | Journal retains latest 1,000 completed entries plus unresolved intents. |

`retry` is an explicit operator command. It resets retry scheduling, readiness
windows, start attempts, and permanent-failure intervention state without changing
desired generation. Identical `apply` does not reset budgets. Stop/start does not
clear recorded start attempts. Deletion remains terminal and cannot be retried
into a Running deployment.

An engine observation error means **unknown**, not missing. No resource mutation
occurs from that observation. Status becomes Degraded/Unknown and preserves the
last successful observation time. After an uncertain mutation, the next eligible
pass re-observes deterministic resource identities before choosing another action.

Resource name and ownership labels must match the installation, deployment, role,
and immutable workload fingerprint. Previously bound persistent volumes must also
retain their exact engine identity: copied labels on a replacement volume do not
permit an empty data replacement. Conflicts and missing/changed volumes block
without adopting or destroying unrelated resources.

Running reconciliation creates the network and volumes, starts PostgreSQL and
waits for readiness, starts the model and waits for readiness, then starts the API.
Stop/delete proceed in reverse service order. Delete removes containers/network
and inventories retained volumes. Healthy dependencies are not recreated because
another dependency is unready.

Provider errors are classified rather than printed into state or events. Permanent
Running-operation failures require operator retry after the cause is corrected.
A failing create has an intent journal entry; successful observation determines
whether it actually happened. A provider timeout does not authorize a duplicate
create. Acceptance changes generation durably; older in-flight outcomes cannot
mark a newer generation Ready. Operator retry has an epoch so an older status
write cannot overwrite its reset.

## Repeat the fake-provider proof

From the MealCheck repository, using a Go 1.26 toolchain:

```sh
go test -race ./internal/infra/controller ./internal/infra/state ./internal/infra/spec
go test -race ./internal/infra/control ./cmd/mealcheck-controller
```

The second command binds local Unix sockets. If an execution sandbox prohibits
socket binding, run that command in a normal local terminal; do not silently skip
the socket permission or stale-socket tests. macOS uses short temporary socket
paths to stay below the Unix socket path-length limit.

The controller tests use a controllable clock and fake observations; they do not
wait ten minutes to exercise a ten-minute budget.

| Scenario | Test evidence |
| --- | --- |
| Running → Stopped → Running → Deleted; retain volumes | `TestLifecycleAndIdempotency` |
| Success followed by create timeout; no duplicate | `TestCrashAndUnknownCreate` |
| Close/reopen durable state after create-before-commit crash | `TestCrashReopensDurableStore` |
| Delete accepted during creation wins on next pass | `TestDeleteDuringCreate` |
| Engine unknown, unowned collision, removed bound data | `TestUnavailableCollisionAndDataLoss` |
| Copied labels cannot replace a bound volume | `TestCopiedLabelsCannotReplaceBoundVolume` |
| Stop/delete clear stale readiness | `TestLifecycleClearsReadyConditions` |
| Five starts, identical-apply resistance, persisted budget, window expiry | `TestPersistedRestartBudgetAndExplicitRetry` |
| Readiness exhaustion and explicit intervention | `TestReadinessWindowRequiresExplicitRetry` |
| Exponential bounds and no outage provisioning | `TestBackoffUnknownObservationAndSanitization` |
| Bounded call, permanent failure, raw-error privacy | `TestBoundedCallsPermanentFailuresAndPrivacy` |
| Stable condition transition timestamps | `TestStableConditionTransitionTime` |
| Operator reset survives stale in-flight status | `TestRetryEpochPreventsLostOperatorReset` |
| Private socket, protocol rejection | `TestSocketPermissionsAndProtocol` |
| Live/stale socket, regular file and symlink refusal | `TestStaleSocketAndUnsafePaths` |
| Strict requests, terminal transitions, canonical model path | `TestHandlerLifecycleAndRejections` |

## Evidence limits

These tests prove core policy and durable state behavior against a fake provider.
They do not prove image compatibility, model inference, Docker resource recovery,
completed-report preservation, login/reboot behavior, or native Linux operation.
The real Docker walkthrough must separately record detection, action, readiness,
and successful product-workflow timings. A healthy container or API endpoint is
not evidence that a real model-backed meal-checking workflow succeeds.

The controller cannot recover private inputs lost from MealCheck's in-memory
input vault. Interrupted-run behavior remains an application responsibility.
The runtime smoke tests use synthetic inputs and isolated state, volumes, ports,
and credentials. Sensitive inputs and secret contents must not appear in evidence.

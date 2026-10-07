# Go Resource Controller for MealCheck

Date: 2026-10-06  
Status: proposed; no controller implementation or runtime verification claimed

## Purpose

Build a Go service that maintains a requested MealCheck deployment on one container-engine
host. MealCheck is the workload; the controller manages the resources that run
it. The controller creates, observes, starts, stops, and repairs containers,
networks, and persistent volumes through a container-engine API.

Develop the controller natively on macOS, first with a fake provider and then
with a local Docker engine running Linux containers in a VM. Native Linux host
operation is a separate verification gate. See the
[implementation plan](resource-controller-implementation-plan.md) for milestones.
The existing macOS deployment remains independently operated. This proposal
does not change the active product priority order.

## Starting Point

MealCheck already contains a Go HTTP API, background run worker, PostgreSQL
state store, filesystem artifacts, and clients for a private llama.cpp endpoint.
The server starts its worker and cleanup goroutines in the same process.
Deployment currently uses shell scripts and launchd service templates; shared
Cloudflare ingress has separate ownership.

The controller must use existing application configuration and workflow
contracts. It must not read meal plans, change run states, retain provider keys,
or implement nutrition logic. Process recovery cannot recover sensitive inputs
held only in the application's in-memory input vault. Interrupted-run behavior
remains an application responsibility and a prerequisite for safe rollout.

## MVP Resource Boundary

A deployment contains:

- One private Docker network.
- One PostgreSQL container and one persistent database volume.
- One llama.cpp container with a pinned image and a read-only, pre-staged model.
- One MealCheck API/worker container with a pinned image and persistent artifact
  volume. Runtime assets must be included in the image or explicitly mounted.
- A loopback-only host API port for operator access and isolated smoke tests.

Use CPU inference and one approved model profile initially. Prebuilt images,
model files, secret files, and a working container-engine host are prerequisites. Their existence
and compatibility must be checked; the controller does not build images,
download arbitrary models, provision a host, or install Docker.

Start with one deployment per controller instance. No public routing,
Cloudflare changes, native launchd workload provider, Kubernetes operator, horizontal scaling,
rolling update, automatic data migration, or cloud account access in MVP.
Do not use Docker Compose as the controller's execution backend: the Go provider
must call the engine API directly so ownership and failure handling are explicit.

## User Contract

The operator supplies a versioned JSON desired-state document through a local
CLI. The controller persists it before acknowledging acceptance. Applying an
identical specification is a no-op; a meaningful change increments generation.

Illustrative document; image digests and paths below are placeholders:

```json
{
  "apiVersion": "mealcheck.dev/v1alpha1",
  "deploymentID": "mealcheck-lab",
  "desiredState": "Running",
  "spec": {
    "profile": "cpu-local-model-v1",
    "apiImage": "registry/mealcheck@sha256:<digest>",
    "postgresImage": "postgres@sha256:<digest>",
    "modelImage": "registry/llama-server@sha256:<digest>",
    "modelPath": "/srv/mealcheck-models/approved-model.gguf",
    "secretProfile": "mealcheck-lab",
    "apiHostPort": 18080,
    "dataPolicy": "Retain"
  }
}
```

Profiles fix internal ports, service names, UID/GID, environment variable names,
resource limits, supported image contracts, and private dependency addresses.
Map these to MealCheck's actual configuration during packaging; do not assume
loopback addresses work between separate containers. Use private service DNS
for PostgreSQL and llama.cpp, with no host port publication for either.

Accepted desired states:

| State | Meaning |
| --- | --- |
| Running | Required resources exist; containers are running; dependencies and application are ready. |
| Stopped | Existing owned containers are stopped; data and resource identity are retained. An initially stopped deployment needs no provisioning. |
| Deleted | Owned containers and network are removed; persistent data is retained. |

Deletion is terminal for a deployment ID. Keep a durable tombstone and do not
silently recreate a deleted deployment. Destructive volume purge is excluded
from MVP. Retained volumes remain inventoried with owner and retention status.

CLI operations: `apply --file`, `get`, `stop`, `start`, `delete`, and `events`.
`apply` returns deployment ID and accepted generation; it does not claim readiness.
`get` reports desired state, phase, observed generation, conditions, resources,
last successful observation, and pending retry. Status and events are redacted.

The daemon exposes a Unix-domain socket restricted to the operator account.
There is no public control API. Socket protocol must reject unknown schema
versions, unknown fields, unsupported profiles, unpinned images, invalid IDs,
port conflicts, and paths outside configured allowlists.

## Architecture and Persistence

Proposed implementation layout:

```text
cmd/mealcheck-controller/          daemon and CLI entrypoints
internal/infra/spec/               validation and profile resolution
internal/infra/state/              durable deployment, operation, event store
internal/infra/controller/         queue, planning, reconciliation, status
internal/infra/provider/           provider contract and error classification
internal/infra/provider/docker/    Docker Engine API adapter
internal/infra/provider/fake/      deterministic fault injection
internal/infra/health/             bounded readiness probes
```

Use a separate SQLite controller database on the host. Existing SQLite support
makes this a reasonable initial choice; verify the driver's runtime behavior.
Do not put controller state inside the managed PostgreSQL database. Persist
accepted spec, generation, deletion tombstone, resource bindings, operation
journal, conditions, retry count, and next eligible retry time. Never store
secret contents or meal inputs there.

Acquire a process-lifetime exclusive OS lock before opening the mutation loop.
Reject a second controller against the same state directory. MVP supports one
writer and serial reconciliation per deployment. It does not implement a
multi-host lease or distributed fencing protocol.

A provider supports bounded observation plus ensure/create, start, stop, remove,
and readiness-related inspection of owned resources. All calls accept contexts.
The controller owns policy; the adapter owns Docker-specific translation.
No arbitrary shell command or user-supplied executable is accepted.

## Ownership and Idempotency

Each resource has a deterministic name and labels containing controller
installation UUID, deployment ID, resource role, and specification fingerprint.
The installation UUID is generated once and persisted. Generation is recorded
for diagnosis but must not make persistent-volume identity change on each apply.

Before mutation, inspect ownership and role. A name collision with an unowned
resource produces `OwnershipConflict`; never adopt, stop, or remove it. After a
create conflict, re-observe and accept only a compatible owned resource.
Preserve controller state backups: state loss is not permission to adopt old
resources. Automatic adoption after lost state is excluded from MVP.

For each external mutation:

1. Persist operation intent with a unique operation ID and target fingerprint.
2. Invoke the provider with a deadline.
3. Re-observe the engine to determine the actual result.
4. Persist confirmed resource binding and operation outcome.

A timeout is an unknown outcome, not proof of failure. Re-observe before retrying
creation. If creation succeeded before the controller crashed, the next pass
finds the owned resource by deterministic identity and records it rather than
creating a duplicate. Journal entries aid diagnosis; observed provider state
remains necessary to resolve incomplete operations.

## Reconciliation Algorithm

Reconcile on accepted changes, daemon startup, retry expiry, and periodic drift
inspection. Poll every 5 seconds by default; event subscriptions are optional
later. Treat engine events as hints rather than authoritative state.

Each pass:

1. Load desired state and generation; observe all expected owned resources.
2. Classify observation as known, missing, or unknown. Engine unavailability
   must never be interpreted as all resources missing.
3. Resolve incomplete journal operations through observation.
4. Choose one bounded next action from current desired and observed state.
5. Recheck generation and deletion intent before beginning a mutation.
6. Perform the action, re-observe, and persist status for the evaluated generation.
7. Requeue if more work or delayed readiness checks remain.

Do not hold a database transaction across engine calls. A concurrent `apply`
may supersede an in-flight action; record its actual outcome and converge to the
latest accepted generation on the next pass. Never publish the old generation
as ready for the new spec. A deletion request is processed after any in-flight
bounded call resolves, and the next pass removes any resource it created.

Running creation order: network and volumes, PostgreSQL and its readiness,
llama.cpp and its readiness, then MealCheck and its readiness. Readiness failure
sets a condition; it does not repeatedly recreate healthy dependencies.

Stopped order: MealCheck, llama.cpp, PostgreSQL. Deleted order: stop/remove
MealCheck, llama.cpp, PostgreSQL; remove the private network; retain volumes.
Never delete unowned attachments to force network removal. Report a blocked
cleanup and retry after the conflict is resolved.

## Status and Failure Policy

Phases: `Pending`, `Provisioning`, `Ready`, `Degraded`, `Stopped`, `Deleting`,
`Deleted`, and `Blocked`. Phase summarizes the observation; conditions explain
it. Include `ResourcesReady`, `DependenciesReady`, `ApplicationReady`, and
`Progressing`, each with reason, transition time, and evaluated generation.

`Ready` requires all readiness conditions true for the current generation.
An engine outage immediately makes status degraded/unknown on the next failed
observation; do not present stale readiness as current. Include observation time.

| Failure | Controller response |
| --- | --- |
| Container process exits | Restart the owned container subject to the restart budget. |
| Container unexpectedly disappears | Recreate it with the same persistent data bindings. |
| Engine unreachable or request outcome unknown | Record degraded observation; re-observe before mutation. |
| Database/model unready | Report the failed dependency; avoid restarting the whole stack. |
| Model answers health probes but cannot infer | Report smoke-test failure; do not call this verified readiness. |
| Missing retained database/artifact volume | Block; do not silently create an empty replacement. |
| Port collision, invalid mount, incompatible resource | Block with actionable reason. |
| Image unavailable or transient engine error | Retry with bounded backoff. |
| Repeated crash or readiness failure | Exhaust budget, retain diagnostics, require operator intervention. |

Defaults: engine call deadline 15 seconds; health probe deadline 2 seconds;
startup readiness window 120 seconds per service; transient retry backoff
1–30 seconds with jitter; at most 5 automatic starts per container in 10 minutes.
An exhausted start budget resets only after the configured window or explicit
operator retry; an identical apply does not reset it. These are proposed bounds,
not measured performance. Tune the model readiness window from real CPU tests.

Use no automatic engine restart policy for managed containers in the initial
profile; the controller owns recovery decisions. A separate host supervisor may
restart the controller daemon. If the controller is down, existing containers
keep running but reconciliation stops.

For MVP, reject image, model, port, or profile changes after initial acceptance.
Require a separately designed update workflow before permitting replacements.
Start/stop changes remain supported. This avoids introducing destructive database
version changes or unverified release rollback into the first resource lifecycle.

## Data and Privilege Boundaries

Database and artifacts survive container restart, controller restart, stop, and
deployment deletion. Model files are read-only. Check UID/GID and write access
before considering the application ready. Specify backup and restore separately;
a retained volume is not a backup and one-host deployment is not high availability.

Resolve `secretProfile` to preconfigured host files outside the repository.
Mount secrets read-only; never accept secret text in the desired-state document.
The packaging adapter must map files to existing MealCheck/PostgreSQL configuration
without logging values. If an environment-only interface requires a startup
wrapper, document the residual visibility to Docker administrators. Scope any
credential rotation workflow separately.

Docker socket access grants broad host control. Run the experiment on an
isolated host; socket permissions restrict callers but do not remove that
privilege. Restrict image registries, mount roots, ports, and deployment profiles.
Do not expose provider inspection output that contains environment secrets.
Use structured logs containing operation ID, deployment, generation, role,
action, duration, and classified outcome; sanitize engine error messages.

## Verification and Acceptance

First establish a packaged Linux workload that passes the existing real-model
workflow: normalize, review, deterministic verification, accessible report.
Health endpoints alone are insufficient. Use synthetic meal inputs and an
explicit isolated base URL for smoke scripts. Linux/model compatibility remains
unverified until this gate passes.

| Scenario | Required evidence |
| --- | --- |
| Fresh Running apply | One stack reaches Ready for the accepted generation; real workflow succeeds. |
| Repeat identical apply | No additional resources, restarts, or generation changes. |
| Crash after successful create before state commit | Restart discovers the same resource; no duplicate. |
| Timeout after create succeeds | Observation resolves the unknown outcome without duplicate provisioning. |
| Concurrent start/stop or delete during create | Final resources converge to latest accepted state; stale generation is never Ready. |
| Kill API container | Controller restores it within polling/retry bounds; completed reports remain accessible. |
| Kill model container | Controller restores it; application surfaces bounded dependency failure and later accepts new work. |
| Engine outage | No speculative replacements; status reports unknown observation, then converges after recovery. |
| Remove persistent volume | Controller blocks with data-loss condition instead of booting an empty replacement. |
| Unowned name/network collision | Controller reports conflict and leaves unrelated resources untouched. |
| Stop/start | Containers stop and resume; database and reports persist. |
| Delete twice, then restart controller | Containers/network remain absent; retained data and tombstone remain recorded. |
| Crash-loop budget | Attempts stop at the documented limit; actionable status appears. |
| Two daemons, same state directory | Second writer fails before resource mutation. |
| Privacy inspection | No sensitive inputs or secret contents in state, logs, CLI output, or evidence. |

Fake-provider tests must control time and inject failures at the mutation/commit
boundary. Real engine integration tests verify labels, network isolation, mounts,
collision behavior, and persistence. Run meaningful Go tests and the race detector
for controller/state packages. Do not claim the fake provider proves Linux or
Docker compatibility. Prefer measured convergence time over an untested promise.

## Implementation Sequence and Deliverables

1. Package and verify the Linux workload, including real inference and existing
   interrupted-run behavior. Resolve application recovery gaps separately.
2. Implement schema, durable store, exclusive lock, CLI, fake provider, and
   create/observe/stop/delete planning. Prove idempotency and crash recovery.
3. Implement the Docker provider and isolated engine integration tests.
4. Add readiness, retry budgets, drift repair, status, and sanitized diagnostics.
5. Run failure demonstrations and write a reproduction/cleanup runbook.

The first demonstrable slice is: apply Running, observe Ready, kill the API
container, observe recovery, stop/start, then delete while retaining data.
Complete the crash-after-create test before calling creation idempotent.

Deliver: controller source, pinned workload packaging, schema/example without
secrets, tests, operator runbook, and an evidence record with commit, versions,
host details, commands, observed timings, resource inventory, and limitations.
This is a multi-stage project, not an assumed eight-hour extension of the Linux
reliability sprint. Actual implementation estimates follow the packaging gate.

## Later VM Extension

After MVP passes, introduce a separate `ComputeInstance` resource managed through
one chosen provider's Go SDK. Specify host provisioning, bootstrap identity,
remote observation, operation tokens, uncertain-create recovery, fencing, cost
limits, and teardown semantics before implementation. A VM adapter is not a
small substitution for the Docker adapter: replacing a host changes storage,
connectivity, and controller failure boundaries.

That extension would support a precise claim about VM lifecycle control.
The MVP supports the narrower claim: a Go controller reconciles a persistent
MealCheck container deployment and recovers from measured resource failures.

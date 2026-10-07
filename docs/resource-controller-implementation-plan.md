# Go Resource Controller Implementation Plan

Date: 2026-10-06  
Status: M0–M6 implemented; M0–M5 validated; M6 current-session checks validated, login/reboot unverified; M7 deferred

## Outcome and Scope

Implement the [resource controller specification](resource-controller-spec.md)
as an isolated operations component. Develop the Go controller on the current
Mac; use a fake provider for core tests, then Docker's Linux VM for real workload
integration. Native Linux host operation (M7) is deferred by the current implementation request.
No separate physical machine or cloud VM is required to begin.

The controller manages one deployment's containers, private network, and durable
volumes. Existing MealCheck business logic and current native deployment keep
their responsibilities. Release updates, public ingress changes, volume purge,
VM provisioning, and fleet scheduling remain outside this plan.

A milestone is complete only when its exit evidence is recorded. Passing fake
provider tests, container tests on macOS, and native Linux tests support distinct
claims; record which environment was actually exercised.

## Milestone Map

| Milestone | Deliverable | Dependency | Exit gate |
| --- | --- | --- | --- |
| M0 | Environment and workload contract | None | Exact local constraints and isolated deployment profile documented. |
| M1 | Verified containerized MealCheck workload | M0 | Real model-backed workflow and persistent-data restart checks pass. |
| M2 | Durable desired state and operator CLI | M0 | Accepted state survives restart; single-writer and validation checks pass. |
| M3 | Reconciliation against fake resources | M2 | Lifecycle, idempotency, and uncertain-outcome tests pass. |
| M4 | Real Docker resource provider | M1, M3 | Controller creates, stops, starts, and deletes an isolated stack safely. |
| M5 | Readiness, drift repair, and failure budgets | M4 | Measured resource recovery and failure-policy checks pass. |
| M6 | Mac supervision and reproducible demonstration | M5 | Controller restart and Docker outage walkthroughs pass on the Mac. |
| M7 | Native Linux host verification | M6 | Same lifecycle and recovery gates pass with Docker Engine and systemd. |

M1 and M2 can progress independently after M0. If Docker setup is blocked,
continue M2–M3. M4 cannot pass until the workload packaging gate passes.
This ordering does not imply parallel agent delegation.

## M0 — Establish Environment and Workload Contracts

### Implementation tasks

- Inspect the current Mac's architecture, available memory/disk, Go toolchain,
  Docker client/engine availability, engine endpoint, and existing port usage.
  Record observations rather than assuming Docker is installed or running.
- Choose a compatible CPU model and pinned container images. On Apple Silicon,
  prefer compatible ARM64 images; document any emulation and measured impact.
- Map MealCheck configuration to container DNS, database credentials, runtime
  assets, model path, artifacts, and permissions. Inspect the actual configuration
  code and existing smoke scripts before selecting variable names.
- Define an isolated `mealcheck-lab` profile: separate port, state directory,
  volumes, network, credentials, model mount, and resource limits. Never point
  the experiment at the current native deployment's database or artifact paths.
- Identify which application restart scenarios lose in-memory input. Define
  expected interrupted-run outcomes; track necessary application fixes separately
  as separately reviewable application changes.

### Deliverables and exit criteria

Create a workload contract under `deploy/controller/README.md` describing image
contracts, architecture, dependency addresses, mounts, secret handling, resources,
engine connection, and cleanup. Record the environment findings and unresolved
blockers. Exit when the configuration is concrete enough to package and validate;
an unavailable engine blocks M1/M4, not the fake-provider implementation.

## M1 — Package and Verify the Workload

### Implementation tasks

- Add a reproducible MealCheck container build with required runtime assets and
  explicit user permissions. Pin build inputs and the selected model-server
  runtime. Resolve model availability before testing.
- Provide a local packaging/setup harness for running the three containers on
  a private network. This may use development tooling; the eventual controller
  provider must call Docker APIs directly.
- Publish only the lab API port on loopback. Keep PostgreSQL/model ports private.
- Initialize PostgreSQL and supply credentials through the agreed secret-file
  adapter. Pre-stage the model read-only and provision separate persistent
  database/artifact volumes.
- Run the existing synthetic-input real-model workflow with an explicit lab URL:
  normalize, review, deterministic checks, and retrieve report artifacts.
- Restart the API and dependencies; inspect completed-data persistence and
  interrupted-run behavior. Fix only demonstrated application recovery gaps in
  a separately reviewable change.

### Deliverables and exit criteria

Commit container packaging, non-secret examples, and setup/cleanup instructions.
Record versions, image digests, architecture, real inference outcome, resource
usage, and restart results. Exit only when the product workflow succeeds and
completed database state/artifacts survive restart. Health responses alone do
not pass this milestone. Linux containers on a Mac do not verify systemd.

## M2 — Implement Durable Desired State and CLI

### Implementation tasks

- Implement strict v1alpha1 parsing and approved profiles in `internal/infra/spec`.
- Implement SQLite controller state outside the managed workload: installation
  UUID, spec, generation, tombstone, bindings, journal, conditions, retry state,
  and redacted events. Add a schema version and transactional initialization.
- Acquire an exclusive process-lifetime lock before resource mutation. Make a
  second daemon using the same state directory fail clearly.
- Implement the local Unix-socket daemon interface and CLI commands: `apply`,
  `get`, `start`, `stop`, `delete`, and `events`. Document command exit statuses.
- Persist changes before acknowledgment. Identical applies do not increment
  generation. Reject immutable-profile/image/model/port changes after acceptance.
- Define legal transitions and explicit retry behavior; terminal deletion keeps
  a tombstone. Bound event retention so the controller database cannot grow
  indefinitely through polling.

### Deliverables and exit criteria

The CLI accepts and displays desired state without touching real infrastructure.
Tests prove restart persistence, identical apply behavior, invalid input rejection,
immutable field rejection, terminal deletion, socket access restrictions, and
single-writer enforcement. No secret contents enter state or output.

## M3 — Implement Reconciliation with a Fake Provider

### Implementation tasks

- Define provider observations, owned-resource identity, action results, and
  classified errors. Distinguish missing resources from unknown observations.
- Implement serial reconciliation, periodic enqueue, desired generation checks,
  operation intent journaling, bounded calls, and post-action observation.
- Implement Running, Stopped, and Deleted lifecycle ordering. Preserve volume
  identity and block missing previously bound data volumes.
- Implement a fake provider with controllable time, resources, and failure points.
  Simulate success before timeout, crash after create before commit, name
  collisions, engine outage, and concurrent desired-state changes.
- Recover incomplete operations by observation rather than blindly repeating
  creation. Keep stale evaluated generations from satisfying current readiness.

### Deliverables and exit criteria

Meaningful controller/state tests and the race detector pass. Demonstrate that
repeated reconciliation converges without duplicate resources, deletion wins
against an in-flight create on the subsequent pass, unrelated resources remain
untouched, and restart resolves incomplete operations. Tests must use controllable
scheduling rather than long sleeps. This milestone proves policy, not Docker
compatibility.

## M4 — Implement the Docker Provider

### Implementation tasks

- Select the maintained Go engine client and pin its dependency after checking
  current official API documentation. Define supported engine/API versions and
  version negotiation behavior in the workload contract.
- Support explicit local engine endpoints for macOS and Linux. Do not assume
  the macOS socket lives at the Linux default path. Keep remote TCP access out
  of the initial profile.
- Implement network, volume, and container inspection/create/start/stop/remove
  using contexts, deterministic names, ownership labels, and fingerprints.
- Implement mount/secret adapters and permission checks. Configure managed
  containers without automatic engine restart policies.
- Re-inspect on conflict or uncertain outcomes; distinguish an incompatible
  owned resource from a compatible one. Never adopt an unowned name collision.
- Connect the planner to the real provider and inspect actual inventory after
  each lifecycle operation.

### Deliverables and exit criteria

On the isolated Mac Docker engine, the controller provisions one working stack;
repeat apply creates no extra resources or restarts; stop/start retains reports;
delete removes containers/network and retains volumes. Real integration tests
prove ownership, private ports, mounts, permissions, and collision handling.
The controller invokes no Compose or arbitrary shell commands to manage resources.

## M5 — Add Readiness and Failure Recovery

### Implementation tasks

- Add dependency-aware probes and current-generation status conditions. Separate
  resource existence, process health, dependency health, and product smoke proof.
- Add periodic drift inspection, transient backoff with jitter, persisted retry
  scheduling, startup windows, and restart budgets from the specification.
- Record sanitized action events and last successful observation. Engine outage
  marks observation unknown/degraded and never triggers speculative replacement.
- Inject API/model exit, unexpected container removal, engine outage, missing
  data volume, repeated startup failure, and controller crash during mutation.
- Provide the operator's explicit retry mechanism without allowing identical
  apply to evade restart budgets. Verify clean context cancellation and daemon
  shutdown during bounded provider calls.

### Deliverables and exit criteria

All specification failure scenarios have measured outcomes or an explicit
remaining blocker. In particular: API recovery preserves completed reports;
model recovery permits a subsequent successful real workflow; missing data
blocks; crash-loop attempts stop at the budget; engine recovery produces no
duplicates. Interrupted user jobs follow the application's documented policy,
not an invented controller-level retry promise.

Record detection, action, readiness, and end-to-end recovery timings separately.
Do not equate a container's running state with successful inference.

## M6 — Integrate Mac Supervision and Demonstrate Operations

### Implementation tasks

- Add a launchd template for the native Go controller under a dedicated lab label.
  Explicitly configure state path, socket, engine endpoint, log destination,
  environment, and executable path for the selected account.
- Determine LaunchAgent versus LaunchDaemon from the actual Docker runtime and
  account model. Verify behavior at login and reboot; do not assume a user's
  Docker Desktop engine is available before login to a system daemon.
- Let launchd restart the controller process; let the controller own workload
  recovery. An unavailable engine must yield backoff and actionable status,
  rather than restarting Docker Desktop or fighting its lifecycle.
- Record a walkthrough: apply, real workflow, kill API, recovery, controller
  restart, Docker outage/recovery, stop/start, delete, retained-data inspection.
- Add reproducible installation, inspection, log access, uninstall, and isolated
  resource cleanup instructions. Retained-volume purge remains manual and outside
  controller MVP commands.

### Deliverables and exit criteria

Launchd restarts a killed controller and reconciliation resumes from durable
state without duplicates. Engine unavailability and the verified login/reboot
behavior are documented. Existing native MealCheck services and shared ingress
are untouched. A new operator can repeat the demonstration using the runbook.

At this point the Mac-based container-controller MVP is complete. Its evidence
supports container lifecycle and recovery claims on the tested Mac runtime.

## M7 — Verify Native Linux Operation

### Implementation tasks

- Use an isolated local Linux VM or separately selected Linux host. A paid cloud
  host is optional and is a separate provisioning step.
- Install the same pinned workload on native Docker Engine with Linux-specific
  paths and permissions; build/run the Go controller for the target architecture.
- Add a systemd unit and document controller account, engine socket privileges,
  startup dependencies, state ownership, and logs.
- Repeat the lifecycle, real workflow, restart persistence, uncertain-create,
  collision, engine outage, and failure-budget gates. Verify reboot startup.
- Record any host-specific changes; keep provider policy shared across platforms.

### Deliverables and exit criteria

A Linux runbook and evidence record show the same resource lifecycle and recovery
behavior, including systemd startup after reboot. Linux readiness is claimed only
after this gate. VM creation/scheduling remains outside the implemented controller.

## Evidence and Completion Tracking

Maintain `docs/resource-controller-evidence.md` during implementation. Each
milestone entry contains status, commit, environment, versions, commands, observed
results, measured timings where relevant, and remaining limitations. Keep synthetic
inputs and redact secrets. Link failures to the change that resolves them.

| Completion checkpoint | Required milestones |
| --- | --- |
| Core Go policy demonstration | M0, M2, M3 |
| Working real-resource controller | M0–M5 |
| Supervised Mac MVP | M0–M6 |
| Verified native Linux deployment | M0–M7 |

Do not assign an eight-hour deadline to this project. Estimate implementation time
after M0 establishes the environment and M1 exposes packaging/recovery work.
A future VM milestone requires a new provider/lifecycle design rather than being
silently included in this plan.

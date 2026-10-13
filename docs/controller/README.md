# MealCheck Go resource controller

The controller maintains one requested MealCheck deployment on a container-engine
host. It owns container, network, and retained-volume lifecycle through the Docker
API; MealCheck remains responsible for application logic and interrupted inputs.

## Reading order

1. [Specification](specification.md): architecture, ownership, reconciliation and contracts.
2. [Failure policy](failure-policy.md): uncertain outcomes, retries, readiness and data safety.
3. [Verification ledger](evidence/README.md): implemented milestones, observed results and limits.
4. [Mac operations](operations/macos.md): installation, lifecycle and inspection.

## Design and operation

- [Workload contract](workload.md): resources, prerequisites and historical packaging profile.
- [Command interface](cli.md): daemon, CLI and private socket behavior.
- [Manifest contracts](manifests.md): trusted systems and constrained deployments.
- [Implementation plan](implementation-plan.md): milestone acceptance criteria and remaining work.
- [Deployment assets](../../deploy/controller/README.md): packaging, supervision, verification tools and JSON definitions.

Go entrypoint: `cmd/mealcheck-controller`; implementation: `internal/infra`.

## Status and evidence boundaries

M0–M7 are implemented, including manifest separation. Recorded verification
includes fake-provider/race tests and real Docker/model workflows on ARM64 and
Intel Macs. Native Linux/systemd operation is deferred. Login/reboot and actual
Docker Desktop shutdown/restart remain unverified. See the ledger for precise
results; source acceptance is not completion of those runtime gates.

This directory holds current documentation and historical evidence. Deployment
assets live under `deploy/controller`; evidence artifacts are not runtime state.

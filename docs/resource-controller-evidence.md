# Resource Controller Evidence

Implementation scope: M0–M6. M7 native Linux host operation is deferred.

## M0 — Environment and workload contract

Date: 2026-10-07. Status: verified and independently reviewed.

Contract commit: `f591ac4`. PR: https://github.com/chranama/MealCheck/pull/2.

Observed macOS ARM64 with Go 1.26.4, 128 GiB RAM, 1.6 TiB free disk; Docker
Desktop 4.94.0 / Engine 29.8.2 linux/arm64 at the desktop Unix socket. Verified
local Qwen3-0.6B-Q4_K_M model availability and no listener on port 18080.
Commands: `uname -m`, `go version`, `sysctl -n hw.memsize`, `df -h`, Docker
context/info, model inventory, and workload configuration source inspection.

The independent review requested explicit M7 deferral and milestone evidence;
these corrections are included. Runtime/model image digest proof belongs to M1.
No production services or public ingress were changed.

## M1–M6 — Implemented and reviewed

| Milestone | PR | Evidence |
| --- | --- | --- |
| M1 | [#3](https://github.com/chranama/MealCheck/pull/3) | [Packaging and interrupted-run recovery](../deploy/controller/packaging-evidence.md) |
| M2 | [#4](https://github.com/chranama/MealCheck/pull/4) | Durable restart, validation, lifecycle, socket permissions/protocol and locking race tests |
| M3 | [#5](https://github.com/chranama/MealCheck/pull/5) | Fake lifecycle, uncertain outcomes, actual state reopen, concurrent delete and data identity race tests |
| M4 | [#6](https://github.com/chranama/MealCheck/pull/6) | [Real Docker lifecycle](../deploy/controller/docker-provider-evidence.md) |
| M5 | [#7](https://github.com/chranama/MealCheck/pull/7) | [Measured drift recovery](../deploy/controller/recovery-evidence.md), [failure policy tests](../deploy/controller/failure-policy.md) |
| M6 | [#8](https://github.com/chranama/MealCheck/pull/8) | [Mac supervision and scoped outage](../deploy/controller/mac-operations.md) |

An independent review agent reviewed every milestone. Findings were repaired
before source acceptance. The user authorized acceptance and merge of PRs #2–#8
in dependency order on 2026-10-07. Their live merge status is recorded on GitHub;
source acceptance and integration do not complete the remaining runtime gates.

Full controller race suites and opt-in real Docker ownership/incarnation test
passed. The repository-wide Go suite initially exposed a legacy assertion tied
to the former BYOK expired-input diagnostic; M1 now checks the actionable updated
message. A second full real model workflow passed after API/model drift recovery.

Final deletion left no managed containers/network, retained both bound data
volumes and generation4 tombstone, and was idempotent. The temporary LaunchAgent
was uninstalled. Packaging volumes and controller volumes remain retained; the
loopback build registry remains outside managed workload scope.

M6 login/reboot and actual Docker Desktop shutdown/restart were not exercised.
The scoped connection outage kept the shared engine running. Those runtime gates
remain unverified, and the original M6 exit gate is therefore only partially met.
No workstation reboot or native production service change was performed.
M7 native Linux host operation/systemd remains deferred by user instruction.

## Final validation and review ledger

`go test ./...` passed after the diagnostic assertion repair. The final
`go test -race ./internal/infra/... ./cmd/mealcheck-controller
./internal/runs/execution ./internal/state/postgres` suite passed. Shell syntax,
Python syntax and plist parsing passed. PostgreSQL's opt-in live regression and
Docker's opt-in live ownership/incarnation tests were exercised separately as
recorded in M1/M4; ordinary test runs skip those live gates without explicit
configuration.

The independent review agent accepted these source snapshots:

| PR | Reviewed head | Outcome |
| --- | --- | --- |
| #2 | `d00537e` | M0 accepted |
| #3 | `4ef7506` | M1 accepted, including final regression repair |
| #4 | `29cf5e0` | M2 accepted |
| #5 | `851a0d5` | M3 accepted |
| #6 | `aa27b37` | M4 accepted |
| #7 | `22729e2` | M5 accepted |
| #8 | `f4ebeb4` | M6 source accepted; original runtime gate partial |

Later evidence-only updates do not change those implementation snapshots.
Acceptance here means an automated independent-agent source review, not a human
GitHub approval or merge. Operator login/reboot verification remains outstanding.

## Merge-stage CI repair

GitHub's Ubuntu runner exposed two socket tests that assumed the macOS path
`/private/tmp` existed. They now use the short Unix path `/tmp`, preserving the
same permission, protocol and stale-socket assertions. The fix originated in
PR #4 (`028b154`) and was propagated through dependent milestones before merging.
Mac socket/CLI race tests passed after the repair; GitHub CI is the independent
Ubuntu regression gate. This does not establish M7 native Linux deployment.

The integration request accepts the implemented Mac controller with the recorded
M6 login/reboot and actual Docker Desktop shutdown/restart limits unchanged.

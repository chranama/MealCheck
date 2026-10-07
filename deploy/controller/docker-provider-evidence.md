# M4 Docker provider evidence — 2026-10-07

Client: github.com/moby/moby/client v0.6.1 and API v1.56.1, maintained split Go
SDK with automatic Engine API negotiation. See Docker's [official SDK
reference](https://docs.docker.com/reference/api/engine/sdk/). Tested Engine29.8.2
(API1.56, minimum1.40), Docker Desktop4.94.0 linux/arm64. Other engine versions
are negotiated but not runtime verified. Only explicit local Unix endpoints are
accepted; no shell/Compose invocation in the provider.

The native controller provisioned network, two volumes, PostgreSQL, llama.cpp,
and MealCheck API under installation82dac727 deploymentmealcheck-lab. All six
resource identities were recorded; volume bindings use random incarnation labels
because Docker volume names are reused after deletion. Actual profile settings
are checked alongside ownership/fingerprint labels.

Real normalization, operator review confirmation, deterministic checks, and
report artifacts passed for synthetic run `run_552b21f7b10c5eaf60b9ae80`. API port
was loopback18080; database/model had no published ports. Containers used no
engine restart policy; bind secrets/model were read-only and API UID10001.

Stop reached Stopped in14.897s; start reached Ready in14.654s. All six resource
identities remained unchanged, and the report remained byte-identical:
`77b2e528d71fe0c670c89e22f376258d43e2b7d5833bcd294bcc8f9e420093f7`.
Repeated apply retained generation3. Timings include5s reconciliation polling;
this is a measured lab run, not a service-level guarantee.

Unit race tests cover fixed-profile compatibility, unsafe/escaping secret paths,
remote endpoint refusal, and sanitized unavailable-engine errors. Opt-in live
Engine test creates only uniquely named empty test resources and passed collision
refusal, idempotent volume ensure, distinct incarnation after volume recreation,
and retained-volume purge refusal:

```sh
MEALCHECK_CONTROLLER_TEST_ENGINE=unix://$HOME/.docker/run/docker.sock \
  go test -race ./internal/infra/provider/docker
```

Delete/retained inventory and failure-recovery walkthrough are recorded with
M5/M6. Linux containers on this Mac do not establish native Linux host operation.

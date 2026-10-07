# MealCheck container-controller workload contract

The `cpu-local-model-v1` profile is an isolated experiment, separate from the
native MealCheck server and Cloudflare ingress. The controller manages resources
through Docker's API; packaging scripts are development tools only.

## Environment baseline (2026-10-07)

Inspected development machine: macOS ARM64, 128 GiB physical RAM, 1.6 TiB free
workspace disk, Go 1.26.4 darwin/arm64. Docker Desktop 4.94.0 exposes Docker Engine
29.8.2 linux/arm64, API 1.56 (minimum 1.40). The selected context is `desktop-linux`
and endpoint is `unix:///Users/chranama/.docker/run/docker.sock`. Sandbox access
to this socket is restricted; inspection outside the sandbox succeeded. Port
18080 had no listener. Docker Desktop is a user-session prerequisite; the
controller must not start or restart Docker Desktop.

A real CPU model is available at
`/Users/chranama/infra/models/gguf/Qwen3-0.6B-Q4_K_M.gguf`. This is not a vocabulary
fixture. Pre-stage a lab copy under an explicitly allowed model root; never
modify the original. Model hash and container image digests must be recorded by
packaging before runtime acceptance. Use ARM64 images without emulation.

## Approved workload profile

| Resource | Contract |
| --- | --- |
| Network | Dedicated private bridge; deployment-scoped deterministic name |
| PostgreSQL | PostgreSQL 17, DNS `postgres`, port 5432; database/user `mealcheck`; persistent volume at `/var/lib/postgresql/data`; no published port |
| Model | CPU llama.cpp server, DNS `model`, port 8080; alias `mealcheck-lab-model`; read-only GGUF mount `/models/model.gguf`; no published port |
| API | Nonroot UID/GID 10001; port 8080; only `127.0.0.1:18080` published; artifact volume `/var/lib/mealcheck/artifacts` |
| State | Host directory outside application volumes; installation-specific SQLite and restricted Unix socket |
| Secrets | Host directory mode 0700; `postgres-password` and `database-url` files mode 0600; mounted read-only under `/run/secrets` |

API runtime configuration uses the actual `internal/core/config.go` names:
`MEALCHECK_ADDR=0.0.0.0:8080`, `MEALCHECK_STORE=postgres`,
`MEALCHECK_HOSTED_MODE=local_model`, `MEALCHECK_LOCAL_MODEL_ENABLED=true`,
`MEALCHECK_LOCAL_MODEL_BASE_URL=http://model:8080/v1`,
`MEALCHECK_LOCAL_MODEL_NAME=mealcheck-lab-model`,
`MEALCHECK_DATA_DIR=/var/lib/mealcheck`,
`MEALCHECK_ARTIFACT_DIR=/var/lib/mealcheck/artifacts`, and
`MEALCHECK_FNDDS_FALLBACK_PATH=/opt/mealcheck/data/reference/fndds-2021-2023/fndds.sqlite`.
The API startup adapter reads `database-url` into `DATABASE_URL`; PostgreSQL uses
`POSTGRES_PASSWORD_FILE`. Docker administrators can inspect API environment
credentials; the controller never copies their contents into state/events.

Build the API from this repository with runtime `data/` and `examples/` assets.
Images must be digest-pinned in accepted desired state. Limit the API to 2 CPUs
and 2 GiB, database to 2 CPUs and 1 GiB, and CPU model to 8 CPUs and 4 GiB. Engine
restart policies are disabled because controller policy owns recovery.

## Restart and data boundaries

Completed metadata and artifacts survive restart, stop/start and deletion. Data
volumes are retained; missing bound data blocks automatic replacement. A model
mount is read-only. Initially stopped deployments need no resources.

Sensitive pending inputs live in `runinput.Vault` in the API process. An API
restart loses these inputs; the controller cannot reconstruct them. Queued or
in-progress jobs must not be transparently replayed. Runtime packaging tests must
record their actual terminal/expiry behavior and any demonstrated application
recovery gap separately. Completed reports are the restart-persistence gate.

## Isolation and cleanup

Use deployment ID `mealcheck-lab`, dedicated credentials, a dedicated state root,
and new database/artifact volumes. Never mount native production databases or
artifact directories, or reuse its ports. Delete removes owned containers and
network but retains data volumes. Any manual purge must inspect installation and
deployment ownership labels first and is outside controller commands.

## Acceptance evidence

Packaging is accepted only after the existing synthetic real-model workflow
passes against explicit `http://127.0.0.1:18080`, including review confirmation and
report retrieval, then completed-data persistence passes after restart.
Environment availability alone is not M1 acceptance. Native Linux/systemd
operation is excluded from this implementation request.

## Packaging reproduction

Run from the repository root:

```bash
./deploy/controller/package-lab.sh prepare
./deploy/controller/package-lab.sh build
./deploy/controller/package-lab.sh up
./deploy/controller/package-lab.sh smoke
./deploy/controller/package-lab.sh restart
./deploy/controller/package-lab.sh down
```

`prepare` stages a checksum-verified model and random lab-only credentials outside
the repository. Override `MEALCHECK_LAB_ROOT` and `MEALCHECK_LAB_MODEL_SOURCE` for
another allowed local path. `up` refuses existing container/network names.
Volumes persist across `down`; their packaging ownership labels must match.
`smoke` explicitly targets loopback and retains synthetic runs for persistence
inspection. The API image includes `data`, `examples`, and `schemas`.

For a digest-pinned API reference without external publication, run a private
registry published only on loopback port 15000, tag the built image as
`localhost:15000/mealcheck:lab`, push, and record its resulting manifest digest.
Pre-pull that reference on the engine before applying desired state. The registry
is a packaging prerequisite, not a resource owned by the controller.

Hard-killed running jobs terminally fail once their existing worker lease expires
(default run timeout: ten minutes), on the next claim poll; they are never replayed.
This releases the serialized local-model queue. An unexpired lease remains active.
Queued jobs whose volatile input was lost fail with a resubmission message. This
is application policy, not a controller retry of meal inputs.

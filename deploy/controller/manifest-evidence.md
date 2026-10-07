# M7 manifest packaging evidence

Image metadata was inspected on 2026-10-07 through Docker `buildx imagetools inspect`.
The llama.cpp server index resolved to `sha256:33868c035b21dc63f7c60b7438774283fd99215bc319114eb03de5df4ce7cd6b`:

| Role | linux/arm64 manifest | linux/amd64 manifest |
| --- | --- | --- |
| llama.cpp | `a9bc9f7f3832c18a3d8f74a37d495c0ca4e40fe188b774d2bf08a279e502a943` | `a48c794aabc51dc5f9663c358b946b79d4cc02de67ab4d508c2c850fd0bc20f8` |
| PostgreSQL 17.6 | `5d11ffb37e58a7c9a2285359e50f7674e216c99b9114e47b0e7f21187c11252c` | `45cd22f8d32e189d245403954882f88e7a8714301fda80dab6da90f1265b25a3` |

The PostgreSQL variants belong to previously pinned multiarch index
`sha256:f3bd19c606e442c3d7bdfa8002e03fe260a1023351e0ea4598032022b68dd6e3`.
The committed systems use native manifest pins and avoid emulation.

The API Dockerfile previously used an ARM64-only Debian runtime pin. It now uses
parent multiarch index `sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587`,
which retains the existing ARM64 variant and includes native AMD64
`sha256:a4672c0cb26fbdde88e38fa2dfb6c681942306680e41e4378b28770b6e79ee91`.
The Go build-stage pin was already a multiarch index.

ARM64 API `localhost:15000/mealcheck@sha256:3d7ba167fc373f348e98bed7a73e865d981f334f7e37ec07e4da525c8ffb795f`
is the existing verified lab build. The Intel API was crosscompiled from this
repository using `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath`, placed
in the same Dockerfile runtime layout (nonroot UID/GID 10001, same data/examples/schema
assets and entrypoint) on the pinned native AMD64 Debian variant, and transferred
with Docker save/load. The Intel loopback registry digest is
`localhost:15000/mealcheck@sha256:1f83e630d607befe4785ef159ec161c6a723981892407122c254767f852f0c2e`.
Docker save/load converted the original OCI manifest to Docker schema media type;
the server's resulting immutable manifest is the one committed in its system.

Intel server prereqs were verified through SSH: macOS 14 Intel x86_64, Docker Desktop
29.8.2 Engine x86_64, four CPUs, 4102107136 bytes memory. Real GGUF copied into the
isolated test root from the existing staged lab copy, SHA256
`18ea1f301079bba6391ab6d455c0c8565fd5a3214075eb2cd9daf351dedc719b`.
Test controller/source/state/catalog are under
`/Users/chranama-server/MealCheck-controller-lab/m7-test` and `m7-source`.
The existing installed lab controller/supervisor and native MealCheck service are
untouched; no sudo, Docker allocation change, GUI keychain unlock, or ingress
change was required. Public image pull used explicit isolated Docker config and
socket flags because build-stage credential lookup over SSH required the GUI keychain.

## Intel native Mac Docker acceptance

The committed AMD64 system passed read-only plan and durable apply at generation 1,
then reached Ready with the real two thread CPU model. The existing deployed live
workflow passed review confirmation, completed report/artifact retrieval, and
invalid-input/privacy boundaries. Completed synthetic run:
`run_0b73d290c67d7147c2eb7c88`; report SHA256
`cab727c8911caafdf0e1c0b5399374739b8235ed498e2241db1fe35a54dc7525`.
During this bounded workflow the model used 1.48 GiB of its 1.5 GiB limit, API 7.906 MiB
of 768 MiB, PostgreSQL 41.32 MiB of 512 MiB. This proves the tested workload fits this
Engine allocation; it is not a fleet or throughput result.

Recovery passed API kill (Ready in 10.233s), model kill (Ready in 9.697s), and API removal
(Ready in 14.740s). Completed report hash remained identical in every case. Kill
retained container IDs; removal created a new owned API container. The Python
harness now normalizes Docker nanosecond timestamps for Intel Mac Python 3.8.

Repeated apply retained generation 1. Stop/start progressed through generations 2/3,
preserved all six resource IDs and the report hash. Delete converged at generation 4
with a terminal tombstone; repeated delete retained generation 4. Only the two
original owned data volumes remained. The final hardened native controller binary
then reopened this terminal state with the catalog file unavailable, preserving
accepted snapshot, generation, tombstone, and volume bindings. The test daemon was
stopped afterward. The existing installed controller LaunchAgent remained running
at PID 52388. The packaging loopback registry and retained test data remain outside
the controller lifecycle.

Structured results: [recovery](evidence/m7-intel-recovery.json),
[lifecycle](evidence/m7-intel-lifecycle.json), [reopen](evidence/m7-intel-reopen.json).
This is macOS with Docker Desktop's Linux VM; native Linux host operation remains M8.

## ARM64 real runtime acceptance

On 2026-10-07, local Docker Engine29.8.2 reported LinuxARM64,16CPUs and
8214056960bytes memory. The committed ARM64 system digest was
`sha256:bfbbb646bc494724b7a0f977ab7d4310e53e5423844ab5921c4412c733fbeb7c`.
The deployment supplied approved `modelThreads=4` and
`modelNanoCPUs=4000000000`; inspected runtime configuration used those values
without Go source edits. PostgreSQL and API retained their engineer-defined limits.
`plan` left events empty and no accepted deployment. `apply` acknowledged
generation 1 Pending; the controller then observed Ready.

The real local-model submission, review confirmation, completed report, input
rejection and privacy checks passed using the repository live smoke script.
Completed run: `run_2d9ec7cbe742585761175683`. Report SHA256:
`97082582755d0d82aba6944321a86d0c3ecb5da94ab2c9cc3d6788e9d89ea342`.

| Scoped fault | Detection | Running | Fresh Ready | Retained report |
| --- | --- | --- | --- | --- |
| API kill | 4.098s | 4.098s | 8.959s | Same checksum |
| Model kill | 5.026s | 5.026s | 9.853s | Same checksum |
| API remove | 5.200s | 9.981s | 14.968s | Same checksum, new API container ID |

Restarted the native test daemon with an unavailable catalog directory. Fresh
Ready converged using the persisted snapshot with the same six resource IDs and
generation 1. Reapplying the identical deployment succeeded without catalog access
and retained generation 1. Stop reached generation2; start reached generation3
Ready with the same report checksum. Delete reached generation 4/tombstone;
repeated delete kept generation 4. Direct engine inspection found no managed
containers/network and both original retained volumes, incarnations
`a814c0a31c0884ecefd337c8b04f0850` and `5310cea7b283368b1f3b95008afb2823`.
The isolated daemon was stopped after teardown. Lab records remain under
`/tmp/mealcheck-m7-arm64`; retained data was not purged. Shared build registry and
native MealCheck services remain independently operated.

Independent review caught an initial application probe using container port8080
on the host; the provider now resolves it to the declared published host port.
The corrected binary completed these runtime gates; the initial readiness
timeout was explicitly retried and is not counted as a successful gate.

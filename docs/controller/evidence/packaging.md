# M1 packaging evidence — 2026-10-07

Environment: M0's ARM64 Docker Desktop engine; all containers linux/arm64; no
emulation. Go build 1.26.4, PostgreSQL 17.6, llama.cpp b11459 revision
`f498f864fbc0472004ee1c3616c1188c68eb157f`.

Pinned image/model references: `images.env`; API tested reference:
`localhost:15000/mealcheck@sha256:3d7ba167fc373f348e98bed7a73e865d981f334f7e37ec07e4da525c8ffb795f`.
Local registry is published on loopback only and not a managed workload resource.

Commands: `package-lab.sh prepare`, `build`, `up`, `smoke`, `restart`, `down`.
The first workflow exposed missing runtime schemas; adding `schemas/` to the
image resolved review confirmation. Subsequent full real-model workflows passed
normalization, awaiting review, confirmation, deterministic verification, report
artifacts, provider-config rejection and oversized-input rejection. Synthetic
run IDs only; no external provider credentials were used.

Completed report `run_f4ac6704e8bd3019b329a0c0` was byte-identical after API,
model and PostgreSQL stop/start:
`517560deaab37232e69840e6555963b979c7ba60d21676f02740ec50bbd02f0e`.
API volume ownership was initialized by the image as UID/GID10001. PostgreSQL
and model had no host publications; API port was `127.0.0.1:18080`. Restart
policies were unset. Observed idle memory: API7.2MiB, model1.77GiB,
PostgreSQL25.8MiB (not a peak benchmark).

A synthetic run hard-killed during normalization remained running after restart,
blocking the serialized model queue. A separate application fix terminally fails
expired worker leases without replaying input. Dedicated PostgreSQL regression
proved active leases remain protected, expired work fails with a diagnostic,
next work proceeds, and an empty queue still commits recovery. Race tests passed
for PostgreSQL recovery, execution, and memory store. Runtime expiry was
accelerated only for the synthetic abandoned row, then the worker marked it
failed and a subsequent real workflow passed. Default timeout remains ten
minutes; failure is detected on the next claim poll after expiry.

Data volumes were retained. This verifies Linux containers on macOS, not native
Linux host operation or systemd startup. No native production service or shared
ingress was changed.

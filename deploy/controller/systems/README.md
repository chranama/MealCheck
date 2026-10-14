# Trusted system catalog

These are engineer-owned system definitions, separate from the deployment inputs
in `../examples/`. Copy an approved definition to a private account-owned catalog
(mode0700 directory, mode0600 JSON files) and supply `daemon --catalog`. The daemon
refuses symlinks, group/world writable entries, wrong ownership, mismatched names,
and mismatched content digests. A deployment cannot supply a catalog path.

`mealcheck-cpu-arm64--v1.json` uses the verified local ARM64 API build and native
PostgreSQL/llama.cpp pins. It is runnable only after those exact images are preloaded
and a real model and lab-only secret files are staged. The API reference belongs to
a loopback development registry; it is not a published image available elsewhere.
`mealcheck-cpu-amd64--v1.json` targets the four-CPU Intel lab with smaller explicit
limits. See [manifest evidence](../../../docs/controller/evidence/manifests.md) for image resolution and runtime validation.

Before using another API build, the infrastructure engineer must replace the API
image in the system definition and issue a new system version or reviewed content
digest. The deployment operator cannot change image, mount, readiness, or topology
policy. The example declares bounded model CPU/thread and loopback port parameters;
`modelPath` remains constrained by the daemon's independent allowed model roots.
The deployment's secret profile names private files; it never contains passwords.

Use `mealcheck-controller system-digest --file SYSTEM.json` for the canonical digest.
Whitespace is not part of the digest; typed field values and resolved definition are.
Once accepted, the deployment and resolved snapshot persist transactionally.
Editing/removing a catalog file does not modify an accepted workload. This milestone
preserves the immutable-workload boundary; a changed system or workload requires a
new installation until an explicit update mechanism is implemented.

Allowlists and allowed model roots govern new acceptance. Narrowing daemon flags
or deleting catalog files does not revoke the already accepted durable snapshot.
To withdraw an accepted workload's authority, stop/delete it through the controller
and verify convergence; catalog editing is not a revocation mechanism.

JSON Schemas in `../schemas/` describe the shape and disallow unknown fields. The Go
resolver also checks parameter types/bounds, graph references and cycles, mounts,
registry/model policy, image architecture, and actual Docker Engine capacity.

## Production v2

`mealcheck-production-amd64--v2.json` is the engineer-owned definition validated
on the Intel macOS web server on 2026-10-13. It pins PostgreSQL 17.10, llama.cpp,
and the corrected API image built from application revision `7402b33`. The API
packaging normalizes runtime asset permissions so UID 10001 can read catalogs,
fixtures, and schemas even when the build context was staged with umask 077.

The definition preserves production access/CORS, retention, queue, and timeout
settings. The model uses context 4096, two CPU threads, batch 256/microbatch 64,
parallel 1, and a 2 GiB memory limit. PostgreSQL 512 MiB and API 768 MiB bring the
total memory limits to 3328 MiB, within the inspected four-CPU Docker Engine.
The API binds only to `127.0.0.1:18080`.

Canonical system digest:
`sha256:628d781cc368fb7b504b653b1631b716195e53b1e9f90146ce70f0cbac85d183`.
The API image belongs to the web server's loopback registry and is not publicly
downloadable. This deployment used a staged, cross-compiled API binary and a
runtime-only Docker build; rebuilding the repository Dockerfile may produce a
different digest. Preload these exact images or issue a reviewed system version
with replacement pins. Private `mealcheck-production` credentials and the
approved GGUF remain outside this repository. Configure the daemon's catalog,
image allowlists, secret root, and model roots before planning a deployment.

Real-model workflow, API/model container recovery, API recreation, controller
KeepAlive restart, and report persistence passed in a fresh private installation.
Its state directory is `controller-state-v2` under the server's controller root,
with LaunchAgent label `dev.mealcheck.controller.production-v2`. A fresh installation
was required because accepted workloads are immutable. The retired v1 installation,
including its containers, network, volumes, state, LaunchAgent, and server manifests,
was removed after ownership checks; backup and diagnostic evidence were preserved.
Historical native data and public ingress have not been migrated to v2.

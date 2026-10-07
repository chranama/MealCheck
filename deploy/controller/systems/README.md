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
limits. See `../manifest-evidence.md` for image resolution and runtime validation.

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

# Controller deployment assets

Start with the [controller documentation](../../docs/controller/README.md).
These files support an isolated controller workload; they do not change the
native MealCheck service or shared Cloudflare ingress.

| Directory/file | Purpose |
| --- | --- |
| `packaging/` | API image, entrypoint and historical lab packaging script |
| `images.env` | Packaging image references and model checksum |
| `macos/` | Dedicated lab LaunchAgent template and install/uninstall scripts |
| `verification/` | Explicit lab recovery and engine-connectivity experiments |
| `tools/render-spec.py` | Render a deployment referencing a trusted system |
| `systems/` | Engineer-authored trusted system definitions |
| `examples/` | Deployment-user JSON templates |
| `schemas/` | Versioned system/deployment JSON schemas |

Run scripts from the repository root, following the [Mac operating guide](../../docs/controller/operations/macos.md).
The [workload contract](../../docs/controller/workload.md) describes packaging
prerequisites and data boundaries. Verification scripts can mutate isolated lab
resources; follow their explicit preconditions.

Recorded results live in [controller evidence](../../docs/controller/evidence/README.md),
with raw artifacts under `docs/controller/evidence/artifacts`. Models, secrets,
SQLite controller state and retained Docker volumes live outside this repository.

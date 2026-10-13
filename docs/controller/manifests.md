# System and deployment manifests

A trusted **system manifest** defines services, dependency order, image pins,
resource limits, storage, networking, readiness and allowed parameters. An
operator's **deployment manifest** selects a system identity/version/digest and
supplies a deployment identity, desired state and constrained parameters.

The controller validates and persists the accepted resolved snapshot. Subsequent
catalog edits do not change an accepted deployment. Workload updates and data
migration require a separate design; changed accepted configuration is rejected.

- [Contract and compatibility](specification.md#m7-manifest-separation)
- [Trusted system definitions](../../deploy/controller/systems/README.md)
- [Deployment templates](../../deploy/controller/examples/README.md)
- [JSON schemas](../../deploy/controller/schemas/)
- [CLI](cli.md): `system-digest`, read-only `plan`, durable `apply`, observed `get`
- [Mac walkthrough](operations/macos.md)
- [Recorded manifest verification](evidence/manifests.md)

`apply` acknowledges accepted desired state, not application readiness. Use `get`
and the documented real-model workflow to assess observed readiness and behavior.

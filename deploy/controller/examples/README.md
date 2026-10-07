# Deployment inputs

`deployment-arm64.template.json` and `deployment-amd64.template.json` are templates,
not commands to execute without preparation. Their system references/digests match
the committed catalog examples, while `modelPath` must be replaced with a staged
real GGUF beneath the daemon's allowed root. The private `mealcheck-lab` secret
profile must exist under the configured secret root. No secret contents are stored.

Use `render-spec.py --system-file APPROVED_SYSTEM --controller-binary BINARY
--lab-root LAB_ROOT --output NEW_FILE` to render a checksum-verified instance. The
renderer does not edit the trusted system or choose images. Review using
`mealcheck-controller plan --state-dir STATE --file NEW_FILE`, then apply the same
file. `plan` validates and previews without accepting desired state or mutating
Docker. Successful apply means durable acceptance, not application readiness; use
`get` to inspect readiness and `events` for recovery history.

Accepted files are not watched. Subsequent lifecycle transitions use the CLI or an
otherwise identical manifest. Deletion is terminal and retains data volumes.

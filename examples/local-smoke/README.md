# Local Smoke Fixtures

These fixtures support Milestone 7 local full-stack validation.

- `manual-run-request.json` is an invite-gated manual structured run payload.
- `profile-generation-request.template.json` is a BYOK generation payload
  template. Replace the placeholder API key only in local commands.
- The fake provider response for local deterministic BYOK smoke tests is the
  checked-in seeded candidate plan:
  `examples/seeded-one-day-peanut-allergy/plans/candidate.json`.

Do not commit real provider keys in this directory.

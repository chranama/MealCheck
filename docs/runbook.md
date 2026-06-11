# Runbook

This runbook describes the development and hosted deployment shape. It becomes
fully accepted for the MVP when the MacBook deployment has exact service,
tunnel, log, and smoke-test commands recorded.

## Local Development Target

Initial development should work on a normal laptop with:

- local fixture runs
- generated artifact bundles
- deterministic tests
- no required model API key for seeded examples
- no required network access for the first seeded proof

Live model calls should be optional.

## Fixture Validation

Milestone 0 fixtures should validate locally with:

```bash
go run ./cmd/mealcheck-fixture-check
```

This command validates JSON fixtures against the checked-in JSON Schemas and
performs cross-file checks that schemas cannot express, such as case paths,
guideline pack IDs, nutrient catalog IDs, source references, and source claim
references.

## Checker Tests

The seeded checker core should pass:

```bash
go test ./...
```

The current tests verify the seeded `block` decision, unresolved quantity
visibility, sodium warning evidence, computed nutrient totals, and rejection of
LLM-supplied nutrition totals.

## Local CLI Artifact Run

The Milestone 2 CLI writes a full local artifact bundle for the seeded proof
case:

```bash
go run ./cmd/mealcheck validate \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out artifacts/latest
```

The seeded candidate is expected to fail with a `block` decision, so this command
exits `1` after writing artifacts. That is the correct policy behavior.

The bundle includes:

- `decision.json`
- `report.json`
- `report.html`
- `report.md`
- `failures.jsonl`
- `daily-totals.json`
- `resolved-foods.json`
- `unresolved-foods.json`
- `metrics.json`
- `manifest.json`
- `normalized-plan.json`
- redacted run config
- guideline-pack snapshot
- copied JSON Schemas

Read an existing decision and apply the same exit-code policy with:

```bash
go run ./cmd/mealcheck decision artifacts/latest/decision.json
```

Use `compare` when exercising the baseline/candidate CLI surface:

```bash
go run ./cmd/mealcheck compare \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out artifacts/latest-compare
```

For Milestone 2, `compare` uses the same seeded evaluation path and records
`compare` in `manifest.json`. Baseline-specific regression expansion remains a
future checker enhancement.

## MacBook Air Server Target

Hardware target:

- MacBook Air Retina 13-inch, 2019
- 1.6 GHz dual-core Intel Core i5
- 8 GB RAM
- macOS Sonoma 14.8.7

Operational settings:

- keep the MacBook plugged into power
- use wired Ethernet through USB-C if possible
- disable sleep while plugged in
- enable automatic restart after power failure if available
- keep macOS security updates current

## Runtime Shape

Initial hosted runtime:

- Cloudflare Pages static frontend
- API service
- one worker process
- Postgres for run metadata
- filesystem artifact storage
- checked-in guideline packs and fixture nutrient catalog
- cleanup job for expired runs
- Cloudflare Tunnel for API exposure

Current backend command:

```bash
go run ./cmd/mealcheck-server
```

Production-style Postgres mode requires:

```bash
export DATABASE_URL='postgres://mealcheck:mealcheck@localhost:5432/mealcheck?sslmode=disable'
```

Local development without Postgres:

```bash
go run ./cmd/mealcheck-server -store memory
```

Smoke test:

```bash
curl http://127.0.0.1:8080/api/health
curl http://127.0.0.1:8080/api/demo-runs
curl -X POST http://127.0.0.1:8080/api/runs \
  -H 'Content-Type: application/json' \
  -d '{"case_path":"examples/seeded-3-day-peanut-allergy/case.json"}'
```

The queued seeded run is expected to complete with a `block` decision because
the fixture intentionally contains blocking findings.

Invite-gated BYOK generation uses the same run endpoint. Use placeholders below
and do not commit real provider keys:

```bash
curl -X POST http://127.0.0.1:8080/api/runs \
  -H 'Content-Type: application/json' \
  -H "X-MealCheck-Invite-Token: $MEALCHECK_INVITE_TOKEN" \
  -d '{
    "input_mode": "profile_generation",
    "profile": {
      "age": 35,
      "sex": "male",
      "height_cm": 178,
      "weight_kg": 82,
      "activity_level": "moderate"
    },
    "constraints": {
      "days": 3,
      "meals_per_day": 3
    },
    "provider": {
      "type": "openai_compatible",
      "model": "gpt-example",
      "api_key": "replace-with-user-key"
    },
    "repair_json": true
  }'
```

Avoid initially:

- Kubernetes
- local LLM inference
- anonymous live inference
- Redis unless the queue needs it
- direct router port forwarding
- arbitrary user code execution

## Frontend Hosting

The first production frontend deploys the Vite/React app in `ui/` to
Cloudflare Pages as static files.

Suggested Cloudflare Pages settings:

- root directory: `ui`
- build command: `npm ci && npm run build`
- build output directory: `dist`

The frontend should use only public build-time configuration, such as the
backend API base URL.

The MacBook should not serve the production frontend. It should remain focused
on backend API, worker, database, artifacts, source packs, and cleanup.

Local development preview:

```bash
cd ui
npm install
npm run dev
```

Then open `http://localhost:4173`.

## Milestone 7 Local Acceptance

Run the local smoke command from the repository root:

```bash
go run ./cmd/mealcheck-local-smoke
```

This command:

- builds `mealcheck` into a temporary clean build directory
- runs the seeded CLI validation and verifies the expected `block` exit policy
- inspects the generated `decision.json`
- starts an in-memory hosted API harness
- verifies invite-token gating
- verifies allowed and disallowed CORS behavior
- creates and processes one manual structured run
- creates and processes one BYOK run with a fake provider response
- checks run events, reports, artifact listing, and deletion
- verifies the fake provider key is absent from runtime files, reports,
  artifacts, and smoke-test logs

Run the real local browser/full-stack smoke suite:

```bash
cd ui
npm run test:e2e:local
```

This starts the real Go backend on `127.0.0.1:8081` with memory storage and the
Vite frontend on `127.0.0.1:4173`. The backend uses:

```bash
MEALCHECK_STORE=memory
MEALCHECK_INVITE_TOKEN=invite-1
MEALCHECK_ALLOWED_ORIGIN=http://127.0.0.1:4173
MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH=../examples/seeded-3-day-peanut-allergy/plans/candidate.json
```

`MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH` is for local smoke testing only. Do not
set it in the deployed MacBook service.

The first public manual-entry scope is intentionally limited to the existing
17-food fixture catalog used by the seeded proof. This keeps the first public
live path honest about its narrow food resolver. Broader manually entered meals
should wait for a reviewed catalog expansion or FoodData Central strategy.

## Public Access Policy

Public visitors should be able to:

- inspect seeded reports
- download safe artifacts
- understand which guideline pack was used
- see unresolved foods and check evidence

Public visitors should not be able to:

- trigger maintainer-paid model calls
- upload unbounded meal plans
- access private live-run artifacts
- view user-provided API keys or unredacted configs
- receive medical advice from the service

## Hosted Resource Defaults

Initial defaults:

- one active live run
- queue size of 3
- max 20 cases per run
- max 10 minutes per run
- 7-day artifact retention
- explicit upload and output-size limits

These defaults should be enforced in code, not only documented.

## Web MVP Operations Required

The MVP is not accepted until MealCheck is running as a long-standing web
deployment, not just as local code.

Required deployment records:

- Cloudflare Pages project name, production URL, branch, root directory, build
  command, output directory, and public frontend configuration.
- Cloudflare Tunnel name, tunnel ID, credentials location, public API hostname,
  and hostname route to the local API address.
- MacBook runtime user, repository path, runtime data path, artifact path,
  Postgres database name, and log path.
- Environment variables used by the backend service, including
  `DATABASE_URL`, `MEALCHECK_ALLOWED_ORIGIN`, `MEALCHECK_INVITE_TOKEN`,
  `MEALCHECK_DATA_DIR`, and `MEALCHECK_ARTIFACT_DIR`.
- Process supervision setup, expected restart behavior, and commands to start,
  stop, restart, and inspect the backend and tunnel.
- Backup scope for Postgres metadata and retained artifacts.

Required operational commands:

- deploy or pull the latest repo revision on the MacBook
- start, stop, and restart the backend service
- start, stop, and restart the Cloudflare Tunnel
- check Postgres health
- check backend health locally
- check backend health through the public API hostname
- inspect frontend deployment status
- inspect backend and tunnel logs
- trigger cleanup or verify retention behavior
- delete a live run and confirm artifact removal

Required web smoke tests:

- open the production frontend URL from outside the home network
- inspect the seeded report without logging in or using a provider key
- verify the frontend shows backend health when the API is online
- create one invite-gated live run through the web UI or documented API command
- observe run events through completion or failure
- fetch the report and artifact list for the live run
- verify persisted artifacts contain `redacted` provider config only
- delete the live run and verify the report/artifacts are no longer available

Operational topics still to document with exact local commands after the
MacBook is configured:

- source-pack update process
- nutrient catalog update process
- common failure modes and recovery steps

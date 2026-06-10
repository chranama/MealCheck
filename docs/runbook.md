# Runbook

This runbook describes the intended development and hosted deployment shape. It
will become command-specific as implementation lands.

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

Avoid initially:

- Kubernetes
- local LLM inference
- anonymous live inference
- Redis unless the queue needs it
- direct router port forwarding
- arbitrary user code execution

## Frontend Hosting

The first production frontend can deploy `ui/` to Cloudflare Pages from Git with
no build command.

Suggested Cloudflare Pages settings:

- root directory: `ui`
- build command: none
- build output directory: `/`

The frontend should use only public build-time configuration, such as the
backend API base URL.

The MacBook should not serve the production frontend. It should remain focused
on backend API, worker, database, artifacts, source packs, and cleanup.

Local static preview:

```bash
python3 -m http.server 4173 --directory ui
```

Then open `http://localhost:4173`.

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

## Operations To Add After Hosted Implementation

Once hosted code exists, this file should include:

- Cloudflare Pages project setup
- frontend build command and output directory
- frontend environment variables
- Cloudflare Tunnel hostname routing
- start commands
- stop commands
- health checks
- tunnel setup
- backup and cleanup commands
- log locations
- common failure modes
- smoke test commands
- source-pack update process
- nutrient catalog update process

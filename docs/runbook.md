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

## Quick Command Reference

Run commands from the repository root unless a command explicitly changes into
`ui/`.

Validate fixtures:

```bash
go run ./cmd/mealcheck-fixture-check
```

Run the Go test suite:

```bash
go test ./...
```

Run the seeded checker and artifact bundle:

```bash
go run ./cmd/mealcheck validate \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out artifacts/latest
```

The seeded candidate intentionally returns a `block` decision, so the validation
command exits `1` after writing the bundle. Inspect the decision:

```bash
go run ./cmd/mealcheck decision artifacts/latest/decision.json
```

Run the local full-stack/security smoke command:

```bash
go run ./cmd/mealcheck-local-smoke
```

Build local deployment binaries:

```bash
mkdir -p bin
go build -o bin/mealcheck ./cmd/mealcheck
go build -o bin/mealcheck-server ./cmd/mealcheck-server
```

Run the hosted API locally with in-memory storage:

```bash
go run ./cmd/mealcheck-server -store memory
```

Production-style metadata storage uses Postgres through `DATABASE_URL`.

Preview the frontend:

```bash
cd ui
npm install
npm run dev
```

Then open `http://localhost:4173`.

Verify the frontend:

```bash
cd ui
npm run typecheck
npm test
npm run test:e2e
npm run test:e2e:local
npm run build
```

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
export DATABASE_URL='postgres://mealcheck:<POSTGRES_PASSWORD>@localhost:5432/mealcheck?sslmode=disable'
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

Invite-gated BYOK generation uses the same run endpoint. Create a per-user
access code before sharing the live-check path:

```bash
/Users/chranama-server/MealCheck/bin/mealcheck invite create \
  --label "reviewer-name" \
  --expires 2026-07-31 \
  --max-runs 20
```

The command prints the full access code once. MealCheck stores only its secret
hash and usage metadata.

List access codes:

```bash
/Users/chranama-server/MealCheck/bin/mealcheck invite list
```

Revoke one access code by its public ID:

```bash
/Users/chranama-server/MealCheck/bin/mealcheck invite revoke <INVITE_ID>
```

Use the access code in the header below. Do not commit real provider keys or
access codes:

```bash
curl -X POST http://127.0.0.1:8080/api/runs \
  -H 'Content-Type: application/json' \
  -H "X-MealCheck-Invite-Token: $MEALCHECK_ACCESS_CODE" \
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

## Milestone 8 Deployment Package

The local deployment package lives in `deploy/`.

Source-build deployment is enough for the MVP. MealCheck is targeting one known
MacBook Air, so the first deployment should build binaries from the checked-out
repository instead of producing separate release binaries.

Selected deployment values:

| Value | Setting |
|---|---|
| Runtime user | `chranama-server` |
| Repository path | `/Users/chranama-server/MealCheck` |
| Data path | `/Users/chranama-server/MealCheck-data` |
| Artifact path | `/Users/chranama-server/MealCheck-data/artifacts` |
| Log path | `/Users/chranama-server/MealCheck-data/logs` |
| Backend binary | `/Users/chranama-server/MealCheck/bin/mealcheck-server` |
| CLI binary | `/Users/chranama-server/MealCheck/bin/mealcheck` |
| Environment file | `/Users/chranama-server/MealCheck-data/mealcheck-server.env` |
| Postgres database | `mealcheck` |
| Postgres role | `mealcheck` |
| Backend listen address | `127.0.0.1:8080` |
| Backend launchd label | `dev.mealcheck.server` |
| Postgres launchd label | `dev.mealcheck.postgres` |
| Cloudflare Tunnel name | `mealcheck-api` |
| Frontend production URL | `https://mealcheck.dev` |
| API production URL | `https://api.mealcheck.dev` |

Templates:

- `deploy/macos/mealcheck-server.env.example`
- `deploy/macos/dev.mealcheck.server.plist.template`
- `deploy/macos/postgres-setup.sql.template`
- `deploy/cloudflare/tunnel-config.yml.template`
- `deploy/cloudflare/pages-settings.md`
- `deploy/cloudflare/config.json.template`

## Local CLI Deployment

Build the CLI binary from a clean checkout or clean build directory:

```bash
cd /Users/chranama-server/MealCheck
mkdir -p bin
go build -o bin/mealcheck ./cmd/mealcheck
```

Verify the installed binary:

```bash
/Users/chranama-server/MealCheck/bin/mealcheck help
```

Run the seeded validation:

```bash
/Users/chranama-server/MealCheck/bin/mealcheck validate \
  --root /Users/chranama-server/MealCheck \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out /Users/chranama-server/MealCheck-data/artifacts/cli-smoke
```

The seeded candidate intentionally returns `block`, so the command exits `1`
after writing artifacts. Inspect the decision:

```bash
/Users/chranama-server/MealCheck/bin/mealcheck decision \
  /Users/chranama-server/MealCheck-data/artifacts/cli-smoke/decision.json
```

## MacBook First-Time Preparation

Install required tools:

```bash
brew install go postgresql@17 jq cloudflared
```

Install the project Postgres `LaunchDaemon` instead of using
`sudo brew services start postgresql@17`. Homebrew's generated system daemon
tries to run Postgres as root on this machine, but Postgres must run as an
unprivileged user.

```bash
brew services stop postgresql@17 2>/dev/null || true
sudo launchctl bootout system/homebrew.mxcl.postgresql@17 2>/dev/null || true
sudo rm -f /Library/LaunchDaemons/homebrew.mxcl.postgresql@17.plist

sudo cp deploy/macos/dev.mealcheck.postgres.plist.template \
  /Library/LaunchDaemons/dev.mealcheck.postgres.plist
sudo chown root:wheel /Library/LaunchDaemons/dev.mealcheck.postgres.plist
sudo chmod 644 /Library/LaunchDaemons/dev.mealcheck.postgres.plist
sudo plutil -lint /Library/LaunchDaemons/dev.mealcheck.postgres.plist
sudo launchctl bootstrap system /Library/LaunchDaemons/dev.mealcheck.postgres.plist
sudo launchctl kickstart -k system/dev.mealcheck.postgres
```

Verify Postgres:

```bash
pg_isready -h localhost -p 5432
```

Configure the MacBook for long-running backend use while plugged into power:

```bash
sudo pmset -c sleep 0 disksleep 0 displaysleep 10 powernap 0 standby 0 \
  ttyskeepawake 1 tcpkeepalive 1 womp 1 autorestart 1
```

Verify the AC-power profile:

```bash
pmset -g custom
pmset -g assertions
```

`pmset -g custom` should show `sleep 0` under `AC Power`. Keep the MacBook
plugged in and leave the lid open unless it is intentionally running in a
supported clamshell setup. Closing a laptop lid can still sleep the machine.

Create runtime directories:

```bash
mkdir -p /Users/chranama-server/MealCheck-data/artifacts
mkdir -p /Users/chranama-server/MealCheck-data/logs
```

Create the Postgres role and database from the template:

```bash
cp deploy/macos/postgres-setup.sql.template /tmp/mealcheck-postgres-setup.sql
```

Edit `/tmp/mealcheck-postgres-setup.sql`, replace `<POSTGRES_PASSWORD>`, then
run:

```bash
psql postgres -f /tmp/mealcheck-postgres-setup.sql
```

Verify Postgres access:

```bash
psql 'postgres://mealcheck:<POSTGRES_PASSWORD>@localhost:5432/mealcheck?sslmode=disable' \
  -c 'select 1 as mealcheck_db_ok;'
```

Create the production environment file:

```bash
cp deploy/macos/mealcheck-server.env.example \
  /Users/chranama-server/MealCheck-data/mealcheck-server.env
chmod 600 /Users/chranama-server/MealCheck-data/mealcheck-server.env
```

Edit `/Users/chranama-server/MealCheck-data/mealcheck-server.env` and replace:

- `<POSTGRES_PASSWORD>`

Do not set `MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH` in the deployed service.

## Backend Deploy Or Pull

First checkout:

```bash
git clone git@github.com:chranama/MealCheck.git /Users/chranama-server/MealCheck
```

Update an existing checkout:

```bash
cd /Users/chranama-server/MealCheck
git pull --ff-only origin main
```

Build backend binaries:

```bash
cd /Users/chranama-server/MealCheck
mkdir -p bin
go build -o bin/mealcheck ./cmd/mealcheck
go build -o bin/mealcheck-server ./cmd/mealcheck-server
```

Run the production-style backend manually before installing `launchd`:

```bash
set -a
source /Users/chranama-server/MealCheck-data/mealcheck-server.env
set +a
/Users/chranama-server/MealCheck/bin/mealcheck-server \
  -root /Users/chranama-server/MealCheck \
  -addr 127.0.0.1:8080 \
  -data-dir /Users/chranama-server/MealCheck-data \
  -artifact-dir /Users/chranama-server/MealCheck-data/artifacts \
  -store postgres
```

## Backend launchd Service

Milestone 10 uses a system `LaunchDaemon` so the backend can start before a
GUI login after reboot while still running as `chranama-server`. The daemon
waits for local Postgres to accept connections before starting
`mealcheck-server`.

Install the template:

```bash
sudo cp deploy/macos/dev.mealcheck.server.plist.template \
  /Library/LaunchDaemons/dev.mealcheck.server.plist
sudo chown root:wheel /Library/LaunchDaemons/dev.mealcheck.server.plist
sudo chmod 644 /Library/LaunchDaemons/dev.mealcheck.server.plist
sudo plutil -lint /Library/LaunchDaemons/dev.mealcheck.server.plist
```

Start:

```bash
sudo launchctl bootstrap system /Library/LaunchDaemons/dev.mealcheck.server.plist
```

Stop:

```bash
sudo launchctl bootout system/dev.mealcheck.server
```

Restart:

```bash
sudo launchctl kickstart -k system/dev.mealcheck.server
```

Status:

```bash
sudo launchctl print system/dev.mealcheck.server
```

Logs:

```bash
tail -n 200 /Users/chranama-server/MealCheck-data/logs/mealcheck-server.out.log
tail -n 200 /Users/chranama-server/MealCheck-data/logs/mealcheck-server.err.log
```

Local health:

```bash
deploy/macos/wait-for-mealcheck-ready.sh
curl -fsS http://127.0.0.1:8080/api/health | jq .
```

## Cloudflare Pages And Tunnel Draft

Pages settings are in `deploy/cloudflare/pages-settings.md`.

Live Pages values:

- project name: `mealcheck`
- production branch: `main`
- root directory: `ui`
- build command: `npm ci && npm run build`
- output directory: `dist`
- public frontend config:
  `VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.dev`
- Pages URL: `https://mealcheck.pages.dev`
- custom domain: `https://mealcheck.dev`, active

The current Pages project was created by direct Wrangler upload. Cloudflare
reported `Git Provider: No` after creation. If push-to-deploy is required,
connect the existing Pages project to the GitHub repository in the Cloudflare
dashboard.

Build and deploy direct-upload Pages:

```bash
cd /Users/chranama-server/MealCheck/ui
npm ci
VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.dev npm run build

cd /Users/chranama-server/MealCheck
wrangler pages deploy ui/dist \
  --project-name mealcheck \
  --branch main \
  --commit-hash "$(git rev-parse HEAD)" \
  --commit-message "$(git log -1 --pretty=%s)" \
  --commit-dirty=true
```

Pages deployment status:

```bash
wrangler pages deployment list --project-name mealcheck
```

Attach and inspect the Pages custom domain through the API, using Wrangler's
local OAuth config without printing the token:

```bash
TOKEN=$(awk -F' = ' '/^oauth_token/ { gsub(/"/, "", $2); print $2 }' \
  /Users/chranama-server/Library/Preferences/.wrangler/config/default.toml)

curl -fsS -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"name":"mealcheck.dev"}' \
  "https://api.cloudflare.com/client/v4/accounts/0f5ac9230ddfc332774b414898e9f59f/pages/projects/mealcheck/domains" \
  | jq .

curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  "https://api.cloudflare.com/client/v4/accounts/0f5ac9230ddfc332774b414898e9f59f/pages/projects/mealcheck/domains/mealcheck.dev" \
  | jq .
```

Cloudflare DNS record for `https://mealcheck.dev`:

| Type | Name | Target | Proxy |
|---|---|---|---|
| `CNAME` | `@` | `mealcheck.pages.dev` | Proxied |

Tunnel config template:

```bash
deploy/cloudflare/tunnel-config.yml.template
```

After creating the tunnel, copy the config to the MacBook-local cloudflared
config path and replace:

- `<CLOUDFLARE_TUNNEL_ID>`
- `<ABSOLUTE_CLOUDFLARE_CREDENTIALS_JSON>`
- `api.mealcheck.dev`

Manual tunnel start:

```bash
cloudflared tunnel --config /Users/chranama-server/.cloudflared/mealcheck-api.yml run mealcheck-api
```

Install the tunnel as a system `LaunchDaemon`:

```bash
cd /Users/chranama-server/MealCheck
sudo launchctl bootout system/dev.mealcheck.tunnel 2>/dev/null || true
sudo rm -f /Library/LaunchDaemons/dev.mealcheck.tunnel.plist
sudo cp deploy/macos/dev.mealcheck.tunnel.plist.template \
  /Library/LaunchDaemons/dev.mealcheck.tunnel.plist
sudo chown root:wheel /Library/LaunchDaemons/dev.mealcheck.tunnel.plist
sudo chmod 644 /Library/LaunchDaemons/dev.mealcheck.tunnel.plist
sudo plutil -lint /Library/LaunchDaemons/dev.mealcheck.tunnel.plist
sudo launchctl bootstrap system /Library/LaunchDaemons/dev.mealcheck.tunnel.plist
sudo launchctl kickstart -k system/dev.mealcheck.tunnel
sudo launchctl print system/dev.mealcheck.tunnel
```

Tunnel status:

```bash
cloudflared tunnel info mealcheck-api
cloudflared tunnel list
sudo launchctl print system/dev.mealcheck.tunnel
```

Public API health:

```bash
curl -fsS https://api.mealcheck.dev/api/health | jq .
```

## Cleanup, Deletion, And Retention

Live runs default to 7-day retention. Cleanup runs inside `mealcheck-server` on
`MEALCHECK_CLEANUP_INTERVAL`.

Delete a live run:

```bash
curl -fsS -X DELETE https://api.mealcheck.dev/api/runs/<RUN_ID> | jq .
```

Confirm deletion:

```bash
curl -i https://api.mealcheck.dev/api/runs/<RUN_ID>
curl -i https://api.mealcheck.dev/api/runs/<RUN_ID>/report
```

## Backup Policy Draft

Back up Postgres metadata and retained artifacts. The MVP backup target is a
local timestamped directory; move or sync that directory off-machine after the
first real deployment policy is chosen.

Create a backup directory:

```bash
export MEALCHECK_BACKUP_DIR="/Users/chranama-server/MealCheck-data/backups/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$MEALCHECK_BACKUP_DIR"
```

Dump Postgres:

```bash
pg_dump 'postgres://mealcheck:<POSTGRES_PASSWORD>@localhost:5432/mealcheck?sslmode=disable' \
  > "$MEALCHECK_BACKUP_DIR/mealcheck.sql"
```

Copy retained artifacts:

```bash
rsync -a /Users/chranama-server/MealCheck-data/artifacts/ \
  "$MEALCHECK_BACKUP_DIR/artifacts/"
```

Retention note: live artifacts are intentionally short-lived. Backups should
not become indefinite retention for user profile data unless that policy is
explicitly accepted later.

## Public Smoke-Test Checklist

Use the accepted production URLs once Milestone 11 Cloudflare routing is in
place.

- Open `https://mealcheck.dev` from outside the home network.
- Confirm the seeded report loads without login, provider keys, or backend
  access.
- Confirm the frontend shows backend health when
  `https://api.mealcheck.dev/api/health` is online.
- Create an invite-gated live manual run through the UI or API.
- Observe status/events until `completed` or `failed`.
- Fetch `GET /api/runs/<RUN_ID>/report`.
- Fetch `GET /api/runs/<RUN_ID>/artifacts`.
- Verify `configs/redacted-provider.json` contains `api_key: "redacted"` for
  BYOK runs.
- Verify provider keys do not appear in service logs or artifact files.
- Delete the run and confirm report/artifact URLs no longer work.
- Verify a disallowed browser origin does not receive
  `Access-Control-Allow-Origin`.

## Failure Modes And Recovery Draft

Backend down:

- check `sudo launchctl print system/dev.mealcheck.server`
- inspect `MealCheck-data/logs/mealcheck-server.err.log`
- run local health against `127.0.0.1:8080`
- restart with `sudo launchctl kickstart -k system/dev.mealcheck.server`

Postgres down:

- check `sudo launchctl print system/dev.mealcheck.postgres`
- restart with `sudo launchctl kickstart -k system/dev.mealcheck.postgres`
- verify with the `psql ... select 1` command above
- restart the backend after Postgres recovers

Tunnel down:

- run `cloudflared tunnel info mealcheck-api`
- verify local backend health first
- restart the tunnel process or service
- verify public API health

Bad frontend API config:

- inspect the built frontend config source:
  `VITE_MEALCHECK_API_BASE_URL` or `/config.json`
- verify the API hostname independently with `curl`
- verify backend `MEALCHECK_ALLOWED_ORIGIN` matches the production Pages origin
- restart the backend daemon after changing
  `/Users/chranama-server/MealCheck-data/mealcheck-server.env`:
  `sudo launchctl kickstart -k system/dev.mealcheck.server`

Queue full:

- inspect `/api/health` for queued and running counts
- wait for the active run to finish
- restart the backend only if a run appears stuck beyond `MEALCHECK_RUN_TIMEOUT`

Provider failure:

- check the run `error` field
- verify the user-supplied provider base URL, model, and key
- do not log or ask users to send raw API keys

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
- verifies access-code gating
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

The local browser suite uses the legacy shared-token compatibility path. The
production deployment should set `MEALCHECK_INVITE_REQUIRED=true` and create
per-user access codes with `mealcheck invite create`.

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
  `DATABASE_URL`, `MEALCHECK_ALLOWED_ORIGIN`, `MEALCHECK_INVITE_REQUIRED`,
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

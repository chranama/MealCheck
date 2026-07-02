# Deployment Package

This directory contains the deployment package prepared for the MacBook Air
backend, Cloudflare-hosted frontend, and local-model replay profile. The files
use placeholder secret values only; public production hostnames are
intentionally committed.

## Selected MVP Paths

| Value | Proposed setting |
|---|---|
| Runtime user | `chranama-server` |
| Repository path | `/Users/chranama-server/MealCheck` |
| Data path | `/Users/chranama-server/MealCheck-data` |
| Artifact path | `/Users/chranama-server/MealCheck-data/artifacts` |
| Log path | `/Users/chranama-server/MealCheck-data/logs` |
| Backend binary | `/Users/chranama-server/MealCheck/bin/mealcheck-server` |
| CLI binary | `/Users/chranama-server/MealCheck/bin/mealcheck` |
| Environment file | `/Users/chranama-server/MealCheck-data/mealcheck-server.env` |
| Local llama environment file | `/Users/chranama-server/MealCheck-data/mealcheck-llama.env` |
| Local llama listen address | `127.0.0.1:11435` |
| Local llama model | `/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf` |
| Postgres database | `mealcheck` |
| Postgres role | `mealcheck` |
| Backend listen address | `127.0.0.1:8080` |
| Frontend production URL | `https://mealcheck.dev` |
| API production URL | `https://api.mealcheck.dev` |
| Backend launchd label | `dev.mealcheck.server` |
| Local llama launchd label | `dev.mealcheck.llama` |
| Postgres launchd label | `dev.mealcheck.postgres` |
| Tunnel launchd label | `dev.mealcheck.tunnel` |
| Autodeploy launchd label | `dev.mealcheck.autodeploy` |
| Cloudflare Tunnel name | `mealcheck-api` |
| Cloudflare Tunnel ID | `e8cbd8da-735a-4053-9503-880f636670f6` |

## Files

- `macos/mealcheck-server.env.example`: production environment template.
- `macos/mealcheck-llama.env.example`: local llama.cpp model service
  environment template.
- `macos/dev.mealcheck.server.plist.template`: `launchd` template for
  the backend API, worker, and cleanup process as a system `LaunchDaemon`
  running as `chranama-server`.
- `macos/dev.mealcheck.llama.plist.template`: system `LaunchDaemon`
  template for the local `llama-server`, bound to `127.0.0.1:11435`.
- `macos/mealcheck-llama-server.sh`: wrapper that loads the llama service
  environment and starts `llama-server` with measured CPU-only defaults.
- `macos/install-mealcheck-llama-service.sh`: helper for installing,
  restarting, stopping, and inspecting the llama LaunchDaemon.
- `macos/dev.mealcheck.postgres.plist.template`: system `LaunchDaemon`
  template for Postgres `17`, started at boot but running as
  `chranama-server`.
- `macos/dev.mealcheck.tunnel.plist.template`: system `LaunchDaemon`
  template for `cloudflared`, using the MacBook-local tunnel config at
  `/Users/chranama-server/.cloudflared/mealcheck-api.yml`.
- `macos/dev.mealcheck.autodeploy.plist.template`: system `LaunchDaemon`
  template for polling GitHub and applying fast-forward backend updates.
- `macos/mealcheck-autodeploy.sh`: root-run autodeploy script that executes
  Git and Go commands as `chranama-server`, then restarts the backend service
  only when backend code changed.
- `macos/wait-for-mealcheck-ready.sh`: reboot-friendly readiness check that
  waits for Postgres and then the MealCheck health endpoint.
- `macos/postgres-setup.sql.template`: first-time Postgres database and role
  template.
- `cloudflare/tunnel-config.yml.template`: Cloudflare Tunnel ingress template.
- `cloudflare/pages-settings.md`: Cloudflare Pages project settings.
- `cloudflare/config.json.template`: optional runtime frontend config served as
  `/config.json`.
- `local-model/README.md`: reproducible local-model deployment profile for the
  API, Postgres, filesystem artifacts, and a loopback llama.cpp endpoint.
- `local-model/compose.postgres.dev.yml`: optional disposable Postgres fallback
  for developer laptops on host port `5433`.
- `local-model/mealcheck-server.env.example`: local-model API environment
  template.
- `local-model/mealcheck-llama.env.example`: llama.cpp environment template for
  profile replay.

## Source-Build Deployment

The MVP uses source-build deployment instead of release binaries. Build both
server and CLI binaries from the checked-out repository:

```bash
cd /Users/chranama-server/MealCheck
mkdir -p bin
go build -o bin/mealcheck ./cmd/mealcheck
go build -o bin/mealcheck-server ./cmd/mealcheck-server
```

This is sufficient for MVP because the deployment target is one known MacBook
Air, the project is early, and source builds keep the release process simple.
Release binaries can be added later if there are multiple target machines or a
public download story.

## Local-Model Replay Profile

Use `deploy/local-model/` when a maintainer needs to replay the production
local-LLM path without Cloudflare. The profile runs the Go API and worker from a
source-built `bin/mealcheck-server`, stores metadata in a host-local Postgres
service, writes artifacts under `.mealcheck-local-model/artifacts`, and expects
private loopback Postgres and llama.cpp services. This mirrors the production
MacBook shape: Postgres and llama.cpp are administered outside the API runner,
not started as containers by the profile.

Quick start:

```bash
pg_isready -d 'postgres://mealcheck:mealcheck@127.0.0.1:5432/mealcheck?sslmode=disable'
curl -fsS http://127.0.0.1:11435/v1/models | jq .
mkdir -p .mealcheck-local-model
cp deploy/local-model/mealcheck-server.env.example \
  .mealcheck-local-model/mealcheck-server.env
MEALCHECK_PROFILE_ENV_FILE=.mealcheck-local-model/mealcheck-server.env \
  scripts/run-local-model-deployment-profile.sh
```

Smoke it from another terminal:

```bash
MEALCHECK_DEPLOYED_API_URL=http://127.0.0.1:8080 \
  scripts/test-deployed-local-model-live.sh
```

See `deploy/local-model/README.md` for the llama.cpp profile, optional
disposable Postgres fallback, model-name resolution, smoke evidence directory,
and reset commands.

## Secret Handling

Before deployment, copy templates to their runtime locations and replace only
the secret or machine-local placeholder values:

- `<POSTGRES_PASSWORD>`
- `<CLOUDFLARE_TUNNEL_ID>`
- `<CLOUDFLARE_ACCOUNT_TAG>`
- `<ABSOLUTE_CLOUDFLARE_CREDENTIALS_JSON>`

Never commit copied production files containing real tokens, passwords, tunnel
credentials, or private hostnames.

Production live-run access should use per-user access codes created with:

```bash
/Users/chranama-server/MealCheck/bin/mealcheck invite create \
  --label "reviewer-name" \
  --expires 2026-07-31 \
  --max-runs 20
```

Set `MEALCHECK_INVITE_REQUIRED=true` in the backend environment. The legacy
`MEALCHECK_INVITE_TOKEN` variable is still supported as a shared fallback for
local migration, but it should not be the production access model.

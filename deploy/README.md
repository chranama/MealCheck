# Deployment Package

This directory contains the local deployment package prepared for the MacBook
Air backend and Cloudflare-hosted frontend. The files use placeholder secret
values only; public production hostnames are intentionally committed.

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
| Postgres database | `mealcheck` |
| Postgres role | `mealcheck` |
| Backend listen address | `127.0.0.1:8080` |
| Frontend production URL | `https://mealcheck.dev` |
| API production URL | `https://api.mealcheck.dev` |
| Backend launchd label | `dev.mealcheck.server` |
| Postgres launchd label | `dev.mealcheck.postgres` |
| Tunnel launchd label | `dev.mealcheck.tunnel` |
| Cloudflare Tunnel name | `mealcheck-api` |
| Cloudflare Tunnel ID | `e8cbd8da-735a-4053-9503-880f636670f6` |

## Files

- `macos/mealcheck-server.env.example`: production environment template.
- `macos/dev.mealcheck.server.plist.template`: `launchd` template for
  the backend API, worker, and cleanup process as a system `LaunchDaemon`
  running as `chranama-server`.
- `macos/dev.mealcheck.postgres.plist.template`: system `LaunchDaemon`
  template for Postgres `17`, started at boot but running as
  `chranama-server`.
- `macos/dev.mealcheck.tunnel.plist.template`: system `LaunchDaemon`
  template for `cloudflared`, using the MacBook-local tunnel config at
  `/Users/chranama-server/.cloudflared/mealcheck-api.yml`.
- `macos/wait-for-mealcheck-ready.sh`: reboot-friendly readiness check that
  waits for Postgres and then the MealCheck health endpoint.
- `macos/postgres-setup.sql.template`: first-time Postgres database and role
  template.
- `cloudflare/tunnel-config.yml.template`: Cloudflare Tunnel ingress template.
- `cloudflare/pages-settings.md`: Cloudflare Pages project settings.
- `cloudflare/config.json.template`: optional runtime frontend config served as
  `/config.json`.

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

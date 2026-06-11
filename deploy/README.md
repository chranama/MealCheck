# Deployment Package

This directory contains the local deployment package prepared for the MacBook
Air backend and Cloudflare-hosted frontend. The files use placeholders only;
do not commit copied files with real secrets or production hostnames unless the
values are intentionally public.

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
| Frontend placeholder URL | `https://mealcheck.example.com` |
| API placeholder URL | `https://api.mealcheck.example.com` |
| Backend launchd label | `com.mealcheck.server` |
| Cloudflare Tunnel name | `mealcheck-api` |

## Files

- `macos/mealcheck-server.env.example`: production environment template.
- `macos/com.mealcheck.server.plist.template`: `launchd` template for the
  backend API, worker, and cleanup process.
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
the placeholder values:

- `<POSTGRES_PASSWORD>`
- `<MEALCHECK_INVITE_TOKEN>`
- `<CLOUDFLARE_TUNNEL_ID>`
- `<CLOUDFLARE_ACCOUNT_TAG>`
- `<ABSOLUTE_CLOUDFLARE_CREDENTIALS_JSON>`
- `https://mealcheck.example.com`
- `https://api.mealcheck.example.com`

Never commit copied production files containing real tokens, passwords, tunnel
credentials, or private hostnames.

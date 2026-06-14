# Cloudflare Pages Settings

Use these settings for the first public MealCheck frontend deployment.

| Setting | Value |
|---|---|
| Project name | `mealcheck` |
| Production branch | `main` |
| Root directory | `ui` |
| Build command | `npm ci && npm run build` |
| Build output directory | `dist` |
| Production frontend URL | `https://mealcheck.dev` |
| Production API URL | `https://api.mealcheck.dev` |

## Live Cloudflare State

Observed on 2026-06-14:

| Value | State |
|---|---|
| Account ID | `0f5ac9230ddfc332774b414898e9f59f` |
| Pages project | `mealcheck` |
| Git provider | `No` |
| Production deployment | `80e4ae36-17cb-4128-8a1a-d09e97fc6818` |
| Production branch | `main` |
| Pages URL | `https://mealcheck.pages.dev` |
| Custom domain | `https://mealcheck.dev`, active |

The first deployment was uploaded directly with Wrangler:

```bash
cd /Users/chranama-server/MealCheck
cd ui
npm ci
VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.dev npm run build
cd ..
wrangler pages project create mealcheck --production-branch main
wrangler pages deploy ui/dist \
  --project-name mealcheck \
  --branch main \
  --commit-hash <GIT_COMMIT> \
  --commit-message "<GIT_COMMIT_MESSAGE>" \
  --commit-dirty=true
```

The original Milestone 11 plan called for a Git-connected Pages project. The
current project is a direct-upload project because Wrangler exposes project
creation and deployment commands but not repository connection. If automated
push-to-deploy becomes required, connect the existing `mealcheck` Pages project
to the GitHub repository in the Cloudflare dashboard.

## Custom Domain DNS

Cloudflare Pages reports `mealcheck.dev` as active after adding the apex DNS
record in the `mealcheck.dev` zone:

| Type | Name | Target | Proxy |
|---|---|---|---|
| `CNAME` | `@` | `mealcheck.pages.dev` | Proxied |

Cloudflare flattens the apex CNAME. Observed Pages domain validation moved to
`active` on 2026-06-14.

## Public Environment

Preferred configuration:

```text
VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.dev
```

Alternative runtime configuration:

1. Copy `deploy/cloudflare/config.json.template` to `ui/public/config.json`.
2. Confirm the API URL is `https://api.mealcheck.dev`.
3. Build and deploy the frontend.

Only public values belong in Cloudflare Pages environment variables or
`config.json`. Do not put invite tokens, provider keys, database URLs, tunnel
credentials, or server-only paths in frontend configuration.

## CORS Pairing

The backend environment must set:

```text
MEALCHECK_ALLOWED_ORIGIN=https://mealcheck.dev
```

The API hostname should route through Cloudflare Tunnel to:

```text
http://127.0.0.1:8080
```

Observed API tunnel route on 2026-06-14:

| Value | State |
|---|---|
| Tunnel name | `mealcheck-api` |
| Tunnel ID | `e8cbd8da-735a-4053-9503-880f636670f6` |
| Public API hostname | `api.mealcheck.dev` |
| Local service | `http://127.0.0.1:8080` |

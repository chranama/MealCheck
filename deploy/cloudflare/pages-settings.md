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

Observed on 2026-06-15:

| Value | State |
|---|---|
| Account ID | `0f5ac9230ddfc332774b414898e9f59f` |
| Pages project | `mealcheck` |
| Git provider | `Yes` |
| Production deployment | `dd76ce42-4a09-4482-b38e-0ba0a8d3b0f4` |
| Source commit | `94271e5901938d1ced9dd675c264cf095fbbbac6` |
| Production branch | `main` |
| Pages URL | `https://mealcheck.pages.dev` |
| Custom domain | `https://mealcheck.dev`, active |

The active project is Git-integrated with GitHub repository
`chranama/MealCheck`. Cloudflare builds from the repository with:

```text
Root directory: ui
Build command: npm ci && npm run build
Build output directory: dist
```

Production and preview environment variables:

```text
NODE_VERSION=22
VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.dev
```

Historical note: the first Milestone 11 deployment used a Wrangler Direct
Upload Pages project. Cloudflare rejected an API attempt to attach the
repository in place with `You cannot update the source object in a Direct
Uploads project`, so the project was replaced with a new Git-integrated Pages
project and the `mealcheck.dev` custom domain was rebound.

## Custom Domain DNS

Cloudflare Pages reports `mealcheck.dev` as active after binding the custom
domain to the Git-integrated Pages project. The `mealcheck.dev` zone keeps the
apex record:

| Type | Name | Target | Proxy |
|---|---|---|---|
| `CNAME` | `@` | `mealcheck.pages.dev` | Proxied |

Cloudflare flattens the apex CNAME. Observed Pages domain validation was active
after the Git-integrated cutover on 2026-06-15.

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

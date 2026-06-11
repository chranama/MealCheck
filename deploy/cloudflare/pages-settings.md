# Cloudflare Pages Settings

Use these settings for the first public MealCheck frontend deployment.

| Setting | Value |
|---|---|
| Project name | `mealcheck` |
| Production branch | `main` |
| Root directory | `ui` |
| Build command | `npm ci && npm run build` |
| Build output directory | `dist` |
| Production frontend URL | `https://mealcheck.example.com` |
| Production API URL | `https://api.mealcheck.example.com` |

## Public Environment

Preferred configuration:

```text
VITE_MEALCHECK_API_BASE_URL=https://api.mealcheck.example.com
```

Alternative runtime configuration:

1. Copy `deploy/cloudflare/config.json.template` to `ui/public/config.json`.
2. Replace the placeholder API URL.
3. Build and deploy the frontend.

Only public values belong in Cloudflare Pages environment variables or
`config.json`. Do not put invite tokens, provider keys, database URLs, tunnel
credentials, or server-only paths in frontend configuration.

## CORS Pairing

The backend environment must set:

```text
MEALCHECK_ALLOWED_ORIGIN=https://mealcheck.example.com
```

The API hostname should route through Cloudflare Tunnel to:

```text
http://127.0.0.1:8080
```

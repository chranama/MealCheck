# MealCheck UI

This directory is the Vite/React frontend for MealCheck.

It builds to static assets. Deploy `ui/dist` from Cloudflare Pages after running
the Vite build.

The frontend is TypeScript-based and uses runtime public config for the backend
API origin. During local development, `?api=` can override the API base URL, for
example:

```text
http://localhost:4173/?api=http://127.0.0.1:8080
```

Local development:

```bash
npm install
npm run dev
```

Then open:

```text
http://localhost:4173
```

Verification:

```bash
npm run typecheck
npm test
npm run test:e2e
npm run test:e2e:local
npm run build
```

The default Playwright suite targets the installed Chrome channel and mocks
backend routes, so it does not require a running Go backend or model provider.

The local full-stack Playwright suite starts the real Go backend with memory
storage and a fake provider response path. It verifies seeded viewing, manual
run creation/deletion, BYOK fake-provider creation/redaction, and CORS headers.

Seeded artifacts live under `public/demo-runs/`. Refresh the current demo
bundle from the repository root with:

```bash
go run ./cmd/mealcheck validate \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out ui/public/demo-runs/seeded-3-day-peanut-allergy
```

The refresh command exits `1` because the seeded plan intentionally produces a
`block` decision after writing the artifact bundle.

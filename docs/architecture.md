# Architecture

MealCheck has two layers:

1. A local checker engine and CLI.
2. A hosted BYOK wrapper that schedules runs and serves reports.

The hosted layer must wrap the same engine and artifact contract. It should not
create a second product model.

## Core Flow

```text
case loader
  -> config resolver
  -> input-mode handler
  -> optional LLM generator or repairer
  -> meal-plan normalizer
  -> food and portion resolver
  -> guideline-pack rule loader
  -> deterministic checks
  -> baseline/candidate regression classifier
  -> report and artifact builder
```

## LLM Roles

MealCheck separates LLM roles from verification roles:

- Generator LLM: optional model that creates a meal plan from profile and
  constraints, or from profile, constraints, and a custom user prompt.
- Repair LLM: optional bounded repair step for malformed JSON or minor schema
  mismatches. It must not invent missing quantities, units, or nutrition-critical
  details.
- Explainer LLM: optional model that rewrites check failures in user-friendly
  language.

Nutrition totals, allergen checks, threshold checks, and source-pack compliance
must come from deterministic code and source data, not from LLM assertions.

## Components

### Checker Engine

Owns case loading, config resolution, plan normalization, food resolution,
guideline-pack loading, deterministic checks, regression classification, and
artifact generation.

Input-mode handling supports:

- manual structured entry without an LLM
- profile-only LLM generation
- prompt-based LLM generation

All three paths must produce the same normalized JSON meal-plan artifact before
verification starts.

### Guideline Pack

Versioned local data derived from official public sources. The pack stores
source metadata, derived rules, thresholds, citations, and disclaimers. Source
selection and preprocessing are documented in `docs/nutritional-guidelines.md`.

The engine should record the exact pack used in every artifact bundle.

### Nutrient Catalog

Initial nutrient data should come from local fixtures sufficient for seeded
examples. A later version can add FoodData Central lookup and cache behavior.

The resolver should make uncertainty visible. Unresolved foods are not silently
ignored.

### CLI

The first local surface:

- `mealcheck validate`
- `mealcheck compare`
- `mealcheck decision`

The CLI proves the artifact contract before service mode grows.

### Static Frontend

The first live frontend is a small Vite/React app under `ui/`. It builds to
static assets for Cloudflare Pages, reads checked-in seeded artifact bundles
from `ui/public/demo-runs/`, renders decision details, daily nutrition totals,
food resolution, source references, and artifact links, and shows backend
health when an API base URL is configured.

The seeded frontend must remain useful when the hosted backend is offline.

The Milestone 6 frontend follows the useful parts of the
`llm-extraction-platform` UI architecture:

- TypeScript for UI-facing contracts, component props, API payloads, and report
  artifact shapes.
- `src/lib/api.ts` as the only place that joins API URLs, performs JSON
  requests, formats backend errors, and exposes endpoint functions.
- `src/lib/runtime_config.ts` for public runtime configuration loaded from
  `/config.json`, with query-string override support for local development.
- Feature-oriented components instead of one large entrypoint:
  `components/common`, `components/live-run`, `components/report`, and
  `components/shell`.
- Pure utility modules for payload construction, manual-plan normalization,
  SSE parsing, artifact href construction, and report formatting.
- Test factories for seeded reports, run states, events, and API responses.
- Vitest coverage for API/config/payload/SSE boundaries and Playwright
  mocked-backend coverage for seeded, manual, BYOK profile, BYOK prompt,
  deletion, and provider-key non-persistence flows.

TypeScript is a build-time reliability tool only. It does not replace backend
JSON Schema validation, and it does not add a hosted frontend runtime. The
deployed output remains static Cloudflare Pages assets.

Runtime config may expose only public values, such as the backend API base URL
or feature flags. Invite tokens, provider keys, admin credentials, database
URLs, and tunnel credentials must never appear in runtime config, Vite env
values, frontend source, build output, reports, or artifacts.

### Hosted API

Accepts validation requests, validates inputs, creates queued jobs, streams run
events, and exposes generated reports and artifacts.

The API should not contain evaluation logic. It should orchestrate the same
engine used by the CLI.

The first hosted implementation lives in `cmd/mealcheck-server` and exposes the
documented `/api/*` surface over Go's standard HTTP server. It binds to
`127.0.0.1:8080` by default so Cloudflare Tunnel can publish it without direct
router port forwarding.

### Worker

Runs one check job at a time on the MacBook-hosted deployment target.

Initial worker policy:

- one active run
- small bounded queue
- timeout per run
- max plan size
- max model calls when BYOK generation or repair is enabled
- no anonymous paid inference

The worker processes both checked-in case paths and runtime cases generated
from hosted manual/BYOK input through the same artifact writer used by the CLI.
For BYOK runs, the worker first normalizes the plan, writes a runtime case, and
then runs deterministic evaluation.

### Storage

Initial hosted storage:

- Postgres for run metadata and job state.
- Filesystem for artifact bundles.
- Local source-pack files under version control.
- Optional nutrient-cache storage only after live FoodData Central lookup is
  added.
- No Redis until queue complexity justifies it.

The implementation includes a Postgres-backed store and applies its initial
schema at server startup. Tests use an in-memory store to avoid requiring local
Postgres for the normal development suite.

## Hosted BYOK Flow

```text
browser
  -> Cloudflare Pages static frontend
  -> Cloudflare Tunnel
  -> MacBook-hosted API
  -> run queue
  -> worker
  -> optional remote model APIs using user-provided keys
  -> local deterministic checker
  -> artifact bundle
  -> report/events/artifact endpoints
```

Keys are accepted only for live BYOK runs. Public demo runs should replay seeded
or cached artifacts. Hosted BYOK requires trusting the MealCheck backend
process because keys briefly exist in request and process memory before being
sent to the selected provider.

Milestone 5 implements this as an in-memory pending-input map shared by the API
handler and the worker. The database stores only run metadata and the generated
runtime case path. Provider API keys are removed from memory when the worker
claims the run, the run is deleted, the pending input expires, or cleanup
removes expired pending state. Expired pending inputs fail closed before
provider invocation. Custom OpenAI-compatible endpoints receive the supplied
key and must be trusted by the user.

Generation and repair are normalization steps only. The remote provider may
produce or repair a meal-plan JSON document, but nutrition compliance,
guideline checks, and report decisions are still produced by the local
deterministic checker. Repair is limited to one attempt and must not invent
missing foods, quantities, units, nutrition totals, or compliance judgments.

## Full Stack Hosting Shape

MealCheck should use split frontend/backend hosting:

- Cloudflare Pages serves the static frontend.
- Cloudflare Tunnel exposes the MacBook-hosted backend.
- The MacBook runs the API, worker, Postgres, cleanup job, source packs, and
  artifact storage.

Preferred domain shape:

- `https://mealcheck.dev` for the frontend
- `https://api.mealcheck.dev` for the backend API

The production frontend should call the backend through a public API base URL.
That value is safe to expose in frontend build output. Secrets, provider keys,
database URLs, and tunnel credentials must never be embedded in the frontend.

If the backend is offline, the static frontend should still load and show seeded
demo reports, cached examples, or a clear backend-unavailable state.

For the first live demo, Cloudflare Pages should use `ui` as the project root,
run the Vite build, and publish `ui/dist`.

## MacBook Air Deployment Target

Initial server target:

- MacBook Air Retina 13-inch, 2019
- 1.6 GHz dual-core Intel Core i5
- 8 GB RAM
- macOS Sonoma 14.8.7

Default resource envelope:

- one active hosted run
- queue size of 3 to 5 runs
- max 20 meal-check cases per run
- max 5 to 10 minute run duration
- short artifact retention
- no Kubernetes
- no local LLM inference as a primary path
- local fixture nutrient catalog for public seeded runs

## Internet Exposure

Preferred exposure:

- Cloudflare Tunnel.
- No direct router port forwarding.
- Backend reachable only through the tunnel/proxy.
- Admin or live-run features protected by authentication.

## Design Risks

- Presenting guideline checks as medical advice.
- Trusting LLM-generated nutrition totals.
- Hiding unresolved food or portion matches.
- Secret leakage through artifacts, logs, or traces.
- Public users triggering maintainer-paid inference.
- Hosted mode accepting broad arbitrary workloads too early.
- Source packs becoming stale without visible versioning.

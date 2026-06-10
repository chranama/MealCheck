# Backend Server

This document captures the first backend server target for MealCheck.

The intended server is the same constrained MacBook Air class already scoped in
this workspace:

- Host: Christophers-MacBook-Air
- User: `chranama-server`
- Project checkout: `/Users/chranama-server/MealCheck`
- OS: macOS 14.8.7
- Hardware class: 2019 Intel MacBook Air, 8 GB RAM

MealCheck should treat this machine as a constrained personal server, not a
general production cluster.

## Server Responsibilities

The backend server should run the hosted wrapper around the same checker engine
used by the CLI.

Initial responsibilities:

- serve the hosted API
- run one checker worker process
- store run metadata and job state
- write artifact bundles to local filesystem storage
- expose public seeded demo reports
- accept controlled live BYOK runs with strict limits
- clean up expired metadata and artifacts

The backend must not become a separate nutrition platform with different
semantics from the local CLI.

## Runtime Shape

Initial runtime:

- API service bound to localhost
- one worker process
- Postgres for metadata and job state
- filesystem storage for artifact bundles
- checked-in guideline packs and fixture nutrient catalog
- cleanup job for retention enforcement
- Cloudflare Tunnel for public API exposure

Avoid initially:

- Kubernetes
- Docker as a required production dependency
- Redis unless the queue becomes too complex for Postgres-backed state
- local LLM inference
- direct router port forwarding
- anonymous live inference paid for by the maintainer
- executing arbitrary user code or scraping arbitrary websites

## Frontend Boundary

The production frontend should not be served by this MacBook. It should be a
static site deployed to Cloudflare Pages.

The MacBook server owns only backend responsibilities:

- API requests
- run queueing
- worker execution
- metadata storage
- report and artifact access
- cleanup and retention
- optional BYOK provider calls

The frontend should call the backend at a public API subdomain exposed through
Cloudflare Tunnel, such as `api.mealcheck.<domain>`.

## Resource Requirements

Default resource envelope:

- one active live run
- queue size of 3 to 5 runs
- max 20 cases per run
- max 5 to 10 minutes per run
- explicit upload and output-size limits
- short artifact retention, initially 7 days
- API, worker, and cleanup processes only

These limits should be enforced in code and configuration, not only documented.

## Security Requirements

Secrets:

- User-provided model API keys must never be written to Postgres, logs, reports,
  metrics, persisted configs, or artifact bundles.
- BYOK credentials should be held only in memory or short-lived encrypted job
  state if async execution requires it.
- Persisted configs must redact secret material.
- Secrets must be discarded when a run completes, fails, expires, or is
  cancelled.

Network exposure:

- Expose the backend through Cloudflare Tunnel.
- Bind the API service to localhost unless there is a specific reason not to.
- Do not expose Postgres publicly.
- Do not use direct router port forwarding for the initial deployment.

Public access:

- Public visitors may inspect seeded reports and safe artifacts.
- Public visitors must not trigger maintainer-paid model calls.
- Live BYOK runs should require authentication or another explicit access gate.

Safety:

- MealCheck reports must include a non-medical-use disclaimer.
- Health-sensitive profile data should follow the retention and minimization
  rules in `docs/privacy-and-safety.md`.
- The hosted service should avoid collecting unnecessary personal data.

## Required Programs

The list below is the working server install plan. Exact package dependencies
belong in the project dependency file once implementation starts.

| Program | Purpose | Initial Plan |
| --- | --- | --- |
| Homebrew | macOS package manager | Use existing server install. |
| Xcode Command Line Tools | Native build tools and developer utilities | Required for package builds. |
| Git | Source control and deploy pulls | Required. |
| GitHub CLI (`gh`) | GitHub auth and repository operations | Useful for deploy and repo setup. |
| OpenSSH | SSH Git access to GitHub | Required for private deploy keys if used. |
| Go stable toolchain | Runtime implementation for checker engine, CLI, API, worker, and cleanup jobs | Preferred first implementation path. |
| Postgres | Run metadata, job state, and retention bookkeeping | Start when hosted mode needs a database. |
| Cloudflare Tunnel | Safe public API exposure without router port forwarding | Preferred exposure path. |
| `jq` | Inspecting JSON artifacts, API responses, and smoke test output | Helpful for operations and debugging. |
| Node.js | Frontend build/runtime only if the hosted UI needs it | Not a backend requirement until a frontend package exists. |
| `launchd` | Process supervision | Use LaunchAgents or LaunchDaemons after start commands exist. |
| `brew services` | Manage local services such as Postgres | Useful for Postgres. |

## Initial Install Commands

No project-specific install commands have been run yet.

Likely first install path for the Go implementation:

```bash
brew install go postgresql@17 jq cloudflared
```

Start Postgres when the backend implementation needs the database:

```bash
brew services start postgresql@17
```

The exact database name, user, migrations, and environment variables should be
added after the backend implementation defines them.

## Backend Environment

The first hosted command is:

```bash
go run ./cmd/mealcheck-server
```

Default behavior:

- bind address: `127.0.0.1:8080`
- metadata store: Postgres
- runtime data: `.mealcheck-data/`
- artifact storage: `.mealcheck-data/artifacts/`
- queue size: 3
- active workers: 1
- run timeout: 10 minutes
- retention: 7 days

Required for Postgres-backed mode:

```bash
export DATABASE_URL='postgres://mealcheck:mealcheck@localhost:5432/mealcheck?sslmode=disable'
```

Useful local development mode without Postgres:

```bash
go run ./cmd/mealcheck-server -store memory
```

Supported configuration:

- `MEALCHECK_ADDR`
- `MEALCHECK_DATA_DIR`
- `MEALCHECK_ARTIFACT_DIR`
- `MEALCHECK_STORE`
- `DATABASE_URL`
- `MEALCHECK_ALLOWED_ORIGIN`
- `MEALCHECK_INVITE_TOKEN`
- `MEALCHECK_QUEUE_SIZE`
- `MEALCHECK_MAX_UPLOAD_BYTES`
- `MEALCHECK_RUN_TIMEOUT`
- `MEALCHECK_RETENTION`
- `MEALCHECK_WORKER_POLL`
- `MEALCHECK_CLEANUP_INTERVAL`

The Postgres schema is applied at server startup by the Postgres store.

## Operating Requirements

The MacBook should be configured as a server:

- keep it plugged into power
- prefer wired Ethernet through USB-C if available
- disable sleep while plugged in
- keep macOS security updates current
- enable automatic restart after power failure if available
- keep the `MealCheck` checkout under `/Users/chranama-server/MealCheck`
- keep generated artifacts outside the Git checkout once service mode exists

Proposed future local paths:

- Repository: `/Users/chranama-server/MealCheck`
- Runtime data: `/Users/chranama-server/MealCheck-data`
- Artifact storage: `/Users/chranama-server/MealCheck-data/artifacts`
- Logs: `/Users/chranama-server/MealCheck-data/logs`

These paths are provisional until the implementation lands.

## Server Readiness Checklist

The server is ready for the first hosted proof when:

- the repository pulls cleanly from GitHub
- the Go toolchain is available
- project dependencies resolve with `go mod`
- Postgres is installed, running, and reachable locally for production-style
  metadata storage
- the backend can run a seeded no-secret meal-plan check
- the API can serve a seeded report from local artifacts
- the Cloudflare Pages frontend can call the tunneled backend API
- one worker processes jobs with configured timeouts and queue limits
- generated artifacts do not include secrets
- a tunnel exposes only the intended HTTP surface
- start, stop, health check, and cleanup commands are documented in `runbook.md`

## Open Decisions

- Decide final runtime data and artifact paths.

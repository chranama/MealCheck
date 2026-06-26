# MealCheck

[![CI](https://github.com/chranama/MealCheck/actions/workflows/ci.yml/badge.svg)](https://github.com/chranama/MealCheck/actions/workflows/ci.yml)

MealCheck verifies ingredient-level meal plans against user constraints and
versioned public guideline sources. Plans may come from a user, an LLM, or a
bounded normalization workflow, but guideline compliance is checked
deterministically.

It answers one practical question:

`Does this meal plan satisfy the declared checks well enough to use, or should it be revised?`


## What It Does

- Accepts concise ingredient-level meal-plan text through the hosted
  local-model path.
- Keeps BYOK/custom-provider generation and repair in the configurable backend
  API for local, self-hosted, and debug deployments.
- Keeps the local CLI as a deterministic case-file verifier and artifact
  writer; it does not call remote model providers.
- Defines a qualification boundary for whether candidate content is specific
  enough to become a verifiable meal plan.
- Normalizes the plan into a strict meal-plan schema.
- Resolves foods and portions against a reviewed nutrient catalog, with an
  optional exact-match FNDDS SQLite fallback for eligible gram-based misses.
- Applies deterministic checks from a versioned guideline pack.
- Produces a `pass`, `warn`, or `block` decision with evidence.
- Preserves seeded proof artifacts, local structured case-file verification,
  and a local llama compact-output adapter for development and regression work.

## Current Shape

The repository contains a Vite/React static frontend, a hosted API/worker, a
checker core, JSON Schemas, seeded fixtures, a reviewed nutrient catalog, an
optional FNDDS fallback path, and a local CLI that writes artifact bundles from
case files. The hosted API can be configured either for the public local-model
workflow or for local/self-hosted BYOK provider workflows.

The deployed MVP shape is:

- static frontend on Cloudflare Pages at `https://mealcheck.dev`
- MacBook-hosted backend exposed through Cloudflare Tunnel at
  `https://api.mealcheck.dev`
- private llama.cpp local model service on the MacBook backend
- hosted live verification through the server-owned local model
- seeded proof artifacts preserved in the repository, including
  [a rendered HTML report](https://chranama.github.io/MealCheck/seeded-report.html)
  served by GitHub Pages from the
  [checked-in source file](docs/seeded-report.html)
- BYOK OpenAI, Anthropic, Gemini, and custom OpenAI-compatible providers
  preserved for local or self-hosted backend/API deployments, plus debug and
  regression tests
- deterministic structured case-file verification preserved in the local CLI
  and debug path

The Cloudflare Pages frontend is connected
to the GitHub repository and automatically deploys from `main`.

The hosted website is a bounded local-model verification demo: paste a concise
ingredient-level meal plan and receive a source-backed report. The downloaded
repository is the deterministic local verifier/debug surface, the self-hostable
BYOK/custom endpoint surface, and the intended base for future agent-tool
integration.
Multi-day hosted inputs work best when each day is labeled explicitly, such as
`Day 1`, `Day 2`, and so on, with meals and ingredient amounts grouped under
the correct day. Clear day sections let the backend process each day in a
smaller local-model call; ambiguous multi-day text falls back to the unbatched
whole-plan path.

The tightened product direction adds first-class meal-plan qualification before
verification: candidate text may be not a meal plan, too vague, recipe-like but
undecomposed, eligible, or eligible with unresolved items.

## Verified By CI

GitHub Actions runs the core proof gates on every push to `main`, pull request,
and manual dispatch:

- fixture validation with `go run ./cmd/mealcheck fixture-check`
- backend tests with `go test ./...`
- local CLI/API smoke proof with `go run ./cmd/mealcheck local-smoke`
- frontend typecheck, unit tests, and build
- mocked Playwright browser workflow
- local-stack Playwright workflow against the real Go backend, memory storage,
  and fake provider response

Deployed live checks remain release operations because they depend on the
production tunnel, hosted local model, and optional external provider paths.

## Documentation

- [Documentation Index](docs/README.md)
- [Product](docs/product.md)
- [User Story](docs/user-story.md)
- [Current Priorities](docs/current-priorities.md)
- [Plan Recommendation](docs/plan_recommendation.md)
- [Nutritional Guidelines](docs/nutritional-guidelines.md)
- [Privacy And Safety](docs/privacy-and-safety.md)
- [Contracts](docs/contracts.md)
- [CLI](docs/cli.md)
- [API](docs/api.md)
- [Architecture](docs/architecture.md)
- [Web Design](docs/web-design.md)
- [Backend Server](docs/backend_server.md)
- [Implementation Plan](docs/implementation-plan.md)
- [Runbook](docs/runbook.md)
- [Decision Log](docs/decision-log.md)
- [Deployment Package](deploy/README.md)

For local validation, development, build, deployment, and smoke-test commands,
use the [Runbook](docs/runbook.md).

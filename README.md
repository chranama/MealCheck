# MealCheck

[![CI](https://github.com/chranama/MealCheck/actions/workflows/ci.yml/badge.svg)](https://github.com/chranama/MealCheck/actions/workflows/ci.yml)

MealCheck evaluates ingredient-level meal plans against declared user constraints
and rules derived from versioned public guideline sources. It answers one bounded
question:

`Does this meal plan violate my constraints or source-backed rules, and what should I fix next?`

In the hosted web deployment, a user submits concise meal-plan text. The
server-owned local model normalizes the text into a source-linked structured
plan. After the user reviews, corrects if needed, and confirms that plan,
deterministic code performs the checks and produces the decision.

Current build: v1.0, the baseline for this hosted local-model workflow. Contract
and schema versions are tracked separately.


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
- Produces a `pass`, `warn`, or `block` decision with source-backed findings
  and concrete recovery guidance.
- Preserves seeded artifacts, local structured case-file verification,
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
- seeded artifacts preserved in the repository, including
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

The hosted website checks one concise ingredient-level day of meals against
declared constraints and source-backed rules. The downloaded repository is the
deterministic local verifier/debug surface and the self-hostable BYOK/custom
endpoint surface.
Hosted local-model runs accept one day only. Semi-structured lines and paragraph
text are both supported when meals have clear anchors and bounded food spans.
The backend chunks the day by meal, sends each meal text span plus its numbered
source items to the small local model, and reattaches deterministic meal
metadata after source-ID reconciliation. Multi-day plans, recipes, grocery
lists, and unrelated long prose belong in a local/self-hosted workflow or should
be split before submission. The public path keeps the small local model on the
critical path by requiring it to normalize each meal chunk before deterministic
checks run.

The tightened product direction adds first-class meal-plan qualification before
verification: candidate text may be not a meal plan, too vague, recipe-like but
undecomposed, outside the hosted one-day contract, eligible, or eligible with
unresolved items.

## Verified By CI

GitHub Actions runs the core CI gates on every push to `main`, pull request,
and manual dispatch:

- fixture validation with `go run ./cmd/mealcheck fixture-check`
- backend tests with `go test ./...`
- local CLI/API smoke path with `go run ./cmd/mealcheck local-smoke`
- frontend typecheck, unit tests, and build
- mocked Playwright browser workflow
- local-stack Playwright workflow against the real Go backend, memory storage,
  and fake provider response

Deployed live checks remain release operations because they depend on the
production tunnel, hosted local model, and optional external provider paths.

## Reviewer Path

For a fast technical review, start with:

1. This README for the product boundary, hosted shape, and CI gates.
2. [`docs/current-priorities.md`](docs/current-priorities.md) for implemented
   slices, evaluation results, operating loops, and deferred non-goals.
3. [`docs/evaluation.md`](docs/evaluation.md) for the P0 normalization and P1
   food/unit resolver metrics and commands.
4. [`docs/architecture.md`](docs/architecture.md) for the local-model,
   deterministic-checker, API/worker, storage, and frontend boundaries.
5. [`docs/decision-log.md`](docs/decision-log.md) for tradeoffs around
   normalized-plan review, correction artifacts, source inspection, local replay,
   local-model capacity, and Python operator tooling.
6. [`docs/runbook.md`](docs/runbook.md) for local validation, deployment,
   smoke-test, and artifact-inspection workflows.

Trace one hosted local-model run through the repo surfaces: input text becomes a
source-linked normalized plan, review actions are retained, deterministic checks
produce the decision, and completed reports link findings back to source rows
and guideline references.

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

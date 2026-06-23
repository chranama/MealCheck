# MealCheck

MealCheck verifies LLM-generated or LLM-normalized meal plans against user
constraints and versioned public guideline sources.

It answers one practical question:

`Does this meal plan satisfy the declared checks well enough to use, or should it be revised?`


## What It Does

- Accepts concise ingredient-level meal plans through the hosted local-model
  path, plus BYOK/custom-provider and structured JSON workflows in the local
  CLI/API/debug surfaces.
- Defines a qualification boundary for whether candidate content is specific
  enough to become a verifiable meal plan.
- Normalizes the plan into a strict meal-plan schema.
- Resolves foods and portions against a nutrient catalog.
- Applies deterministic checks from a versioned guideline pack.
- Produces a `pass`, `warn`, or `block` decision with evidence.
- Supports server-local model verification, seeded repo proof artifacts,
  BYOK/custom endpoint verification for local/API users, and local structured JSON
  verification.

## Current Shape

The project has a Vite/React frontend and hosted API. It has seeded fixtures,
JSON Schemas, a small local nutrient catalog, a checker core, a local CLI that
writes the artifact bundle, a static frontend with a hosted local-model
verification workflow, a hosted API/worker wrapper, and local/API
bring-your-own-key generation and repair paths.

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
  preserved for repo/CLI/API and self-hosted deployments
- structured manual JSON verification preserved in the local CLI/debug path

The Cloudflare Pages frontend is connected
to the GitHub repository and automatically deploys from `main`.

The hosted website is a bounded local-model verification demo: paste a concise
ingredient-level meal plan and receive a source-backed report. The downloaded
repository is the higher-control local verifier/debug surface, the BYOK/custom
endpoint surface, and the intended base for future agent-tool integration.
Multi-day hosted inputs work best when each day is labeled explicitly, such as
`Day 1`, `Day 2`, and `Day 3`, with meals and ingredient amounts grouped under
the correct day. Clear day sections let the backend process each day in a
smaller local-model call; ambiguous multi-day text falls back to the unbatched
whole-plan path.

The tightened product direction adds first-class meal-plan qualification before
verification: candidate text may be not a meal plan, too vague, recipe-like but
undecomposed, eligible, or eligible with unresolved items.


## Documentation

- [Documentation Index](docs/README.md)
- [Product](docs/product.md)
- [User Story](docs/user-story.md)
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

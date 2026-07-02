# Local-Model Deployment Profile

This profile replays the hosted local-model runtime without Cloudflare. In this
project, "local" means MealCheck's server-owned local LLM path: the API accepts
`local_model` input, normalizes through a private llama.cpp endpoint, pauses for
source-linked review, writes report artifacts, and stores run metadata in
Postgres.

The profile components are:

- `mealcheck-server` API, worker, and cleanup process built from this checkout
- host-local Postgres metadata storage on `127.0.0.1:5432`
- filesystem artifact storage under `.mealcheck-local-model/artifacts`
- llama.cpp OpenAI-compatible endpoint on `127.0.0.1:11435/v1`

## Verify Local Services

The canonical profile matches production shape: Postgres and llama.cpp are
local host services administered outside this runner, not containers started by
the profile. Verify Postgres first:

```bash
pg_isready -d 'postgres://mealcheck:mealcheck@127.0.0.1:5432/mealcheck?sslmode=disable'
```

Verify the local model endpoint:

```bash
curl -fsS http://127.0.0.1:11435/v1/models | jq .
```

## Optional Disposable Postgres

Use the bundled Compose file only when a local Postgres service is unavailable
on a developer laptop. This is a convenience fallback, not the production-parity
profile:

```bash
docker compose -f deploy/local-model/compose.postgres.dev.yml up -d
```

The fallback Postgres listens on host port `5433` to avoid clashing with a
host-local `5432` service. When using it, copy
`deploy/local-model/mealcheck-server.env.example` and change `DATABASE_URL` to:

```bash
DATABASE_URL='postgres://mealcheck:mealcheck@127.0.0.1:5433/mealcheck?sslmode=disable'
```

## Start llama.cpp Manually

To start a profile llama endpoint from the checkout, copy the env template,
edit the model path, and run the existing wrapper from the repository root:

```bash
mkdir -p .mealcheck-local-model/models .mealcheck-local-model/logs
cp deploy/local-model/mealcheck-llama.env.example \
  .mealcheck-local-model/mealcheck-llama.env

MEALCHECK_LLAMA_ENV_FILE=.mealcheck-local-model/mealcheck-llama.env \
  deploy/macos/mealcheck-llama-server.sh
```

The local model service must stay bound to loopback. Do not expose
`127.0.0.1:11435` through Cloudflare or router forwarding.

## Start The API

Copy the API env template if local changes are needed:

```bash
mkdir -p .mealcheck-local-model
cp deploy/local-model/mealcheck-server.env.example \
  .mealcheck-local-model/mealcheck-server.env
```

Run the profile:

```bash
MEALCHECK_PROFILE_ENV_FILE=.mealcheck-local-model/mealcheck-server.env \
  scripts/run-local-model-deployment-profile.sh
```

The runner builds `bin/mealcheck-server`, waits for the configured Postgres
service, resolves `MEALCHECK_LOCAL_MODEL_NAME=auto` from `/v1/models`, creates
the data and artifact directories, and starts the API on `127.0.0.1:8080`.

## Smoke The Profile

In another terminal, run the deployed local-model smoke test against the profile:

```bash
MEALCHECK_DEPLOYED_API_URL=http://127.0.0.1:8080 \
MEALCHECK_DEPLOYED_OUTPUT_DIR=.mealcheck-local-model/smoke/$(date +%Y%m%d-%H%M%S) \
  scripts/test-deployed-local-model-live.sh
```

The smoke path submits a bounded one-day local-model run, waits for
`awaiting_review`, fetches the normalized-plan review artifact, confirms the
review, verifies completed review and recommendation artifacts, checks local
model rejection policy, deletes the run, and confirms the deleted run is no
longer retrievable.

## Stop And Reset

Stop the API with `Ctrl-C`. If you used the optional disposable Postgres
fallback, stop it with:

```bash
docker compose -f deploy/local-model/compose.postgres.dev.yml down
```

Remove generated profile data when a clean replay is needed:

```bash
rm -rf .mealcheck-local-model
```

If you also used the optional disposable Postgres fallback, remove its volume
with:

```bash
docker compose -f deploy/local-model/compose.postgres.dev.yml down -v
```

# CLI

MealCheck exposes a local CLI for deterministic meal-plan checking, artifact
bundle generation, decision exit-code enforcement, and hosted access-code
administration.

The CLI is intended for local validation and operator workflows. It does not
call remote LLM providers, does not use BYOK provider keys, and does not start
the hosted API. Hosted live generation is documented in [API](api.md).

Run commands from the repository root unless a command uses `--root`.

## Build And Run

During development, run the CLI through Go:

```bash
go run ./cmd/mealcheck help
```

Build a local binary:

```bash
mkdir -p bin
go build -o bin/mealcheck ./cmd/mealcheck
```

Then run:

```bash
bin/mealcheck help
```

The deployed MacBook layout uses:

```text
/Users/chranama-server/MealCheck/bin/mealcheck
```

## Commands

| Command | Purpose |
|---|---|
| `mealcheck validate` | Evaluate a case file and write the full artifact bundle. |
| `mealcheck compare` | Exercise the baseline/candidate command surface and write a compare-mode bundle. |
| `mealcheck decision` | Read an existing `decision.json` and apply MealCheck exit-code policy. |
| `mealcheck local-llama normalize` | Expand compact local llama JSON into canonical MealCheck plan JSON. |
| `mealcheck local-llama schema` | Print the compact local llama response schema. |
| `mealcheck invite create` | Create a hosted API access code in Postgres. |
| `mealcheck invite list` | List hosted API access-code metadata from Postgres. |
| `mealcheck invite revoke` | Revoke a hosted API access code by id. |

Use `mealcheck help` for the command summary:

```text
usage:
  mealcheck validate --case <case.json> [--out artifacts/latest] [--strict]
  mealcheck compare --case <case.json> [--out artifacts/latest] [--strict]
  mealcheck decision [--strict] <decision.json>
  mealcheck local-llama normalize --input compact.json [--out normalized-plan.json]
  mealcheck local-llama schema
  mealcheck invite create --label <label> [--expires YYYY-MM-DD] [--max-runs N]
  mealcheck invite list
  mealcheck invite revoke <access-code-id>
```

## Exit Codes

The CLI uses the same decision policy for `validate`, `compare`, and
`decision`.

| Exit Code | Meaning |
|---:|---|
| `0` | The command succeeded and the decision was `pass`, or `warn` without `--strict`. |
| `1` | The command succeeded and the decision was `block`, or `warn` with `--strict`. |
| `2` | Invalid command usage, configuration error, unreadable input, resolver failure, or unusable artifacts. |

Decision values:

| Decision | Default Exit | Strict Exit | Meaning |
|---|---:|---:|---|
| `pass` | `0` | `0` | No blocking violation detected. |
| `warn` | `0` | `1` | Review needed, but not an automatic block by default. |
| `block` | `1` | `1` | Plan should not be used without revision. |

The seeded example intentionally returns `block`, so a successful seeded
validation exits `1` after writing artifacts.

## Validate A Case

`validate` loads a checked-in or local case JSON file, evaluates the candidate
meal plan, writes a full artifact bundle, prints the decision summary, and
exits according to the decision policy.

```bash
go run ./cmd/mealcheck validate \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out artifacts/latest
```

Equivalent binary form:

```bash
bin/mealcheck validate \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out artifacts/latest
```

Options:

| Option | Default | Meaning |
|---|---|---|
| `--root` | `.` | Repository root used to resolve case, data, schemas, and guideline files. |
| `--case` | required | Case JSON path, relative to `--root`. |
| `--out` | `artifacts/latest` | Artifact output directory. |
| `--strict` | `false` | Treat `warn` decisions as failing with exit code `1`. |

Typical output:

```text
case: seeded-3-day-peanut-allergy
decision: block
risk: high
summary: Peanut allergy conflict detected.
checks requiring attention:
- allergy.peanuts
artifacts: artifacts/latest
report: artifacts/latest/report.md
```

The exact summary and failed checks depend on the case data and guideline pack.

## Compare A Case

`compare` currently runs the same seeded evaluation path as `validate`, but
writes `compare` into `manifest.json`. It preserves the CLI shape intended for
future baseline-versus-candidate regression checks.

```bash
go run ./cmd/mealcheck compare \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out artifacts/latest-compare
```

Options are the same as `validate`:

| Option | Default | Meaning |
|---|---|---|
| `--root` | `.` | Repository root used to resolve case, data, schemas, and guideline files. |
| `--case` | required | Case JSON path, relative to `--root`. |
| `--out` | `artifacts/latest` | Artifact output directory. |
| `--strict` | `false` | Treat `warn` decisions as failing with exit code `1`. |

## Read A Decision

`decision` reads an existing `decision.json`, prints the same decision summary,
and exits according to the same policy used by `validate` and `compare`.

```bash
go run ./cmd/mealcheck decision artifacts/latest/decision.json
```

Strict mode treats `warn` as failing:

```bash
go run ./cmd/mealcheck decision --strict artifacts/latest/decision.json
```

Options:

| Option | Default | Meaning |
|---|---|---|
| `--strict` | `false` | Treat `warn` decisions as failing with exit code `1`. |

`decision` requires exactly one positional path to a `decision.json` file.
Unknown fields in that file are rejected.

## Local llama Adapter

`local-llama` supports the local model feasibility harness. It does not start
`llama-server` and does not call a remote provider.

`local-llama schema` prints the active compact JSON Schema used for llama.cpp
schema-constrained decoding. The active schema is the source-ID row contract:

```bash
go run ./cmd/mealcheck local-llama schema
```

`local-llama normalize` expands compact model output into canonical MealCheck
plan JSON. It accepts the active source-ID row contract, the earlier v3
`[day, meal_code, food, quantity, unit]` row contract, the v2 `b`/`l`/`d` tuple
contract, and the first object-item compact contract used by old local artifacts:

```bash
go run ./cmd/mealcheck local-llama normalize \
  --input /tmp/mealcheck-local-llama/run-1/compact-plan.json \
  --out /tmp/mealcheck-local-llama/run-1/normalized-plan.json \
  --plan-id local-llama-smoke
```

Active compact input shape:

```json
{
  "i": [
    [1, 1, "b", "cooked oatmeal", 1, "cup"],
    [2, 1, "l", "grilled chicken breast", 4, "oz"],
    [3, 1, "d", "baked salmon", 4, "oz"]
  ]
}
```

Each active row is `[source_item_id, day, meal_code, food, quantity, unit]`.
Meal codes are `b` breakfast, `m` morning snack, `l` lunch, `a` afternoon snack,
`d` dinner, `s` snack, and `e` evening snack. The adapter rejects unknown
fields, malformed rows, missing or duplicate source item IDs, unsupported meal
codes, nonpositive quantities, and unsupported units.

## Artifact Bundle

`validate` and `compare` write the shared artifact bundle used by the local CLI
and hosted worker.

```text
artifacts/latest/
  decision.json
  recommendation.json
  report.json
  report.html
  report.pdf
  report.md
  failures.jsonl
  daily-totals.json
  resolved-foods.json
  unresolved-foods.json
  metrics.json
  manifest.json
  normalized-plan.json
  configs/
    run.json
    redacted-provider.json
  guideline-pack/
    pack.json
    citations.json
  schemas/
    decision.schema.json
    meal-plan.schema.json
    guideline-pack.schema.json
    nutrient-catalog.schema.json
    report.schema.json
```

`redacted-provider.json` is present for contract parity with hosted BYOK runs.
Local CLI validation does not use provider keys.

Important files:

| File | Purpose |
|---|---|
| `decision.json` | Machine-readable decision, risk, failed checks, unresolved items, and artifact links. |
| `recommendation.json` | Deterministic modified-plan recommendation attempt, or an unavailable reason. |
| `report.json` | Full machine-readable report. |
| `report.md` | Human-readable report. |
| `report.html` | Browser-readable report. |
| `report.pdf` | Portable report artifact. |
| `failures.jsonl` | One failed-check record per line. |
| `daily-totals.json` | Resolved daily nutrient totals. |
| `resolved-foods.json` | Foods and quantities resolved against the nutrient catalog. |
| `unresolved-foods.json` | Items that could not be fully resolved. |
| `metrics.json` | Run metrics. |
| `manifest.json` | Artifact list, command mode, and bundle metadata. |
| `normalized-plan.json` | Candidate meal plan after normalization. |

## Case Input

`validate` and `compare` accept a case file with `settings`,
candidate meal plan, nutrient catalog reference, and guideline-pack reference.
`settings` contains only `nutrition_targets` and `verification_constraints`;
old top-level `profile` and `constraints` fields are rejected as unknown fields.

The seeded proof case is:

```text
examples/seeded-3-day-peanut-allergy/case.json
```

Use `--root` when running the binary outside the repository root:

```bash
/Users/chranama-server/MealCheck/bin/mealcheck validate \
  --root /Users/chranama-server/MealCheck \
  --case examples/seeded-3-day-peanut-allergy/case.json \
  --out /Users/chranama-server/MealCheck-data/artifacts/cli-smoke
```

`--case` is interpreted relative to `--root`.

## Hosted Access Codes

The `invite` subcommands manage access codes for the hosted API documented in
[API](api.md). These commands use Postgres and require either `DATABASE_URL` or
`--database-url`.

They do not work against the in-memory local server store.

### Create An Access Code

```bash
mealcheck invite create \
  --label reviewer-chris \
  --expires-in 720h \
  --max-runs 5
```

With an explicit database URL:

```bash
mealcheck invite create \
  --database-url "$DATABASE_URL" \
  --label reviewer-chris \
  --expires 2026-07-17 \
  --max-runs 5
```

Options:

| Option | Default | Meaning |
|---|---|---|
| `--database-url` | `$DATABASE_URL` | Postgres database URL. |
| `--label` | required | Human-readable reviewer or access-code label. |
| `--expires` | none | Expiry as `YYYY-MM-DD` or RFC3339. |
| `--expires-in` | none | Expiry duration such as `720h`. |
| `--max-runs` | unlimited | Optional maximum number of run creations. |

Use only one of `--expires` and `--expires-in`.

The command prints the full access code exactly once:

```text
id: inv_...
label: reviewer-chris
expires_at: 2026-07-17T00:00:00Z
max_runs: 5
access_code: mc_inv_...
store this access code now; MealCheck stores only its hash.
```

Store the full `access_code` securely. MealCheck stores only its secret hash and
cannot show the full code again.

### List Access Codes

```bash
mealcheck invite list
```

With an explicit database URL:

```bash
mealcheck invite list --database-url "$DATABASE_URL"
```

Output is tab-separated metadata:

```text
inv_...  reviewer-chris  used=1/5  expires=2026-07-17T00:00:00Z
```

The full access-code secret is never shown by `list`.

### Revoke An Access Code

```bash
mealcheck invite revoke inv_...
```

With an explicit database URL:

```bash
mealcheck invite revoke --database-url "$DATABASE_URL" inv_...
```

Revocation marks the code unusable for future run creation. Existing completed
run artifacts are not deleted by revoking an access code.

## Local Smoke Test

Run the local CLI smoke command from the repository root:

```bash
go run ./cmd/mealcheck-local-smoke
```

The smoke command builds the CLI in a temporary clean build directory, runs the
seeded validation, verifies the expected `block` exit behavior, reads the
decision artifact, starts a local hosted stack with a fake provider response,
and verifies BYOK redaction behavior.

## BYOK Key Posture

The `mealcheck` CLI commands do not accept provider API keys and do not call
OpenAI, Anthropic, Gemini, or OpenAI-compatible endpoints. That is intentional:
CLI validation is deterministic and works from existing case files.

For the highest-control BYOK workflow, clone the repository and run the
MealCheck backend locally from the terminal, then submit BYOK requests to
`127.0.0.1` with temporary, scoped, budget-limited provider keys. In that
configuration the key still transits the browser or local `curl` process and
the local MealCheck backend process, but it is not sent to a hosted MealCheck
operator. Avoid using long-lived personal account keys for MealCheck testing.

## Relationship To The API

The CLI and hosted API share the checker engine and artifact writer:

- `mealcheck validate` and `mealcheck compare` write local artifact bundles
  synchronously.
- `POST /api/runs` queues hosted work asynchronously and exposes artifacts
  through API links after the worker completes.
- CLI commands do not accept provider API keys and do not call OpenAI,
  Anthropic, Gemini, or OpenAI-compatible model endpoints.
- Hosted BYOK generation and repair are API/server features, not CLI features.
- Hosted BYOK should be treated as a convenience test surface. Local backend
  operation is the recommended path when provider-key control matters most.

Use the CLI when you want a deterministic local check from an existing case
file or structured JSON debugging workflow. Use the hosted API or web UI when
you want public policy-limited qualification preflight or BYOK model
generation. Invite-required mode remains available for private deployments.

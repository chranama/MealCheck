# API

MealCheck exposes a small hosted API for public demo reports and invite-gated
live meal-plan checks. The frontend should treat this API as asynchronous:
creating a run queues work, and separate endpoints expose status, events,
reports, and artifacts.

The API is intended for a static frontend plus a small self-hosted backend. It
does not run a local LLM. When a live generation run needs an LLM, the caller
supplies a BYOK provider key in the request.

## Authentication

`GET` endpoints are currently public. `DELETE /api/runs/{run_id}` is also
public in the MVP API and treats the unguessable run id as the deletion
capability. Live run creation can be access-code gated with
`MEALCHECK_INVITE_REQUIRED=true`. Per-user access codes are created by an
operator with `mealcheck invite create`; the full code is shown once, while the
backend stores only the secret hash and usage metadata.

```http
X-MealCheck-Invite-Token: <access-code>
```

The legacy `MEALCHECK_INVITE_TOKEN` environment variable is still supported as
a shared access-code fallback during local migration. Production deployments
should prefer per-user access codes so individual reviewers can have expiry,
revocation, and run limits.

The server always adds an `X-Request-ID` response header. A client may send its
own `X-Request-ID`; otherwise, the server assigns one.

Provider API keys are supplied only on live generation requests. Treat them as
one-run bearer secrets: the browser sends the key to the MealCheck backend, the
backend holds it in pending memory until the worker claims the run, and the
backend sends it to the selected provider endpoint. MealCheck does not persist
provider keys to run metadata, reports, logs, metrics, runtime case files, or
artifact bundles. Hosted BYOK users should use temporary, scoped,
budget-limited, revocable keys; for maximum control, run MealCheck locally from
the repository and submit requests to the local backend.

## Runtime Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/health` | Service health, queue, store, and retention snapshot. |
| `GET` | `/api/demo-runs` | List static public demo reports. |
| `GET` | `/api/demo-runs/{demo_id}` | Read static demo metadata and decision summary. |
| `GET` | `/api/demo-runs/{demo_id}/report` | Fetch a static demo `report.json`. |
| `GET` | `/api/demo-runs/{demo_id}/artifacts` | List static demo artifacts. |
| `GET` | `/api/demo-runs/{demo_id}/artifacts/{path}` | Fetch one static demo artifact. |
| `POST` | `/api/runs` | Queue a live meal-plan check. |
| `GET` | `/api/runs/{run_id}` | Read live run status and summary. |
| `GET` | `/api/runs/{run_id}/events` | Read live run events as an SSE stream. |
| `GET` | `/api/runs/{run_id}/report` | Fetch completed live run `report.json`. |
| `GET` | `/api/runs/{run_id}/artifacts` | List completed live run artifacts. |
| `GET` | `/api/runs/{run_id}/artifacts/{path}` | Fetch one completed live run artifact. |
| `DELETE` | `/api/runs/{run_id}` | Delete live run metadata, pending input, and artifacts. |

## Run Lifecycle

`POST /api/runs` does not wait for the meal-plan check to complete. It validates
and queues the request, appends a `queued` event, and returns `202 Accepted`
with a `run_id` and links.

The client should then:

1. Read `GET /api/runs/{run_id}` for current status.
2. Read `GET /api/runs/{run_id}/events` for progress events.
3. When status becomes `completed`, fetch `GET /api/runs/{run_id}/report` or
   `GET /api/runs/{run_id}/artifacts`.
4. If status becomes `failed`, display the run `error` field.
5. Optionally call `DELETE /api/runs/{run_id}` when the user wants to remove
   stored run data.

Run statuses:

| Status | Meaning |
|---|---|
| `queued` | The request was accepted and is waiting for the worker. |
| `running` | The worker claimed the run. |
| `completed` | The report and artifact bundle were written. |
| `failed` | The worker could not complete the run. |
| `deleted` | The run was deleted by request. |

Run events:

| Event | Meaning |
|---|---|
| `queued` | Run was accepted. |
| `started` | Worker started processing. |
| `plan_normalized` | Hosted input was converted into a normalized plan. |
| `artifact_written` | Artifact bundle was written. |
| `completed` | Decision is available. |
| `failed` | Run failed. |

## Create A Run

`POST /api/runs` accepts JSON only. Unknown fields are rejected. The default
request body limit is `1,000,000` bytes, configured by
`MEALCHECK_MAX_UPLOAD_BYTES`.

The response is intentionally a job handle, not the final report:

```json
{
  "run_id": "run_8d8c75f9a22a4be7f4533e73",
  "status": "queued",
  "expires_at": "2026-06-18T15:04:05Z",
  "links": {
    "self": "/api/runs/run_8d8c75f9a22a4be7f4533e73",
    "events": "/api/runs/run_8d8c75f9a22a4be7f4533e73/events",
    "report": "/api/runs/run_8d8c75f9a22a4be7f4533e73/report",
    "artifacts": "/api/runs/run_8d8c75f9a22a4be7f4533e73/artifacts"
  }
}
```

### Request Modes

`POST /api/runs` supports four request shapes.

| Mode | Fields | LLM Used | Purpose |
|---|---|---:|---|
| Checked-in case | `case_path` | no | Developer/demo compatibility for checked-in examples. |
| `manual_structured` | `input_mode`, `profile`, `constraints`, `candidate_plan` | no | Check a structured meal plan supplied by the user. |
| `profile_generation` | `input_mode`, `profile`, `constraints`, `provider` | yes | Ask a provider to generate a plan from the profile and constraints. |
| `prompt_generation` | `input_mode`, `profile`, `constraints`, `generation_prompt`, `provider` | yes | Ask a provider to generate a plan from a user prompt plus profile and constraints. |

`case_path` cannot be combined with `input_mode`. Hosted live requests should
use one of the three `input_mode` values.

### Profile

```json
{
  "age": 35,
  "sex": "male",
  "height_cm": 178,
  "weight_kg": 82,
  "activity_level": "moderate",
  "goal": "maintain_weight",
  "calorie_target_kcal": 2000,
  "protein_target_g": 98
}
```

Validation rules:

- `age` must be at least `18`.
- `sex` must be `male` or `female`.
- `height_cm` and `weight_kg` must be positive.
- `activity_level` must be `inactive`, `low_active`, `moderate`, `active`, or
  `very_active`.

### Constraints

```json
{
  "days": 3,
  "meals_per_day": 3,
  "allergies": ["peanuts"],
  "excluded_foods": ["shellfish"],
  "diet_pattern": "general",
  "max_sodium_mg_per_day": 2300,
  "max_added_sugar_g_per_meal": 10,
  "max_saturated_fat_pct_calories": 10,
  "calorie_tolerance_pct": 15,
  "requires_shopping_list": true,
  "requires_prep_safety_notes": true
}
```

Validation rules:

- `days` must be between `1` and `7`.
- `meals_per_day` must be between `1` and `6`.
- Constraint fields that are omitted use Go zero values. The current frontend
  should send explicit values for all MVP guideline controls it displays.

### Manual Structured Request

```bash
curl -fsS -X POST "http://127.0.0.1:8080/api/runs" \
  -H "Content-Type: application/json" \
  -H "X-MealCheck-Invite-Token: ${MEALCHECK_ACCESS_CODE}" \
  --data '{
    "input_mode": "manual_structured",
    "profile": {
      "age": 35,
      "sex": "male",
      "height_cm": 178,
      "weight_kg": 82,
      "activity_level": "moderate",
      "goal": "maintain_weight",
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "constraints": {
      "days": 1,
      "meals_per_day": 3,
      "allergies": ["peanuts"],
      "excluded_foods": [],
      "diet_pattern": "general",
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_shopping_list": true,
      "requires_prep_safety_notes": true
    },
    "candidate_plan": {
      "schema_version": "0.1",
      "plan_id": "manual-example",
      "description": "One day manually entered plan.",
      "days": [
        {
          "day": 1,
          "meals": [
            {
              "name": "breakfast",
              "items": [
                { "food": "cooked oatmeal", "quantity": 1, "unit": "cup" }
              ]
            }
          ]
        }
      ],
      "shopping_list": [],
      "prep_notes": []
    }
  }'
```

Manual plan validation rules:

- `candidate_plan.schema_version` must be `0.1`.
- `candidate_plan.plan_id` is required.
- At least one day, meal, and item is required.
- Each item must have `food`.
- Quantified items must have a positive `quantity` and one of these units:
  `g`, `oz`, `cup`, `tbsp`, `tsp`, `serving`.
- Unquantified items must include `quantity_text`,
  `resolution_status: "unresolved"`, and `unresolved_reason`.

### BYOK Profile Generation Request

```bash
curl -fsS -X POST "http://127.0.0.1:8080/api/runs" \
  -H "Content-Type: application/json" \
  -H "X-MealCheck-Invite-Token: ${MEALCHECK_ACCESS_CODE}" \
  --data '{
    "input_mode": "profile_generation",
    "profile": {
      "age": 35,
      "sex": "male",
      "height_cm": 178,
      "weight_kg": 82,
      "activity_level": "moderate",
      "goal": "maintain_weight",
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "constraints": {
      "days": 3,
      "meals_per_day": 3,
      "allergies": ["peanuts"],
      "excluded_foods": ["shellfish"],
      "diet_pattern": "general",
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_shopping_list": true,
      "requires_prep_safety_notes": true
    },
    "provider": {
      "type": "openai",
      "model": "gpt-example",
      "api_key": "user-supplied-key"
    },
    "repair_json": true
  }'
```

Generation rules:

- `provider.type` must be one of `openai`, `anthropic`, `gemini`, or
  `openai_compatible`.
- `provider.model` and `provider.api_key` are required for all BYOK providers.
- Native provider types use official provider endpoints. `provider.base_url` is
  accepted only for `openai_compatible` and defaults to
  `https://api.openai.com/v1`.
- `openai_compatible` sends the supplied API key to `provider.base_url`; use
  only endpoints you trust.
- OpenAI and OpenAI-compatible requests call `/chat/completions` with
  `response_format: { "type": "json_object" }`; Anthropic requests call
  `/v1/messages`; Gemini requests call `/v1beta/models/{model}:generateContent`.
- `repair_json` defaults to `true` for generation modes and allows one bounded
  repair attempt when provider output is not valid normalized meal-plan JSON.

### BYOK Prompt Generation Request

```json
{
  "input_mode": "prompt_generation",
  "profile": {
    "age": 35,
    "sex": "male",
    "height_cm": 178,
    "weight_kg": 82,
    "activity_level": "moderate",
    "goal": "maintain_weight",
    "calorie_target_kcal": 2000,
    "protein_target_g": 98
  },
  "constraints": {
    "days": 3,
    "meals_per_day": 3,
    "allergies": ["peanuts"],
    "excluded_foods": ["shellfish"],
    "diet_pattern": "general",
    "max_sodium_mg_per_day": 2300,
    "max_added_sugar_g_per_meal": 10,
    "max_saturated_fat_pct_calories": 10,
    "calorie_tolerance_pct": 15,
    "requires_shopping_list": true,
    "requires_prep_safety_notes": true
  },
  "generation_prompt": "Create a simple 3 day high-protein meal plan.",
  "provider": {
    "type": "gemini",
    "model": "gemini-example",
    "api_key": "user-supplied-key"
  },
  "repair_json": true
}
```

`generation_prompt` is required for `prompt_generation`.

### Checked-In Case Request

This mode exists for developer smoke tests and checked-in examples:

```json
{
  "case_path": "examples/seeded-3-day-peanut-allergy/case.json"
}
```

`case_path` must be relative and must currently point inside `examples/`.

## Read Run Status

```bash
curl -fsS "http://127.0.0.1:8080/api/runs/run_8d8c75f9a22a4be7f4533e73"
```

Example queued response:

```json
{
  "run": {
    "id": "run_8d8c75f9a22a4be7f4533e73",
    "case_path": ".mealcheck-data/runtime-cases/run_8d8c75f9a22a4be7f4533e73/case.json",
    "status": "queued",
    "created_at": "2026-06-11T15:04:05Z",
    "updated_at": "2026-06-11T15:04:05Z",
    "expires_at": "2026-06-18T15:04:05Z"
  },
  "links": {
    "self": "/api/runs/run_8d8c75f9a22a4be7f4533e73",
    "events": "/api/runs/run_8d8c75f9a22a4be7f4533e73/events",
    "report": "/api/runs/run_8d8c75f9a22a4be7f4533e73/report",
    "artifacts": "/api/runs/run_8d8c75f9a22a4be7f4533e73/artifacts"
  }
}
```

Completed runs include `decision`, `risk_level`, `summary`, and
`completed_at`. Failed runs include `error`.

## Read Run Events

```bash
curl -fsS "http://127.0.0.1:8080/api/runs/run_8d8c75f9a22a4be7f4533e73/events"
```

The response content type is `text/event-stream`. Current implementation
returns the events available at request time; clients can reconnect with
`?after=<last_event_id>`.

Example event:

```text
id: 1
event: queued
data: {"id":1,"run_id":"run_8d8c75f9a22a4be7f4533e73","type":"queued","message":"run queued","created_at":"2026-06-11T15:04:05Z"}
```

## Reports And Artifacts

`GET /api/runs/{run_id}/report` serves `report.json` from the run artifact
bundle. It is available after the run completes.

`GET /api/runs/{run_id}/artifacts` returns:

```json
{
  "run_id": "run_8d8c75f9a22a4be7f4533e73",
  "artifacts": [
    {
      "path": "decision.json",
      "url": "/api/runs/run_8d8c75f9a22a4be7f4533e73/artifacts/decision.json",
      "type": "json"
    }
  ]
}
```

Artifact paths are restricted to the run bundle. Absolute paths and `..` path
traversal are rejected.

## Demo Runs

Demo endpoints serve prebuilt public artifacts from the static frontend bundle.
They do not enqueue backend work and do not require an access code.

```bash
curl -fsS "http://127.0.0.1:8080/api/demo-runs"
```

Example shape:

```json
{
  "schema_version": "0.1",
  "demo_runs": [
    {
      "id": "seeded-3-day-peanut-allergy",
      "title": "Seeded 3-day peanut allergy report",
      "summary": "Example report with blocking findings.",
      "base_path": "demo-runs/seeded-3-day-peanut-allergy",
      "links": {
        "self": "/api/demo-runs/seeded-3-day-peanut-allergy",
        "report": "/api/demo-runs/seeded-3-day-peanut-allergy/report",
        "artifacts": "/api/demo-runs/seeded-3-day-peanut-allergy/artifacts"
      }
    }
  ]
}
```

## Health

```bash
curl -fsS "http://127.0.0.1:8080/api/health"
```

Example response:

```json
{
  "status": "ok",
  "store": "postgres",
  "queued_runs": 0,
  "running_runs": 0,
  "queue_size": 3,
  "active_run_limit": 1,
  "retention_days": 7
}
```

## Delete A Run

```bash
curl -fsS -X DELETE "http://127.0.0.1:8080/api/runs/run_8d8c75f9a22a4be7f4533e73"
```

Example response:

```json
{
  "status": "deleted"
}
```

Deletion marks the run deleted in the store, removes any pending in-memory
input, and deletes the run artifact directory. Deleted runs are no longer
returned by status or artifact endpoints.

## Error Responses

Errors use a consistent JSON envelope:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "constraints days must be between 1 and 7",
    "request_id": "req_8d8c75f9a22a4be7f4533e73"
  }
}
```

Representative error codes:

| Code | Typical Status | Cause |
|---|---:|---|
| `invalid_request` | `400` | Invalid JSON, unknown field, oversized body, invalid mode, bad profile, bad constraints, or invalid plan. |
| `unauthorized` | `401` | Live run creation requires a valid access code. |
| `invite_limit_reached` | `429` | The access code has reached its configured run limit. |
| `not_found` | `404` | Run, demo run, report, or artifact does not exist. |
| `method_not_allowed` | `405` | HTTP method is not supported on the route. |
| `queue_full` | `429` | The live run queue is at capacity. |
| `store_error` | `500` | Backend store operation failed. |
| `artifact_error` | `500` | Artifact file could not be read. |

## CORS

When `MEALCHECK_ALLOWED_ORIGIN` is set, the server only emits
`Access-Control-Allow-Origin` for an exact matching request `Origin`. Allowed
headers are:

```text
Content-Type, X-MealCheck-Invite-Token, X-Request-ID
```

Allowed methods are:

```text
GET, POST, DELETE, OPTIONS
```

All `OPTIONS` requests return `204 No Content`.

## Runtime Limits

Default hosted limits:

| Setting | Default | Environment variable |
|---|---:|---|
| Active workers | `1` | fixed for MVP |
| Queue size | `3` | `MEALCHECK_QUEUE_SIZE` |
| Request body limit | `1,000,000` bytes | `MEALCHECK_MAX_UPLOAD_BYTES` |
| Run timeout | `10m` | `MEALCHECK_RUN_TIMEOUT` |
| Retention | `7 days` | `MEALCHECK_RETENTION` |
| Worker poll interval | `1s` | `MEALCHECK_WORKER_POLL` |
| Cleanup interval | `1h` | `MEALCHECK_CLEANUP_INTERVAL` |

The API is designed around bounded hosted use on a small server: one active run,
a short queue, explicit retention, and user-supplied provider cost.

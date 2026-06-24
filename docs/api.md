# API

MealCheck exposes a small hosted API for public demo reports and live meal-plan
checks. The frontend should treat this API as asynchronous:
creating a run queues work, and separate endpoints expose status, events,
reports, and artifacts.

The API is intended for a static frontend plus a small self-hosted backend. It
can run either a server-owned local model or a BYOK model-provider path,
depending on deployment configuration.

## Access Mode

`GET` endpoints are currently public. `DELETE /api/runs/{run_id}` is also
public in the MVP API and treats the unguessable run id as the deletion
capability.

The hosted MVP supports two access modes:

- `public_byok`: live qualification and run creation do not require an access
  code. Abuse is bounded by request-rate, queue, body-size, text-length, daily
  run, timeout, and retention policies. This is the recommended hosted
  `mealcheck.dev` shape.
- `invite_required`: live qualification and run creation require an access
  code. This remains available for private or self-hosted deployments.

Set `MEALCHECK_ACCESS_MODE=public_byok` or
`MEALCHECK_ACCESS_MODE=invite_required`. If unset, the server uses
`invite_required` when `MEALCHECK_INVITE_TOKEN` or
`MEALCHECK_INVITE_REQUIRED=true` is configured; otherwise it uses
`public_byok`.

Per-user access codes are created by an operator with `mealcheck invite create`;
the full code is shown once, while the backend stores only the secret hash and
usage metadata.

```http
X-MealCheck-Invite-Token: <access-code>
```

The legacy `MEALCHECK_INVITE_TOKEN` environment variable is still supported as
a shared access-code fallback during local migration. Private deployments that
need access codes should prefer per-user access codes so individual reviewers
can have expiry, revocation, and run limits.

The server always adds an `X-Request-ID` response header. A client may send its
own `X-Request-ID`; otherwise, the server assigns one.

## Hosted Mode

`MEALCHECK_HOSTED_MODE` controls the live model-backed product shape:

- `local_model`: hosted live checks use the server-owned llama.cpp endpoint
  configured with `MEALCHECK_LOCAL_MODEL_*`. Clients submit meal-plan text but
  do not submit provider credentials or local endpoint URLs.
- `byok`: hosted live checks use client-supplied BYOK provider credentials for
  qualification normalization and generation.

If unset, hosted mode defaults to `byok` for compatibility. Access mode and
hosted mode are separate: `MEALCHECK_ACCESS_MODE=public_byok` still means the
public policy gate is active, even when `MEALCHECK_HOSTED_MODE=local_model`.

Provider API keys are supplied only on BYOK qualification and live
generation requests. Treat them as one-run bearer secrets: the browser sends the
key to the MealCheck backend, the backend uses it only for the requested
provider call, and MealCheck does not persist provider keys to run metadata,
reports, logs, metrics, runtime case files, or artifact bundles. Hosted BYOK
users should use temporary, scoped, budget-limited, revocable keys; for maximum
control, run MealCheck locally from the repository and submit requests to the
local backend.

In `public_byok` mode, hosted `openai_compatible` custom endpoints are disabled
unless `MEALCHECK_PUBLIC_OPENAI_COMPATIBLE=true`. Even when enabled, public mode
rejects localhost, private IP, link-local, non-HTTPS, and non-default-port
custom endpoint URLs. Native OpenAI, Anthropic, and Gemini providers remain
available.

## Runtime Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/health` | Service health, queue, store, and retention snapshot. |
| `GET` | `/api/status` | Public user-visible service status summary. |
| `GET` | `/api/demo-runs` | List static public demo reports. |
| `GET` | `/api/demo-runs/{demo_id}` | Read static demo metadata and decision summary. |
| `GET` | `/api/demo-runs/{demo_id}/report` | Fetch a static demo `report.json`. |
| `GET` | `/api/demo-runs/{demo_id}/artifacts` | List static demo artifacts. |
| `GET` | `/api/demo-runs/{demo_id}/artifacts/{path}` | Fetch one static demo artifact. |
| `POST` | `/api/qualify` | Classify candidate text and optionally normalize it with the configured hosted model path. |
| `POST` | `/api/runs` | Queue a live meal-plan check. |
| `GET` | `/api/runs/{run_id}` | Read live run status and summary. |
| `GET` | `/api/runs/{run_id}/events` | Read live run events as an SSE stream. |
| `GET` | `/api/runs/{run_id}/report` | Fetch completed live run `report.json`. |
| `GET` | `/api/runs/{run_id}/artifacts` | List completed live run artifacts. |
| `GET` | `/api/runs/{run_id}/artifacts/{path}` | Fetch one completed live run artifact. |
| `DELETE` | `/api/runs/{run_id}` | Delete live run metadata, pending input, and artifacts. |

## Public Status

`GET /api/status` is the public status-page contract. It summarizes
user-visible capabilities and intentionally does not expose raw diagnostics
such as queue depth, store implementation, model filename, hostnames, paths,
policy limits, logs, or recent user input.

`GET /api/health` remains the lower-level runtime diagnostic endpoint for
operators and frontend bootstrapping. Use `/api/status` when the caller needs to
answer whether MealCheck is usable right now.

Status states:

| State | Meaning |
|---|---|
| `operational` | The capability is available. |
| `degraded_performance` | The capability is available but may be slower or temporarily capacity-limited. |
| `partial_outage` | Some user workflows that depend on the capability may fail. |
| `major_outage` | The capability is unavailable. |
| `maintenance` | The capability is intentionally unavailable for maintenance. |
| `unknown` | The service cannot determine the capability status. |

Example response:

```json
{
  "schema_version": "0.1",
  "generated_at": "2026-06-24T12:42:00Z",
  "overall": {
    "state": "operational",
    "message": "All systems operational"
  },
  "components": [
    {
      "id": "meal_check_submission",
      "name": "Meal Check Submission",
      "state": "operational"
    },
    {
      "id": "ai_meal_normalization",
      "name": "AI Meal Normalization",
      "state": "operational"
    },
    {
      "id": "nutrition_allergen_checking",
      "name": "Nutrition & Allergen Checking",
      "state": "operational"
    },
    {
      "id": "report_generation",
      "name": "Report Generation",
      "state": "operational"
    },
    {
      "id": "sample_report",
      "name": "Sample Report",
      "state": "operational"
    }
  ],
  "recent_incidents": [],
  "links": {
    "sample_report": "/api/demo-runs/seeded-3-day-peanut-allergy/report"
  }
}
```

## Qualify Candidate Meal Plan Text

`POST /api/qualify` is the hosted preflight endpoint for pasted candidate text.
It is synchronous and policy-limited. It answers whether the text is a meal plan
eligible for verification, and it can use the configured hosted model path to
normalize detailed meal-plan text into MealCheck JSON.

`provider` may be omitted when the text is already normalized MealCheck JSON or
can be rejected deterministically as not eligible. `provider.model` and
`provider.api_key` are required in `byok` hosted mode when the text needs BYOK
normalization. In `local_model` hosted mode, omit `provider`; the backend uses
the server-owned local model and configured local-model limits.

```bash
curl -fsS -X POST "http://127.0.0.1:8080/api/qualify" \
  -H "Content-Type: application/json" \
  --data '{
    "text": "Day 1 breakfast: 1 cup cooked oatmeal and 1 banana.",
    "settings": {
      "nutrition_targets": {
        "calorie_target_kcal": 2000,
        "protein_target_g": 98
      },
      "verification_constraints": {
        "days": 1,
        "meals_per_day": 1,
        "allergies": ["peanuts"],
        "excluded_foods": [],
        "max_sodium_mg_per_day": 2300,
        "max_added_sugar_g_per_meal": 10,
        "max_saturated_fat_pct_calories": 10,
        "calorie_tolerance_pct": 15,
        "requires_prep_safety_notes": true
      }
    },
    "provider": {
      "type": "gemini",
      "model": "gemini-example",
      "api_key": "user-supplied-key"
    }
  }'
```

Example response:

```json
{
  "qualification": {
    "schema_version": "0.1",
    "status": "eligible_for_verification",
    "reason": "text was normalized into a MealCheck meal plan",
    "provider_used": true,
    "normalized_plan": {
      "schema_version": "0.1",
      "plan_id": "normalized-from-text",
      "days": [
        {
          "day": 1,
          "meals": [
            {
              "name": "breakfast",
              "items": [
                {
                  "food": "cooked oatmeal",
                  "quantity": 1,
                  "unit": "cup"
                }
              ]
            }
          ]
        }
      ]
    }
  }
}
```

Qualification statuses:

| Status | Meaning |
|---|---|
| `not_meal_plan` | The text is not asking for or describing meals. |
| `meal_plan_too_vague` | The text has meals or menu ideas but lacks ingredient quantities or units. |
| `recipe_or_menu_needs_decomposition` | The text is recipe-like or menu-like and needs ingredient decomposition before verification. |
| `eligible_for_verification` | The text is already normalized MealCheck JSON or was normalized successfully. |
| `eligible_with_unresolved_items` | The normalized plan can be checked while preserving explicit unresolved items. |

Qualification is not guideline verification. When `normalized_plan` is present,
the client can submit that plan through a local CLI/debug workflow or use a
generation flow that produces a run artifact for verification.

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

`POST /api/runs` supports four hosted request shapes.

| Mode | Fields | LLM Used | Purpose |
|---|---|---:|---|
| Checked-in case | `case_path` | no | Developer/demo compatibility for checked-in examples. |
| `local_model` | `input_mode`, `settings`, `candidate_text` | yes | Normalize pasted meal-plan text with the server-owned local model, then verify it. |
| `profile_generation` | `input_mode`, `settings`, `provider` | yes | Ask a provider to generate a plan from nutrition targets and verification constraints. |
| `prompt_generation` | `input_mode`, `settings`, `generation_prompt`, `provider` | yes | Ask a provider to generate a plan from a user prompt plus nutrition targets and verification constraints. |

`case_path` cannot be combined with `input_mode`. Hosted live requests should
use `local_model` in hosted local-model deployments, or `profile_generation`
and `prompt_generation` in hosted BYOK deployments. Structured JSON entry is
preserved in the local CLI/debug workflow, not the hosted `/api/runs` endpoint.

### Settings

```json
{
  "nutrition_targets": {
    "calorie_target_kcal": 2000,
    "protein_target_g": 98
  },
  "verification_constraints": {
    "days": 3,
    "meals_per_day": 3,
    "allergies": ["peanuts"],
    "excluded_foods": ["shellfish"],
    "max_sodium_mg_per_day": 2300,
    "max_added_sugar_g_per_meal": 10,
    "max_saturated_fat_pct_calories": 10,
    "calorie_tolerance_pct": 15,
    "requires_prep_safety_notes": true
  }
}
```

Validation rules:

- `nutrition_targets.calorie_target_kcal` must be positive.
- `nutrition_targets.protein_target_g` must be positive.
- `days` must be between `1` and `7`.
- `meals_per_day` must be between `1` and `6`.
- Hosted requests no longer accept demographic profile fields, diet pattern, or
  shopping-list switches. Use the `settings` object above for all BYOK and
  qualification requests.

### Local Structured JSON Verification

Hosted `/api/runs` no longer accepts `input_mode: "manual_structured"`.
Structured JSON verification is still supported by the CLI and local/debug case
file workflow. The same normalized plan validation rules apply there:

- `candidate_plan.schema_version` must be `0.1`.
- `candidate_plan.plan_id` is required.
- At least one day, meal, and item is required.
- Each item must have `food`.
- Quantified items must have a positive `quantity` and one of these units:
  `g`, `oz`, `cup`, `tbsp`, `tsp`, `serving`.
- Unquantified items must include `quantity_text`,
  `resolution_status: "unresolved"`, and `unresolved_reason`.

### Hosted Local Model Run Request

`input_mode: "local_model"` queues a live check that normalizes pasted
meal-plan text through the server-owned local model. The compact local contract
supports up to seven days and up to six meals per day by asking the model for
rows in the form `[source_item_id, day, meal_code, food, quantity, unit]`. The
backend numbers resolved source item lines before prompting the model, then
rejects outputs with missing or duplicated source item IDs.

When the text has clear `Day N` sections for every requested day, the backend
splits the input into per-day extraction calls before it reaches the local
model. Each day is normalized with a smaller one-day prompt, then the backend
restores the original day number and merges the days into one canonical
MealCheck plan. Ambiguous day boundaries fall back to the whole-plan compact
contract.

For best results, multi-day `candidate_text` should label every requested day
explicitly and keep foods, quantities, and units under the matching day:

```text
Day 1 breakfast: 1 cup cooked oatmeal, 0.5 cup blueberries, and 1 cup plain Greek yogurt.
Day 1 lunch: 4 oz grilled chicken breast, 1 cup brown rice, and 1 cup steamed broccoli.
Day 2 breakfast: 2 eggs, 1 cup whole wheat toast, and 1 cup orange segments.
Day 2 lunch: 4 oz tuna, 2 cups mixed greens, and 1 tsp vinaigrette.
```

The unbatched fallback is intentionally preserved for clients that send
acceptable but less regular meal-plan text.

Clients must omit `provider`; the backend injects the configured local model
endpoint and model id.

```json
{
  "input_mode": "local_model",
  "candidate_text": "Day 1 breakfast: 1 cup cooked oatmeal, 0.5 cup blueberries, and 1 cup plain Greek yogurt.\nDay 1 lunch: 4 oz grilled chicken breast, 1 cup brown rice, and 1 cup steamed broccoli.\nDay 1 dinner: 4 oz baked salmon, 1 serving sweet potato, and 1 tbsp olive oil.\nDay 2 breakfast: 2 eggs, 1 cup whole wheat toast, and 1 cup orange segments.\nDay 2 lunch: 4 oz tuna, 2 cups mixed greens, and 1 tsp vinaigrette.\nDay 2 dinner: 5 oz turkey meatballs, 1 cup whole wheat pasta, and 1 cup tomato sauce.\nDay 3 breakfast: 1 cup cottage cheese, 1 serving pineapple, and 1 cup whole grain cereal.\nDay 3 lunch: 4 oz tofu, 1 cup soba noodles, and 1 cup bok choy.\nDay 3 dinner: 5 oz lean beef, 1 cup roasted carrots, and 1 cup barley.",
  "settings": {
    "nutrition_targets": {
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "verification_constraints": {
      "days": 3,
      "meals_per_day": 3,
      "allergies": ["peanuts"],
      "excluded_foods": ["shellfish"],
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_prep_safety_notes": false
    }
  }
}
```

Rules:

- `MEALCHECK_LOCAL_MODEL_ENABLED=true` must be configured.
- `MEALCHECK_LOCAL_MODEL_BASE_URL` must point to the private llama.cpp
  OpenAI-compatible API, normally `http://127.0.0.1:11435/v1`.
- `MEALCHECK_LOCAL_MODEL_NAME` must match the loaded llama.cpp model id.
- `candidate_text` is bounded by `MEALCHECK_LOCAL_MODEL_MAX_INPUT_CHARS`.
- Local model output uses compact meal codes: `b` breakfast, `m` morning snack,
  `l` lunch, `a` afternoon snack, `d` dinner, `s` snack, and `e` evening snack.
  The backend expands these rows into canonical MealCheck JSON before
  verification.
- `provider` is rejected in `local_model` requests.

Before queueing a `local_model` run, the backend applies a deterministic
meal-plan qualification preflight. Obvious non-meal text, meal-like text without
quantities or units, and recipe-like text that has not been decomposed into
day/meal/ingredient rows return `422` and do not call the model:

```json
{
  "error": {
    "code": "meal_plan_not_verifiable",
    "message": "The text does not describe days, meals, recipes, or ingredient-level meal-plan content.",
    "details": {
      "qualification": {
        "schema_version": "0.1",
        "status": "not_meal_plan",
        "reason": "The text does not describe days, meals, recipes, or ingredient-level meal-plan content.",
        "missing_fields": ["meal_plan_content"],
        "provider_used": false
      }
    }
  }
}
```

If local-model normalization fails after preflight, the run may still fail, but
the public run error is phrased as user guidance. Sanitized parser and model
details are preserved in `debug/normalization-failure.json` for operators.

### BYOK Targets Generation Request

```bash
curl -fsS -X POST "http://127.0.0.1:8080/api/runs" \
  -H "Content-Type: application/json" \
  --data '{
    "input_mode": "profile_generation",
    "settings": {
      "nutrition_targets": {
        "calorie_target_kcal": 2000,
        "protein_target_g": 98
      },
      "verification_constraints": {
        "days": 3,
        "meals_per_day": 3,
        "allergies": ["peanuts"],
        "excluded_foods": ["shellfish"],
        "max_sodium_mg_per_day": 2300,
        "max_added_sugar_g_per_meal": 10,
        "max_saturated_fat_pct_calories": 10,
        "calorie_tolerance_pct": 15,
        "requires_prep_safety_notes": true
      }
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
  "settings": {
    "nutrition_targets": {
      "calorie_target_kcal": 2000,
      "protein_target_g": 98
    },
    "verification_constraints": {
      "days": 3,
      "meals_per_day": 3,
      "allergies": ["peanuts"],
      "excluded_foods": ["shellfish"],
      "max_sodium_mg_per_day": 2300,
      "max_added_sugar_g_per_meal": 10,
      "max_saturated_fat_pct_calories": 10,
      "calorie_tolerance_pct": 15,
      "requires_prep_safety_notes": true
    }
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

Demo endpoints serve prebuilt repository artifacts for developer compatibility
and seeded proof inspection. The hosted frontend no longer loads these endpoints
as part of its primary website flow.
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
  "access_mode": "public_byok",
  "hosted_mode": "local_model",
  "queued_runs": 0,
  "running_runs": 0,
  "queue_size": 3,
  "active_run_limit": 1,
  "retention_days": 7,
  "public_openai_compatible": false,
  "max_candidate_text_chars": 20000,
  "max_generation_prompt_chars": 4000,
  "local_model": {
    "enabled": true,
    "ready": true,
    "model": "Qwen3-0.6B-Q4_K_M.gguf",
    "max_input_chars": 6000,
    "max_output_tokens": 1536,
    "timeout_sec": 240,
    "supported_days": 7,
    "supported_meals_per_day": 6
  },
  "policy": {
    "public_request_limit": 60,
    "public_request_window_sec": 60,
    "public_daily_run_limit": 20,
    "queue_size": 3,
    "active_run_limit": 1
  }
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
    "message": "settings verification_constraints days must be between 1 and 7",
    "request_id": "req_8d8c75f9a22a4be7f4533e73"
  }
}
```

Representative error codes:

| Code | Typical Status | Cause |
|---|---:|---|
| `invalid_request` | `400` | Invalid JSON, unknown field, oversized body, invalid mode, bad settings, or invalid plan. |
| `unauthorized` | `401` | Invite-required mode needs a valid access code. |
| `rate_limited` | `429` | Public request rate limit was exceeded. |
| `daily_run_limit_reached` | `429` | Public daily run limit was reached for the client. |
| `invite_limit_reached` | `429` | The access code has reached its configured run limit. |
| `not_found` | `404` | Run, demo run, report, or artifact does not exist. |
| `method_not_allowed` | `405` | HTTP method is not supported on the route. |
| `queue_full` | `429` | The live run queue is at capacity. |
| `provider_error` | `502` | A BYOK provider call failed or returned unusable output. |
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
| Public access mode | `public_byok` unless invite config is set | `MEALCHECK_ACCESS_MODE` |
| Public OpenAI-compatible endpoints | `false` | `MEALCHECK_PUBLIC_OPENAI_COMPATIBLE` |
| Public request limit | `60` per window | `MEALCHECK_PUBLIC_REQUEST_LIMIT` |
| Public request window | `1m` | `MEALCHECK_PUBLIC_REQUEST_WINDOW` |
| Public daily run limit | `20` | `MEALCHECK_PUBLIC_DAILY_RUN_LIMIT` |
| Candidate text limit | `20,000` characters | `MEALCHECK_MAX_CANDIDATE_TEXT_CHARS` |
| Generation prompt limit | `4,000` characters | `MEALCHECK_MAX_GENERATION_PROMPT_CHARS` |
| Run timeout | `10m` | `MEALCHECK_RUN_TIMEOUT` |
| Retention | `7 days` | `MEALCHECK_RETENTION` |
| Worker poll interval | `1s` | `MEALCHECK_WORKER_POLL` |
| Cleanup interval | `1h` | `MEALCHECK_CLEANUP_INTERVAL` |

The API is designed around bounded hosted use on a small server: one active run,
a short queue, explicit retention, and user-supplied provider cost.

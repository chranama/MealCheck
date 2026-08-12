# Privacy And Safety

This document defines MealCheck's MVP privacy and safety policy.

MealCheck handles health-adjacent meal-plan and settings data. It should
therefore minimize
collection, avoid unnecessary retention, and be explicit about when data leaves
the MealCheck server.

## Safety Boundary

MealCheck is a verifier, not a medical nutrition service.

MealCheck may say:

- a plan violates a declared allergy or food exclusion
- a plan exceeds a configured sodium, added sugar, saturated fat, calorie, or
  protein threshold
- a food or portion could not be resolved
- a plan is missing required structure
- a guideline-pack rule passed, warned, or blocked

MealCheck must not say:

- a plan is medically appropriate for a person
- a plan treats, prevents, or manages a disease
- a plan will cause weight loss or improve health outcomes
- a plan is safe for pregnancy, pediatrics, diabetes, kidney disease, eating
  disorders, or other clinical contexts
- a plan is allergen-safe in the clinical sense

Reports should include a short non-medical-use disclaimer.

## Data Categories

MealCheck may receive:

- hosted nutrition targets: calorie target and protein target
- verification constraints: days, meals per day, allergies, excluded foods,
  nutrient limits, calorie tolerance, and prep-safety requirement
- meal-plan data: foods, quantities, units, prep notes, and shopping lists
- prompts: custom user prompts for prompt-based generation in BYOK/API/local
  deployments
- provider credentials: model API keys supplied for BYOK runs outside the
  hosted local-model website flow
- operational metadata: run ID, status, timestamps, decision, and artifact paths

MealCheck should not request:

- name
- email unless auth requires it
- address
- precise location
- diagnosis
- medications
- lab results
- pregnancy status
- eating-disorder history
- payment card data
- insurance information

If a user enters unsupported clinical data anyway, MealCheck should not use it
to create clinical guidance.

## Data Minimization

MVP collection should be limited to fields needed for the selected check.

Default rules:

- Do not require accounts for seeded public demos.
- Do not require a name or email for manual local runs.
- Do not store provider API keys.
- Do not store settings fields in separate database columns unless needed for
  queueing, filtering, or deletion.
- Prefer keeping settings and plan details inside the run artifact
  bundle rather than duplicating them in operational tables.
- Redact secrets from every persisted config and artifact.

## Third-Party Disclosure

Hosted local-model verification sends pasted meal-plan text and settings only
to the private llama.cpp service running on the MealCheck backend host. It does
not ask the user for model-provider credentials and does not send data to
OpenAI, Anthropic, Gemini, or custom model endpoints.

Local structured case-file verification can run without sending settings or
plan data to an LLM provider.

BYOK LLM modes in repo/API/local or self-hosted workflows send data to the
user's selected provider:

- qualification normalization sends pasted candidate meal-plan text, nutrition
  targets, and verification constraints when a provider is needed
- targets-only generation sends nutrition targets and verification constraints
- prompt-based generation sends nutrition targets, verification constraints,
  and the custom prompt
- bounded repair sends invalid model output, schema errors, and enough context
  to repair JSON shape

The MVP provider choices are OpenAI, Anthropic, Gemini, and custom
OpenAI-compatible endpoints.

Any BYOK UI or API client must disclose this before a BYOK qualification or
generation request starts.

MealCheck should not send user settings, prompt, or meal-plan data to
analytics, advertising, or tracking services.

The production frontend at `mealcheck.dev` loads the standard Cloudflare Web
Analytics beacon for page and browser performance measurement. The beacon is
installed only on that exact hostname, is not loaded by local or preview
deployments, and has no application-data or interaction-event integration.
MealCheck does not provide the beacon with settings, prompts, meal plans,
provider credentials, access codes, or form values.

## Secrets

Provider API keys:

- accepted only for BYOK qualification normalization, generation, or bounded
  repair
- treated as one-run bearer secrets, not saved account settings
- sent from the browser to the MealCheck backend and then to the selected
  provider endpoint for that request
- held only in short-lived backend memory while the qualification request is
  active or while the run is queued or active
- never written to Postgres, logs, reports, metrics, artifact bundles, or
  persisted configs
- not persisted in browser `localStorage` or `sessionStorage`
- discarded after qualification returns, when the worker claims the run, the run
  is deleted, the pending input expires, or cleanup removes expired state
- expected to be temporary, scoped, budget-limited, and revocable

Hosted BYOK requires trusting the MealCheck backend process and deployment
operator because the key briefly exists in request memory and process memory.
Users who want the strongest key-control posture should run MealCheck locally
from the repository and submit BYOK requests to their local backend. Custom
OpenAI-compatible endpoints receive the supplied key and should be used only
when the endpoint operator is trusted.

FoodData Central API keys, if added later, are server credentials and must not
be embedded in frontend code or user-visible artifacts.

Public BYOK policy controls:

- gate public hosted use through hard limits rather than trust alone
- limit request rate by client IP
- limit daily live-run creation by client IP
- enforce queue, active-run, body-size, candidate-text, generation-prompt,
  timeout, repair-attempt, artifact-retention, and cleanup limits
- disable public hosted `openai_compatible` custom endpoints by default
- reject public custom endpoint URLs that target localhost, private IPs,
  link-local IPs, non-HTTPS schemes, or non-default HTTPS ports
- return `429` with `Retry-After` when policy capacity is exceeded

Access codes remain available for private deployments:

- gate live run creation for invited reviewers or beta users
- are bearer credentials, not user accounts or proof of identity
- should be generated per reviewer with expiry and optional run limits
- must be stored as hashes, not full codes
- may keep a short operator label such as `reviewer-chris`, but should avoid
  collecting email or other personal identifiers unless they become necessary
- should be revocable without affecting other reviewers
- must not be written to logs, reports, artifacts, frontend config, or
  screenshots

## Logging

Application logs should include:

- request ID
- route
- status code
- run ID
- coarse event names
- duration
- error code

Application logs should not include:

- provider API keys
- settings payloads
- custom prompts
- meal-plan contents
- normalized-plan JSON
- allergy lists
- access codes
- database URLs
- tunnel credentials
- admin tokens

Errors returned to users should be actionable but should not echo sensitive
input unless it is already visible in the user's current report view.

## Retention

Default hosted retention:

- seeded demo reports: retained until replaced by a new seeded artifact set
- live local-model run metadata: 7 days
- live local-model artifacts: 7 days
- logs: shortest practical operational window; target 7 to 14 days
- provider credentials: never persisted

Expired artifacts and metadata should be deleted by a cleanup job.

The implementation should support user-triggered deletion for public live runs.

## Report Visibility

Default visibility:

- seeded demo reports are public
- live local-model reports are private
- shareable live reports require an explicit share action

Shared reports should not expose provider keys, admin metadata, internal file
paths, database IDs beyond the public run ID, or unredacted configs.

Before sharing, the UI should remind the user that nutrition targets,
verification constraints, foods, and prompts may be visible in the report.

## Access Control

Public visitors may:

- view seeded reports
- download safe seeded artifacts
- inspect source citations and guideline-pack IDs

Public visitors may not:

- trigger maintainer-paid model calls
- access private live-run artifacts
- view user-provided API keys
- access admin endpoints

Public hosted live local-model runs may operate without access codes when
policy gates are enabled. Private or self-hosted deployments may still require
a per-user access code or stronger auth gate. Admin operations should require a
separate admin credential.

## Legal And Compliance Notes

MealCheck should not present itself as HIPAA-compliant.

The MVP is not intended to be a covered entity, business associate, medical
device, dietitian service, or clinical decision-support tool. If the project
later moves toward clinical use, account systems, persistent health profiles,
children, disease-specific guidance, or provider integrations, the privacy and
regulatory posture must be re-evaluated before implementation.

Health-adjacent consumer data may still be sensitive even when HIPAA does not
apply. MealCheck should follow data-minimization, truthful-disclosure, and
reasonable-security practices from the start.

## Abuse And Safety Cases

The service should refuse or block:

- pediatric meal-planning requests
- pregnancy or lactation meal-planning requests
- disease-specific diets
- eating-disorder related plans
- supplement or medication-interaction advice
- guaranteed weight-loss claims
- requests to override declared allergies
- requests to hide or ignore unresolved foods

The service may continue with a warning when:

- food matching is uncertain
- quantities are unresolved
- source data is missing added sugar or another nutrient
- a guideline domain is advisory rather than a hard check

## Implementation Requirements

Before public hosted live runs ship, implementation should include:

- non-medical-use disclaimer in the UI and report
- explicit hosted local-model disclosure and BYOK third-party disclosure for
  repo/API/local or self-hosted provider flows
- backend secret and artifact redaction tests
- log redaction policy or tests if structured request logging is added
- private-by-default live reports

Milestone 5 includes backend tests that exercise BYOK provider generation,
bounded repair, redacted provider artifacts, and absence of user API keys from
runtime files and artifact bundles.
- cleanup job for expired live runs
- user-triggered deletion for private runs
- admin-only access to queue and cleanup operations

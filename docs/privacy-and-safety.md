# Privacy And Safety

This document defines MealCheck's MVP privacy and safety policy.

MealCheck handles health-adjacent profile data. It should therefore minimize
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

- profile fields: age, sex, height, weight, activity level, and goal
- constraints: allergies, excluded foods, diet pattern, nutrient limits, and
  food preferences
- meal-plan data: foods, quantities, units, prep notes, and shopping lists
- prompts: custom user prompts for prompt-based generation
- provider credentials: model API keys supplied for BYOK runs
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
- Do not store profile fields in separate database columns unless needed for
  queueing, filtering, or deletion.
- Prefer keeping profile, constraints, and plan details inside the run artifact
  bundle rather than duplicating them in operational tables.
- Redact secrets from every persisted config and artifact.

## Third-Party Disclosure

Manual structured entry can run without sending profile or plan data to an LLM
provider.

BYOK LLM modes send data to the user's selected provider:

- profile-only generation sends profile and constraints
- prompt-based generation sends profile, constraints, and the custom prompt
- bounded repair sends invalid model output, schema errors, and enough context
  to repair JSON shape

The MVP provider choices are OpenAI, Anthropic, Gemini, and custom
OpenAI-compatible endpoints.

The UI must disclose this before a BYOK run starts.

MealCheck should not send user profile, prompt, or meal-plan data to analytics,
advertising, or tracking services.

## Secrets

Provider API keys:

- accepted only for BYOK generation or bounded repair
- held only in memory or short-lived encrypted job state if async execution
  requires it
- never written to Postgres, logs, reports, metrics, artifact bundles, or
  persisted configs
- discarded when the run completes, fails, expires, or is cancelled

FoodData Central API keys, if added later, are server credentials and must not
be embedded in frontend code or user-visible artifacts.

Access codes:

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
- profile payloads
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
- live BYOK run metadata: 7 days
- live BYOK artifacts: 7 days
- logs: shortest practical operational window; target 7 to 14 days
- provider credentials: never persisted

Expired artifacts and metadata should be deleted by a cleanup job.

The implementation should support user-triggered deletion before public live
BYOK access ships.

## Report Visibility

Default visibility:

- seeded demo reports are public
- live BYOK reports are private
- shareable live reports require an explicit share action

Shared reports should not expose provider keys, admin metadata, internal file
paths, database IDs beyond the public run ID, or unredacted configs.

Before sharing, the UI should remind the user that profile, constraints, foods,
and prompts may be visible in the report.

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

Live BYOK runs should require a per-user access code or stronger auth gate.
Admin operations should require a separate admin credential.

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
- explicit BYOK third-party disclosure
- backend secret and artifact redaction tests
- log redaction policy or tests if structured request logging is added
- private-by-default live reports

Milestone 5 includes backend tests that exercise BYOK provider generation,
bounded repair, redacted provider artifacts, and absence of user API keys from
runtime files and artifact bundles.
- cleanup job for expired live runs
- user-triggered deletion for private runs
- admin-only access to queue and cleanup operations

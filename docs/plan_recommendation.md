# Plan Recommendation

This document defines the principles for MealCheck's deterministic meal-plan
recommendation feature.

## Purpose

MealCheck may attempt to recommend a modified meal plan when a verified plan
returns `block` or `warn`. The recommendation should be an explicit edit of the
submitted plan, not a newly generated meal plan.

The feature exists to help the user understand a concrete path from a failed or
warning result to a plan that satisfies the configured checks.

## Core Principles

Recommendations must be deterministic. The backend must not call a model,
sample alternatives, or ask the model to repair a failed plan.

Recommendations must be verification-gated. A recommendation is available only
when MealCheck can re-run the modified plan through the deterministic checker
and the projected decision is `pass`.

Recommendations must be conservative. If the backend cannot make a bounded,
auditable edit, it should return `unavailable` with a clear reason.

Recommendations must be explicit modifications. The artifact should identify
the original item or note, the replacement or addition, the affected day and
meal when applicable, and the checks addressed by the edit.

Recommendations must not invent nutrition-critical details. The backend should
not guess missing quantities, unsupported units, missing meal structure, or
recipe decomposition.

Recommendations must preserve the source plan shape as much as possible. The
backend should prefer small substitutions or additions over broad plan rewrites.

Recommendations must remain advisory product output, not medical or nutritional
treatment advice. The checker can say a modified plan passes configured rules;
it should not claim that the plan is medically appropriate for a person.

## Availability Rule

The recommendation artifact has two valid states:

- `available`: a modified plan exists, the edit list is present, and the
  projected checker decision is `pass`.
- `unavailable`: no bounded deterministic recommendation is available.

Unavailable is a normal response. It is preferred over a weak recommendation.

MealCheck should return `unavailable` for:

- source plans that already pass
- missing required meal structure
- unresolved quantities or units
- unsupported foods where no safe catalog-backed replacement exists
- failed nutritional checks that cannot be corrected by a bounded deterministic
  rule
- any attempted edit whose projected checker decision remains `block` or `warn`

## Supported Edit Classes

Initial deterministic edit classes are intentionally small:

- add a leftover refrigeration prep-safety note when that is the only remaining
  safety-note issue
- replace allergen or excluded-food items with catalog-backed alternatives that
  avoid configured allergies and exclusions
- add a resolved vegetable item for missing daily vegetable coverage

Each edit must be expressed in the recommendation artifact as a structured
change with:

- operation
- affected day and meal, when applicable
- original item, when applicable
- replacement or added item, when applicable
- prep note, when applicable
- reason
- addressed check IDs

## Current Non-Goals

Do not generate a fresh meal plan.

Do not optimize macros, calories, sodium, saturated fat, or added sugar unless
the edit can be expressed as a bounded deterministic rule and re-verified to
`pass`.

Do not decompose recipes into ingredients.

Do not infer quantities from vague text such as `some`, `a bit`, `to taste`, or
`one bowl`.

Do not expand the user's stated diet, allergy, or exclusion constraints.

Do not use the local model or external provider endpoints to create the
recommendation.

## Artifact Contract

The backend writes `recommendation.json` into each artifact bundle.

Important fields:

- `schema_version`: recommendation artifact version.
- `status`: `available` or `unavailable`.
- `reason`: concise explanation of why a recommendation is or is not available.
- `source_decision`: original checker decision.
- `source_plan_id`: original plan identifier.
- `blocking_checks`: failed or warning checks when no recommendation is
  available.
- `changes`: structured edit list when a recommendation is available.
- `modified_plan`: canonical MealCheck meal-plan JSON after edits.
- `projected_decision`: checker decision document for the modified plan.

The `modified_plan` and `projected_decision` fields should be omitted when
`status` is `unavailable`.

## Trust Boundary

The recommendation artifact is downstream of verification. It should not weaken
the verifier by presenting plausible but unverified repairs.

The implementation order is:

1. Evaluate the submitted plan.
2. If the decision is `block` or `warn`, attempt supported deterministic edits.
3. Re-evaluate the modified plan.
4. Publish the recommendation only if the projected decision is `pass`.
5. Otherwise publish an unavailable recommendation with the remaining failed or
   warning checks.

This keeps the recommendation feature aligned with MealCheck's core product
claim: clear verification over generative meal planning.

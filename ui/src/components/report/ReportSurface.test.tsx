import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { decision, reportArtifacts } from "../../test/factories/report";
import { ReportSurface } from "./ReportSurface";

describe("ReportSurface", () => {
  it("shows source inspection for checks, food trace rows, and citations", () => {
    render(
      <ReportSurface
        activeTab="sources"
        artifacts={reportArtifacts({
          decision: decision({
            checks: [
              {
                check_id: "sodium_under_limit",
                status: "warn",
                message: "Sodium exceeds the configured daily limit.",
                affected_days: [1],
                source_refs: ["dga-sodium", "missing-source"],
              },
            ],
          }),
          citations: {
            sources: [
              {
                source_id: "dga-sodium",
                title: "Dietary Guidelines for Americans",
                publisher: "USDA and HHS",
                url: "https://www.dietaryguidelines.gov/",
                claims_used: [
                  {
                    claim_id: "dga-sodium-mg-per-day",
                    summary: "Limit sodium intake for adults.",
                    source_locator: "Chapter 1",
                  },
                ],
              },
            ],
          },
          unresolved: [
            {
              day: 1,
              meal: "breakfast",
              food: "berries",
              quantity_text: "a bowl",
              unresolved_reason: "vague_quantity",
            },
          ],
          normalizationReview: {
            schema_version: "0.1",
            run_id: "run-review",
            created_at: "2026-07-01T12:00:00Z",
            status: "confirmed",
            requires_confirmation: true,
            trust_signals: {
              source_item_count: 1,
              normalized_row_count: 1,
              unresolved_item_count: 1,
              repair_count: 0,
              failed_chunk_count: 0,
            },
            normalized_plan: {
              schema_version: "0.1",
              plan_id: "plan-review",
              description: "Review plan",
              days: [],
              shopping_list: [],
              prep_notes: [],
            },
            rows: [
              {
                day: 1,
                meal_code: "b",
                meal_label: "breakfast",
                source_item_id: 2,
                source_text: "a bowl berries",
                normalized_food: "berries",
                resolved: false,
                quantity_text: "a bowl",
                unresolved_reason: "vague_quantity",
              },
            ],
          },
        })}
        setActiveTab={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Source Inspection" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Check Source Trace" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Food Source Trace" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Missing Source References" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Guideline Citations" })).toBeVisible();
    expect(screen.getByText("Dietary Guidelines for Americans")).toBeVisible();
    expect(screen.getAllByText("missing-source").length).toBeGreaterThan(0);
    expect(screen.getByText("a bowl berries")).toBeVisible();
    expect(screen.getByText("a vague quantity")).toBeVisible();
    expect(screen.getByText("Add a measured quantity and unit.")).toBeVisible();
    expect(screen.getByText("Sodium daily limit")).toBeVisible();
  });

  it("shows available verified recommendation changes", () => {
    render(
      <ReportSurface
        activeTab="recommendation"
        artifacts={reportArtifacts({
          recommendation: {
            schema_version: "0.1",
            status: "available",
            reason: "A deterministic modified meal plan passed the configured checks.",
            source_decision: "block",
            source_plan_id: "plan-source",
            changes: [
              {
                operation: "replace_item",
                day: 1,
                meal: "lunch",
                from: { food: "peanut sauce", quantity: 1, unit: "tbsp" },
                to: { food: "olive oil", quantity: 1, unit: "tbsp" },
                reason: "Replace peanut sauce with a catalog item that avoids configured allergies and exclusions.",
                addresses_checks: ["allergens_absent"],
              },
            ],
            modified_plan: {
              schema_version: "0.1",
              plan_id: "plan-source-recommended",
              description: "Recommended plan",
              days: [],
              shopping_list: [],
              prep_notes: [],
            },
            projected_decision: {
              decision: "pass",
              risk_level: "low",
              summary: "Plan fits the configured checks.",
            },
          },
          normalizationReview: {
            schema_version: "0.1",
            run_id: "run-review",
            created_at: "2026-07-01T12:00:00Z",
            status: "confirmed",
            requires_confirmation: true,
            trust_signals: {
              source_item_count: 1,
              normalized_row_count: 1,
              unresolved_item_count: 0,
              repair_count: 0,
              failed_chunk_count: 0,
            },
            normalized_plan: {
              schema_version: "0.1",
              plan_id: "plan-source",
              description: "Review plan",
              days: [],
              shopping_list: [],
              prep_notes: [],
            },
            rows: [
              {
                day: 1,
                meal_code: "l",
                meal_label: "lunch",
                source_item_id: 9,
                source_text: "1 tbsp peanut sauce",
                normalized_food: "peanut sauce",
                resolved: true,
                quantity: 1,
                unit: "tbsp",
              },
            ],
          },
        })}
        setActiveTab={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Recommendation" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Verified Changes" })).toBeVisible();
    expect(screen.getAllByText("Available").length).toBeGreaterThan(0);
    expect(screen.getByText("Pass")).toBeVisible();
    expect(screen.getByText("Replace Item")).toBeVisible();
    expect(screen.getByText("1 tbsp peanut sauce")).toBeVisible();
    expect(screen.getByText("peanut sauce (1 tbsp)")).toBeVisible();
    expect(screen.getByText("olive oil (1 tbsp)")).toBeVisible();
    expect(screen.getByText("Declared allergens are absent")).toBeVisible();
  });

  it("shows unavailable recommendation reasons and blocking checks", () => {
    render(
      <ReportSurface
        activeTab="recommendation"
        artifacts={reportArtifacts({
          recommendation: {
            schema_version: "0.1",
            status: "unavailable",
            reason: "No deterministic recommendation is available because one or more food quantities or units are unresolved.",
            source_decision: "block",
            source_plan_id: "plan-source",
            blocking_checks: ["quantities_resolvable", "food_group_coverage"],
          },
        })}
        setActiveTab={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "No Verified Change" })).toBeVisible();
    expect(screen.getAllByText("Unavailable").length).toBeGreaterThan(0);
    expect(screen.getByText("No deterministic recommendation is available because one or more food quantities or units are unresolved.")).toBeVisible();
    expect(screen.getByText("Food quantities are clear")).toBeVisible();
    expect(screen.getByText("Food group coverage is present")).toBeVisible();
  });

  it("shows unresolved recovery actions and excluded unresolved foods", () => {
    render(
      <ReportSurface
        activeTab="foods"
        artifacts={reportArtifacts({
          unresolved: [
            {
              day: 1,
              meal: "lunch",
              food: "ham sandwich",
              unresolved_reason: "composed_food_needs_decomposition",
            },
          ],
          normalizationReview: {
            schema_version: "0.1",
            run_id: "run-review",
            created_at: "2026-07-01T12:00:00Z",
            status: "confirmed",
            requires_confirmation: true,
            trust_signals: {
              source_item_count: 1,
              normalized_row_count: 1,
              unresolved_item_count: 1,
              repair_count: 0,
              failed_chunk_count: 0,
            },
            normalized_plan: {
              schema_version: "0.1",
              plan_id: "plan-review",
              description: "Review plan",
              days: [],
              shopping_list: [],
              prep_notes: [],
            },
            rows: [
              {
                day: 1,
                meal_code: "l",
                meal_label: "lunch",
                source_item_id: 7,
                source_text: "ham sandwich",
                normalized_food: "ham sandwich",
                resolved: false,
                unresolved_reason: "composed_food_needs_decomposition",
              },
            ],
          },
          excludedUnresolved: [
            {
              day: 1,
              meal: "dinner",
              food: "sumac",
              quantity: 1,
              unit: "g",
              deterministic_grams: 1,
              unresolved_reason: "unknown_food",
              exclusion_reason: "de_minimis_unresolved_mass",
              policy_id: "de_minimis_unresolved_v1",
            },
          ],
        })}
        setActiveTab={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Unresolved Foods" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Unresolved Summary" })).toBeVisible();
    expect(screen.getAllByText("a mixed dish that needs ingredient details").length).toBeGreaterThan(0);
    expect(screen.getByText("Day 1 Lunch")).toBeVisible();
    expect(screen.getAllByText("Break this mixed dish into ingredients.").length).toBeGreaterThan(0);
    expect(screen.getAllByText("ham sandwich").length).toBeGreaterThan(0);
    expect(screen.getByRole("heading", { name: "Excluded From Totals" })).toBeVisible();
    expect(screen.getByText("sumac")).toBeVisible();
    expect(screen.getByText("de_minimis_unresolved_mass")).toBeVisible();
  });

  it("shows completed local-model normalization trace artifacts", () => {
    render(
      <ReportSurface
        activeTab="normalization"
        artifacts={reportArtifacts({
          normalizationReview: {
            schema_version: "0.1",
            run_id: "run-review",
            created_at: "2026-07-01T12:00:00Z",
            status: "confirmed",
            requires_confirmation: true,
            trust_signals: {
              source_item_count: 2,
              normalized_row_count: 2,
              unresolved_item_count: 1,
              repair_count: 1,
              failed_chunk_count: 0,
            },
            normalized_plan: {
              schema_version: "0.1",
              plan_id: "plan-review",
              description: "Review plan",
              days: [],
              shopping_list: [],
              prep_notes: [],
            },
            rows: [
              {
                day: 1,
                meal_code: "b",
                meal_label: "breakfast",
                source_item_id: 1,
                source_text: "1 cup oatmeal",
                source_parse_status: "resolved",
                normalized_food: "cooked oatmeal",
                resolved: true,
                quantity: 1,
                unit: "cup",
              },
              {
                day: 1,
                meal_code: "b",
                meal_label: "breakfast",
                source_item_id: 2,
                source_text: "a bowl berries",
                source_parse_status: "unresolved",
                normalized_food: "berries",
                resolved: false,
                quantity_text: "a bowl",
                unresolved_reason: "vague_quantity",
              },
            ],
          },
          localModelExtraction: {
            schema_version: "0.1",
            created_at: "2026-07-01T12:00:00Z",
            plan_id: "plan-review",
            chunk_count: 1,
            source_item_count: 2,
            stage_timings: { total_ms: 1200 },
            chunks: [
              {
                index: 0,
                day: 1,
                meal_code: "b",
                meal_label: "breakfast",
                meal_text: "Breakfast",
                source_items: [
                  { id: 1, day: 1, meal_code: "b", text: "1 cup oatmeal", parse_status: "resolved" },
                  { id: 2, day: 1, meal_code: "b", text: "a bowl berries", parse_status: "unresolved" },
                ],
                decoded_rows: [],
                reconciliation: {
                  repair_count: 1,
                  repairs: [
                    { source_item_id: 1, field: "unit", from: "cups", to: "cup", reason: "unit_alias" },
                  ],
                },
              },
            ],
          },
          normalizationEvents: [
            { type: "json_decoded", message: "provider output decoded as normalized meal-plan JSON", created_at: "2026-07-01T12:00:00Z" },
          ],
          reviewActions: [
            { schema_version: "0.1", run_id: "run-review", action: "confirmed", reason: "User confirmed normalized plan for checking.", created_at: "2026-07-01T12:01:00Z" },
            {
              schema_version: "0.1",
              run_id: "run-review",
              action: "corrected",
              reason: "Normalized row corrected before checking.",
              row_index: 1,
              source_item_id: 2,
              source_text: "a bowl berries",
              before: { food: "berries", quantity_text: "a bowl", resolution_status: "unresolved", unresolved_reason: "vague_quantity" },
              after: { food: "blueberries", quantity: 0.5, unit: "cup" },
              created_at: "2026-07-01T12:00:30Z",
            },
          ],
        })}
        setActiveTab={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Normalization Trace" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Source Inventory" })).toBeVisible();
    expect(screen.getAllByText("1 cup oatmeal").length).toBeGreaterThan(0);
    expect(screen.getByText("cooked oatmeal")).toBeVisible();
    expect(screen.getByText("a vague quantity")).toBeVisible();
    expect(screen.getByText("Unit Alias")).toBeVisible();
    expect(screen.getByText("Confirmed")).toBeVisible();
    expect(screen.getByText("Corrected")).toBeVisible();
    expect(screen.getAllByText("a bowl berries").length).toBeGreaterThan(0);
    expect(screen.getByText("blueberries (0.5 cup)")).toBeVisible();
    expect(screen.getByText("Json Decoded")).toBeVisible();
  });
});

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { reportArtifacts } from "../../test/factories/report";
import { ReportSurface } from "./ReportSurface";

describe("ReportSurface", () => {
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
    expect(screen.getByText("Break this mixed dish into ingredients.")).toBeVisible();
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
    expect(screen.getByText("Json Decoded")).toBeVisible();
  });
});

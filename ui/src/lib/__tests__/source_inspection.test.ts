import { describe, expect, it } from "vitest";
import { decision, reportArtifacts } from "../../test/factories/report";
import { buildSourceInspection } from "../source_inspection";

describe("buildSourceInspection", () => {
  it("resolves decision check source refs and reports missing refs", () => {
    const inspection = buildSourceInspection(reportArtifacts({
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
    }));

    expect(inspection.summary.citationSourceCount).toBe(1);
    expect(inspection.summary.checkSourceRefCount).toBe(2);
    expect(inspection.summary.missingSourceRefCount).toBe(1);
    expect(inspection.checkRows[0]).toMatchObject({
      check: "Sodium is under the daily limit",
      sources: "Dietary Guidelines for Americans, Missing Source",
      missing_source_refs: "missing-source",
    });
    expect(inspection.citationRows[0]).toMatchObject({
      claim_count: 1,
      claim_labels: "Sodium daily limit",
      referenced_by_checks: "Sodium is under the daily limit",
    });
    expect(inspection.missingSourceRows).toEqual([
      {
        source_ref: "missing-source",
        referenced_by_checks: "Sodium is under the daily limit",
      },
    ]);
  });

  it("traces review source text to resolved, unresolved, and excluded food outcomes", () => {
    const inspection = buildSourceInspection(reportArtifacts({
      resolved: [
        {
          day: 1,
          meal: "breakfast",
          food: "cooked oatmeal",
          grams: 234,
          nutrients: {
            energy_kcal: 160,
            protein_g: 6,
            sodium_mg: 5,
            added_sugar_g: 0,
          },
        },
      ],
      unresolved: [
        {
          day: 1,
          meal: "breakfast",
          food: "berries",
          quantity_text: "a bowl",
          unresolved_reason: "vague_quantity",
        },
      ],
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
        },
      ],
      normalizationReview: {
        schema_version: "0.1",
        run_id: "run-review",
        created_at: "2026-07-01T12:00:00Z",
        status: "confirmed",
        requires_confirmation: true,
        trust_signals: {
          source_item_count: 3,
          normalized_row_count: 3,
          unresolved_item_count: 2,
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
            source_item_id: 1,
            source_text: "1 cup oatmeal",
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
            normalized_food: "berries",
            resolved: false,
            quantity_text: "a bowl",
            unresolved_reason: "vague_quantity",
          },
          {
            day: 1,
            meal_code: "d",
            meal_label: "dinner",
            source_item_id: 3,
            source_text: "pinch sumac",
            normalized_food: "sumac",
            resolved: false,
            quantity: 1,
            unit: "g",
            unresolved_reason: "unknown_food",
          },
        ],
      },
    }));

    expect(inspection.summary.foodTraceCount).toBe(3);
    expect(inspection.foodRows.find((row) => row.source_item_id === 1)).toMatchObject({
      source_text: "1 cup oatmeal",
      food: "cooked oatmeal",
      quantity: "1 cup",
      status: "Resolved",
      grams: 234,
    });
    expect(inspection.foodRows.find((row) => row.source_item_id === 2)).toMatchObject({
      source_text: "a bowl berries",
      food: "berries",
      quantity: "a bowl",
      status: "Unresolved",
      reason: "a vague quantity",
      recovery_action: "Add a measured quantity and unit.",
    });
    expect(inspection.foodRows.find((row) => row.source_item_id === 3)).toMatchObject({
      source_text: "pinch sumac",
      food: "sumac",
      status: "Excluded From Totals",
      reason: "an unknown food; De Minimis Unresolved Mass",
      grams: 1,
    });
  });
});

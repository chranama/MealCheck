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
    expect(screen.getByRole("heading", { name: "Excluded From Totals" })).toBeVisible();
    expect(screen.getByText("sumac")).toBeVisible();
    expect(screen.getByText("de_minimis_unresolved_mass")).toBeVisible();
  });
});

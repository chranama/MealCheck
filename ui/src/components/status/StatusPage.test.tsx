import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StatusPage } from "./StatusPage";

describe("StatusPage", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    window.history.replaceState(null, "", "/");
  });

  it("renders public operational status without raw diagnostics", async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({
        schema_version: "0.1",
        generated_at: "2026-06-24T12:42:00Z",
        overall: { state: "operational", message: "All systems operational" },
        components: [
          { id: "meal_check_submission", name: "Meal Check Submission", state: "operational" },
          { id: "ai_meal_normalization", name: "AI Meal Normalization", state: "operational" },
          { id: "nutrition_allergen_checking", name: "Nutrition & Allergen Checking", state: "operational" },
          { id: "report_generation", name: "Report Generation", state: "operational" },
          { id: "sample_report", name: "Sample Report", state: "operational" },
        ],
        recent_incidents: [],
        links: {
          sample_report: "/api/demo-runs/seeded-3-day-peanut-allergy/report",
        },
      }),
      { status: 200 },
    ));
    vi.stubGlobal("fetch", fetchMock);

    const { container } = render(<StatusPage runtimeConfig={{ api: { base_url: "http://api" } }} />);

    expect(await screen.findByRole("heading", { name: "All systems operational" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "MealCheck Status" })).toBeVisible();
    expect(screen.getByText("No incidents reported in the past 7 days.")).toBeVisible();
    expect(screen.getByRole("link", { name: "Open sample report" })).toHaveAttribute(
      "href",
      "http://api/api/demo-runs/seeded-3-day-peanut-allergy/report",
    );

    const aiRow = screen.getByText("AI Meal Normalization").closest("article");
    expect(aiRow).not.toBeNull();
    expect(within(aiRow as HTMLElement).getByText("Operational")).toBeVisible();
    expect(container.textContent).not.toContain("queue_size");
    expect(container.textContent).not.toContain("local_model");
    expect(container.textContent).not.toContain("Qwen3");
  });

  it("renders an API-unreachable fallback and can retry", async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new Error("network failed"))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({
          schema_version: "0.1",
          generated_at: "2026-06-24T12:43:00Z",
          overall: { state: "degraded_performance", message: "MealCheck is experiencing degraded performance" },
          components: [
            {
              id: "meal_check_submission",
              name: "Meal Check Submission",
              state: "degraded_performance",
              message: "Meal checks are temporarily at capacity; retry shortly.",
            },
          ],
          recent_incidents: [],
        }),
        { status: 200 },
      ));
    vi.stubGlobal("fetch", fetchMock);

    render(<StatusPage runtimeConfig={{ api: { base_url: "http://api" } }} />);

    expect(await screen.findByRole("heading", { name: "MealCheck API is currently unreachable" })).toBeVisible();
    const websiteRow = screen.getByText("Website").closest("article");
    expect(websiteRow).not.toBeNull();
    expect(within(websiteRow as HTMLElement).getByText("Operational")).toBeVisible();
    const submissionRows = screen.getAllByText("Meal Check Submission");
    const submissionRow = submissionRows[0].closest("article");
    expect(submissionRow).not.toBeNull();
    expect(within(submissionRow as HTMLElement).getByText("Major Outage")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Refresh" }));

    expect(await screen.findByRole("heading", { name: "MealCheck is experiencing degraded performance" })).toBeVisible();
    expect(screen.getByText("Meal checks are temporarily at capacity; retry shortly.")).toBeVisible();
  });
});

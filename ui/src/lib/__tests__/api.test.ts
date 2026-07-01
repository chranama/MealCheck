import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  cleanApiBase,
  confirmNormalizedPlanReview,
  createRun,
  fetchNormalizedPlanReview,
  fetchHealth,
  fetchPublicStatus,
  joinUrl,
  loadLiveArtifacts,
  qualifyMealPlan,
  qualificationFromApiError,
  rejectNormalizedPlanReview,
  requestNormalizedPlanRewrite,
  requestJSON,
} from "../api";
import { DEFAULT_SETTINGS } from "../../constants";
import type { RunPayload } from "../../types";

describe("api", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("cleans and joins API URLs", () => {
    expect(cleanApiBase(" http://127.0.0.1:8080/ ")).toBe("http://127.0.0.1:8080");
    expect(joinUrl("http://127.0.0.1:8080/", "/api/health")).toBe("http://127.0.0.1:8080/api/health");
    expect(joinUrl("", "/demo-runs/index.json")).toBe("/demo-runs/index.json");
  });

  it("requires an API base for backend API paths", async () => {
    await expect(requestJSON("", "/api/runs")).rejects.toThrow("API base URL is required.");
  });

  it("formats backend error envelopes", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(
      JSON.stringify({
        error: { code: "invalid_input", message: "Bad settings", request_id: "req-123" },
      }),
      { status: 422 },
    )));

    await expect(requestJSON("http://api", "/api/runs")).rejects.toThrow(
      "HTTP 422: invalid_input - Bad settings (request_id=req-123)",
    );
  });

  it("parses JSON responses larger than the diagnostic preview", async () => {
    const largeValue = "x".repeat(5000);
    vi.stubGlobal("fetch", vi.fn(async () => new Response(
      JSON.stringify({ largeValue }),
      { status: 200 },
    )));

    await expect(requestJSON<{ largeValue: string }>("", "/demo-runs/large.json")).resolves.toEqual({
      largeValue,
    });
  });

  it("accepts JSON null as a parsed response body", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("null", { status: 200 })));

    await expect(requestJSON<null>("", "/demo-runs/null.json")).resolves.toBeNull();
  });

  it("creates runs with the invite token header", async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({ run_id: "run-1", status: "queued" }),
      { status: 202 },
    ));
    vi.stubGlobal("fetch", fetchMock);

    const payload: RunPayload = {
      input_mode: "profile_generation",
      settings: DEFAULT_SETTINGS,
      provider: {
        type: "openai",
        base_url: "",
        model: "gpt-test",
        api_key: "secret",
      },
      repair_json: true,
    };

    await expect(createRun("http://api", "invite-1", payload)).resolves.toEqual({
      run_id: "run-1",
      status: "queued",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "http://api/api/runs",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          "X-MealCheck-Invite-Token": "invite-1",
        }),
      }),
    );
  });

  it("creates public runs without an invite token header", async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({ run_id: "run-public", status: "queued" }),
      { status: 202 },
    ));
    vi.stubGlobal("fetch", fetchMock);

    const payload: RunPayload = {
      input_mode: "profile_generation",
      settings: DEFAULT_SETTINGS,
      provider: {
        type: "openai",
        base_url: "",
        model: "gpt-test",
        api_key: "secret",
      },
      repair_json: true,
    };

    await expect(createRun("http://api", "", payload)).resolves.toEqual({
      run_id: "run-public",
      status: "queued",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "http://api/api/runs",
      expect.objectContaining({
        method: "POST",
        headers: expect.not.objectContaining({
          "X-MealCheck-Invite-Token": expect.any(String),
        }),
      }),
    );
  });

  it("qualifies meal plan text with the invite token header", async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({
        qualification: {
          schema_version: "0.1",
          status: "eligible_for_verification",
          reason: "ok",
          provider_used: false,
        },
      }),
      { status: 200 },
    ));
    vi.stubGlobal("fetch", fetchMock);

    await expect(qualifyMealPlan("http://api", "invite-1", {
      text: "Day 1 breakfast: 1 cup oatmeal.",
      settings: DEFAULT_SETTINGS,
    })).resolves.toMatchObject({
      qualification: {
        status: "eligible_for_verification",
      },
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "http://api/api/qualify",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          "X-MealCheck-Invite-Token": "invite-1",
        }),
      }),
    );
  });

  it("loads and acts on normalized-plan review endpoints", async () => {
    const fetchMock = vi.fn(async (url: string, options?: RequestInit) => {
      if (url.endsWith("/review") && !options?.method) {
        return new Response(JSON.stringify({
          schema_version: "0.1",
          run_id: "run-review",
          created_at: "2026-07-01T12:00:00Z",
          status: "awaiting_confirmation",
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
            plan_id: "plan-review",
            description: "Review plan",
            days: [],
            shopping_list: [],
            prep_notes: [],
          },
          rows: [],
        }), { status: 200 });
      }
      return new Response(JSON.stringify({
        run: { status: "completed" },
        links: {},
        progress: {
          state: "ready",
          label: "Report ready",
          message: "done",
          updated_at: "2026-07-01T12:00:00Z",
        },
      }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchNormalizedPlanReview("http://api", "run-review")).resolves.toMatchObject({
      requires_confirmation: true,
    });
    await expect(confirmNormalizedPlanReview("http://api", "run-review")).resolves.toMatchObject({
      run: { status: "completed" },
    });
    await rejectNormalizedPlanReview("http://api", "run-review", "bad rows");
    await requestNormalizedPlanRewrite("http://api", "run-review", "rewrite source");

    expect(fetchMock).toHaveBeenCalledWith("http://api/api/runs/run-review/review", expect.objectContaining({
      headers: expect.objectContaining({ accept: "application/json" }),
    }));
    expect(fetchMock).toHaveBeenCalledWith("http://api/api/runs/run-review/review/confirm", expect.objectContaining({
      method: "POST",
    }));
    expect(fetchMock).toHaveBeenCalledWith("http://api/api/runs/run-review/review/reject", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ reason: "bad rows" }),
    }));
    expect(fetchMock).toHaveBeenCalledWith("http://api/api/runs/run-review/review/rewrite", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ reason: "rewrite source" }),
    }));
  });

  it("loads optional completed-run review and recommendation artifacts", async () => {
    const fetchMock = vi.fn(async (url: string) => {
      const path = new URL(url).pathname;
      const json = (value: unknown) => new Response(JSON.stringify(value), { status: 200 });
      if (path.endsWith("/decision.json")) return json({ decision: "warn", risk_level: "medium", summary: "reviewed" });
      if (path.endsWith("/report.json")) return json({ guideline_pack_id: "dga-2025-2030-us-adult-general-v1", constraint_summary: {}, profile_summary: {} });
      if (path.endsWith("/daily-totals.json")) return json([]);
      if (path.endsWith("/resolved-foods.json")) return json([]);
      if (path.endsWith("/unresolved-foods.json")) return json([]);
      if (path.endsWith("/excluded-unresolved-foods.json")) return json([]);
      if (path.endsWith("/manifest.json")) return json({
        mode: "hosted",
        artifacts: [
          "decision.json",
          "recommendation.json",
          "review/normalized-plan-review.json",
          "review/review-actions.jsonl",
          "optional/local-model-chunks.json",
          "optional/normalization-events.json",
        ],
      });
      if (path.endsWith("/guideline-pack/citations.json")) return json({ sources: [] });
      if (path.endsWith("/artifacts")) return json({ artifacts: [] });
      if (path.endsWith("/recommendation.json")) return json({
        schema_version: "0.1",
        status: "unavailable",
        reason: "No supported deterministic edit matched the failed checks.",
        source_decision: "warn",
        source_plan_id: "plan-review",
        blocking_checks: ["sodium_under_limit"],
      });
      if (path.endsWith("/review/normalized-plan-review.json")) return json({
        schema_version: "0.1",
        run_id: "run-review",
        created_at: "2026-07-01T12:00:00Z",
        status: "confirmed",
        requires_confirmation: false,
        trust_signals: {
          source_item_count: 1,
          normalized_row_count: 1,
          unresolved_item_count: 0,
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
        rows: [],
      });
      if (path.endsWith("/optional/normalization-events.json")) return json([
        { type: "json_decoded", message: "decoded", created_at: "2026-07-01T12:00:00Z" },
      ]);
      if (path.endsWith("/optional/local-model-chunks.json")) return json({
        schema_version: "0.1",
        created_at: "2026-07-01T12:00:00Z",
        plan_id: "plan-review",
        chunk_count: 1,
        source_item_count: 1,
        stage_timings: { total_ms: 100 },
        chunks: [],
      });
      if (path.endsWith("/review/review-actions.jsonl")) {
        return new Response(JSON.stringify({
          schema_version: "0.1",
          run_id: "run-review",
          action: "confirmed",
          reason: "ok",
          created_at: "2026-07-01T12:01:00Z",
        }) + "\n", { status: 200 });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(loadLiveArtifacts("http://api", "run-review")).resolves.toMatchObject({
      normalizationReview: {
        run_id: "run-review",
      },
      normalizationEvents: [
        { type: "json_decoded" },
      ],
      localModelExtraction: {
        chunk_count: 1,
      },
      reviewActions: [
        { action: "confirmed" },
      ],
      recommendation: {
        status: "unavailable",
        blocking_checks: ["sodium_under_limit"],
      },
    });
  });

  it("loads health metadata", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(
      JSON.stringify({
        status: "ok",
        store: "memory",
        access_mode: "public_byok",
        public_openai_compatible: false,
      }),
      { status: 200 },
    )));

    await expect(fetchHealth("http://api")).resolves.toMatchObject({
      status: "ok",
      access_mode: "public_byok",
      public_openai_compatible: false,
    });
  });

  it("loads public status metadata", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(
      JSON.stringify({
        schema_version: "0.1",
        generated_at: "2026-06-24T12:42:00Z",
        overall: { state: "operational", message: "All systems operational" },
        components: [
          { id: "meal_check_submission", name: "Meal Check Submission", state: "operational" },
        ],
        recent_incidents: [],
      }),
      { status: 200 },
    )));

    await expect(fetchPublicStatus("http://api")).resolves.toMatchObject({
      overall: { state: "operational" },
      components: [
        { id: "meal_check_submission", state: "operational" },
      ],
    });
  });

  it("exposes ApiError fields for diagnostics", () => {
    const error = new ApiError(500, "server failed");
    expect(error.status).toBe(500);
    expect(error.bodyText).toBe("server failed");
    expect(error.message).toBe("HTTP 500: server failed");
  });

  it("extracts qualification details from run creation errors", () => {
    const error = new ApiError(422, "not verifiable", {
      error: {
        code: "meal_plan_not_verifiable",
        message: "The text does not describe a meal plan.",
        details: {
          qualification: {
            schema_version: "0.1",
            status: "not_meal_plan",
            reason: "The text does not describe days, meals, recipes, or ingredient-level meal-plan content.",
            missing_fields: ["meal_plan_content"],
            provider_used: false,
          },
        },
      },
    });

    expect(qualificationFromApiError(error)).toMatchObject({
      status: "not_meal_plan",
      provider_used: false,
    });
  });

  it("ignores non-qualification API errors", () => {
    const error = new ApiError(500, "server failed", {
      error: { code: "store_error", message: "database unavailable" },
    });

    expect(qualificationFromApiError(error)).toBeNull();
    expect(qualificationFromApiError(new Error("network failed"))).toBeNull();
  });
});

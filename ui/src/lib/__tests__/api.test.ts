import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  cleanApiBase,
  createRun,
  joinUrl,
  requestJSON,
} from "../api";
import { DEFAULT_CONSTRAINTS, DEFAULT_PROFILE } from "../../constants";
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
        error: { code: "invalid_input", message: "Bad profile" },
        request_id: "req-123",
      }),
      { status: 422 },
    )));

    await expect(requestJSON("http://api", "/api/runs")).rejects.toThrow(
      "HTTP 422: invalid_input - Bad profile (request_id=req-123)",
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
      input_mode: "manual_structured",
      profile: DEFAULT_PROFILE,
      constraints: DEFAULT_CONSTRAINTS,
      candidate_plan: {
        schema_version: "0.1",
        plan_id: "manual-1",
        description: "test",
        days: [],
        shopping_list: [],
        prep_notes: [],
      },
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

  it("exposes ApiError fields for diagnostics", () => {
    const error = new ApiError(500, "server failed");
    expect(error.status).toBe(500);
    expect(error.bodyText).toBe("server failed");
    expect(error.message).toBe("HTTP 500: server failed");
  });
});

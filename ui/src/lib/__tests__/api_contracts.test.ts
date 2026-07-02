import { describe, expect, it } from "vitest";
import {
  ContractParseError,
  parseArtifactListResponse,
  parseCreateRunResponse,
  parseHealthResponse,
  parseNormalizedPlanReviewArtifact,
  parseRunDocument,
  parseRuntimeConfig,
} from "../api_contracts";

describe("api_contracts", () => {
  it("parses runtime config and filters unsupported feature flag values", () => {
    expect(parseRuntimeConfig({
      api: { base_url: "https://api.example" },
      features: {
        live: true,
        label: "beta",
        count: 2,
        disabled: null,
        nested: { no: "objects" },
      },
    })).toEqual({
      api: { base_url: "https://api.example" },
      features: {
        live: true,
        label: "beta",
        count: 2,
        disabled: null,
      },
    });
  });

  it("parses health metadata used for hosted workflow gating", () => {
    expect(parseHealthResponse({
      status: "ok",
      store: "postgres",
      access_mode: "invite_required",
      hosted_mode: "local_model",
      public_openai_compatible: false,
      max_candidate_text_chars: 5000,
      local_model: {
        enabled: true,
        ready: true,
        model: "qwen-test",
        max_source_items: 30,
      },
    })).toMatchObject({
      status: "ok",
      store: "postgres",
      access_mode: "invite_required",
      hosted_mode: "local_model",
      local_model: {
        ready: true,
        max_source_items: 30,
      },
    });
  });

  it("parses create-run, run document, artifact-list, and review artifacts", () => {
    expect(parseCreateRunResponse({ run_id: "run-1", status: "queued" })).toEqual({
      run_id: "run-1",
      status: "queued",
    });
    expect(parseRunDocument({
      run: { status: "completed", summary: "done" },
      progress: {
        state: "ready",
        label: "Ready",
        message: "done",
        updated_at: "2026-07-02T12:00:00Z",
      },
    })).toMatchObject({
      run: { status: "completed" },
      progress: { state: "ready" },
    });
    expect(parseArtifactListResponse({
      artifacts: [{ path: "decision.json", type: "json", url: "/api/runs/run-1/artifacts/decision.json" }],
    })).toMatchObject({
      artifacts: [{ path: "decision.json" }],
    });
    expect(parseNormalizedPlanReviewArtifact({
      schema_version: "0.1",
      run_id: "run-review",
      created_at: "2026-07-02T12:00:00Z",
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
        plan_id: "plan-1",
        description: "Plan",
        days: [],
        shopping_list: [],
        prep_notes: [],
      },
      rows: [],
    })).toMatchObject({
      run_id: "run-review",
      trust_signals: { source_item_count: 1 },
    });
  });

  it("throws path-specific errors for malformed contracts", () => {
    expect(() => parseCreateRunResponse({ status: "queued" })).toThrow(ContractParseError);
    expect(() => parseCreateRunResponse({ status: "queued" })).toThrow("create run response.run_id");
    expect(() => parseArtifactListResponse({ artifacts: [{ type: "json", url: "/x" }] })).toThrow(
      "artifact list response.artifacts[0].path",
    );
  });
});

import { describe, expect, it } from "vitest";
import { reportCreationPreflight } from "../report_preflight";
import type { BackendState } from "../../types";

const backend: BackendState = {
  online: true,
  label: "Online",
  kind: "online",
  accessMode: "public_byok",
  hostedMode: "local_model",
  publicOpenAICompatible: false,
  localModel: {
    enabled: true,
    ready: true,
    max_input_chars: 5000,
  },
};

function preflight(overrides: Partial<Parameters<typeof reportCreationPreflight>[0]> = {}) {
  return reportCreationPreflight({
    apiBase: "http://api",
    backend,
    candidateText: "Day 1 breakfast: 1 cup oatmeal.",
    candidateTextTooLong: false,
    inviteToken: "",
    liveStatus: "idle",
    qualificationStatus: "idle",
    reviewStatus: "idle",
    ...overrides,
  });
}

describe("report_preflight", () => {
  it("allows ready local-model report creation", () => {
    expect(preflight()).toMatchObject({
      canCreate: true,
      canQualify: false,
      reason: "ready",
      message: "Create a one-day local-model MealCheck report.",
    });
  });

  it("allows BYOK qualification when text and backend are ready", () => {
    expect(preflight({
      backend: {
        ...backend,
        hostedMode: "byok",
      },
    })).toMatchObject({
      canCreate: true,
      canQualify: true,
      message: "Check eligibility or create a MealCheck report.",
    });
  });

  it("blocks missing API configuration and access codes", () => {
    expect(preflight({ apiBase: "" })).toMatchObject({
      canCreate: false,
      reason: "api_unconfigured",
    });
    expect(preflight({
      backend: {
        ...backend,
        accessMode: "invite_required",
      },
    })).toMatchObject({
      canCreate: false,
      reason: "invite_required",
    });
  });

  it("blocks offline, unavailable local-model, invalid local-model text, and busy states", () => {
    expect(preflight({
      backend: {
        ...backend,
        kind: "offline",
      },
    })).toMatchObject({ reason: "backend_offline" });
    expect(preflight({
      backend: {
        ...backend,
        localModel: { enabled: true, ready: false },
      },
    })).toMatchObject({ reason: "local_model_unavailable" });
    expect(preflight({ candidateText: "" })).toMatchObject({ reason: "candidate_text_required" });
    expect(preflight({ candidateTextTooLong: true })).toMatchObject({ reason: "candidate_text_too_long" });
    expect(preflight({ liveStatus: "running" })).toMatchObject({ reason: "request_in_progress" });
  });
});

import { describe, expect, it } from "vitest";
import {
  DEFAULT_GENERATION_PROMPT,
  DEFAULT_PREP_NOTES,
  DEFAULT_PROVIDER,
  DEFAULT_SETTINGS,
  INITIAL_MANUAL_ITEMS,
} from "../../constants";
import {
  buildLocalModelRunPayload,
  buildQualificationPayload,
  buildManualPlan,
  buildRunPayload,
  normalizeSettings,
} from "../payload";
import type { SettingsDraft } from "../../types";

const draftSettings: SettingsDraft = {
  nutrition_targets: DEFAULT_SETTINGS.nutrition_targets,
  verification_constraints: {
    ...DEFAULT_SETTINGS.verification_constraints,
    allergies: "peanuts, shellfish",
    excluded_foods: "grapefruit",
  },
};

describe("payload", () => {
  it("normalizes nutrition targets and CSV constraint fields", () => {
    expect(normalizeSettings({
      ...draftSettings,
      nutrition_targets: {
        calorie_target_kcal: 2000.6,
        protein_target_g: 98.2,
      },
    })).toMatchObject({
      nutrition_targets: {
        calorie_target_kcal: 2001,
        protein_target_g: 98,
      },
      verification_constraints: {
        allergies: ["peanuts", "shellfish"],
        excluded_foods: ["grapefruit"],
      },
    });
  });

  it("builds a deterministic structured meal plan from manual rows", () => {
    const plan = buildManualPlan(INITIAL_MANUAL_ITEMS, DEFAULT_PREP_NOTES, 123);

    expect(plan.plan_id).toBe("manual-123");
    expect(plan.days).toHaveLength(1);
    expect(plan.days[0].meals.map((meal) => meal.name)).toEqual(["breakfast", "lunch", "dinner"]);
    expect(plan.shopping_list).toHaveLength(3);
    expect(plan.prep_notes).toEqual([
      "Refrigerate cooked foods within 2 hours.",
      "Reheat leftovers until steaming.",
    ]);
  });

  it("rejects empty manual plans", () => {
    expect(() => buildManualPlan([], "")).toThrow("At least one manual food item is required.");
  });

  it("builds a prompt-generation BYOK payload", () => {
    const payload = buildRunPayload({
      mode: "prompt_generation",
      settings: draftSettings,
      provider: {
        ...DEFAULT_PROVIDER,
        model: "gpt-test",
        api_key: "secret",
      },
      generationPrompt: DEFAULT_GENERATION_PROMPT,
      repairJSON: true,
    });

    expect(payload).toMatchObject({
      input_mode: "prompt_generation",
      repair_json: true,
      generation_prompt: DEFAULT_GENERATION_PROMPT,
      provider: {
        type: "openai",
        base_url: "",
        model: "gpt-test",
        api_key: "secret",
      },
      settings: {
        verification_constraints: {
          allergies: ["peanuts", "shellfish"],
        },
      },
    });
    expect(payload).not.toHaveProperty("profile");
    expect(payload).not.toHaveProperty("constraints");
  });

  it("preserves OpenAI-compatible base URLs", () => {
    const payload = buildRunPayload({
      mode: "profile_generation",
      settings: draftSettings,
      provider: {
        type: "openai_compatible",
        base_url: "https://router.local/v1/",
        model: "custom-model",
        api_key: "secret",
      },
      generationPrompt: DEFAULT_GENERATION_PROMPT,
      repairJSON: true,
    });

    expect(payload).toMatchObject({
      input_mode: "profile_generation",
      provider: {
        type: "openai_compatible",
        base_url: "https://router.local/v1",
        model: "custom-model",
      },
    });
  });

  it("clears base URLs for native providers", () => {
    const payload = buildRunPayload({
      mode: "profile_generation",
      settings: draftSettings,
      provider: {
        type: "gemini",
        base_url: "https://example.invalid",
        model: "gemini-test",
        api_key: "secret",
      },
      generationPrompt: DEFAULT_GENERATION_PROMPT,
      repairJSON: true,
    });

    expect(payload).toMatchObject({
      provider: {
        type: "gemini",
        base_url: "",
        model: "gemini-test",
      },
    });
  });

  it("requires provider model and API key for BYOK modes", () => {
    expect(() => buildRunPayload({
      mode: "profile_generation",
      settings: draftSettings,
      provider: DEFAULT_PROVIDER,
      generationPrompt: DEFAULT_GENERATION_PROMPT,
      repairJSON: true,
    })).toThrow("Provider model is required.");
  });

  it("builds local-model run payloads without provider credentials", () => {
    const payload = buildLocalModelRunPayload({
      text: " Day 1 breakfast: 1 cup oatmeal. ",
      settings: draftSettings,
    });

    expect(payload).toMatchObject({
      input_mode: "local_model",
      candidate_text: "Day 1 breakfast: 1 cup oatmeal.",
      settings: {
        verification_constraints: {
          allergies: ["peanuts", "shellfish"],
          days: 1,
        },
      },
    });
    expect(payload).not.toHaveProperty("provider");
    expect(payload).not.toHaveProperty("repair_json");
  });

  it("builds qualification payloads without provider keys when none are supplied", () => {
    const payload = buildQualificationPayload({
      text: " Day 1 breakfast: 1 cup oatmeal. ",
      settings: draftSettings,
      provider: DEFAULT_PROVIDER,
    });

    expect(payload).toMatchObject({
      text: "Day 1 breakfast: 1 cup oatmeal.",
      settings: {
        verification_constraints: {
          allergies: ["peanuts", "shellfish"],
        },
      },
    });
    expect(payload.provider).toBeUndefined();
  });

  it("adds provider config to qualification payloads when BYOK fields are supplied", () => {
    const payload = buildQualificationPayload({
      text: "Day 1 breakfast: 1 cup oatmeal.",
      settings: draftSettings,
      provider: {
        type: "openai_compatible",
        base_url: "https://router.local/v1/",
        model: "custom-model",
        api_key: "secret",
      },
    });

    expect(payload.provider).toMatchObject({
      type: "openai_compatible",
      base_url: "https://router.local/v1",
      model: "custom-model",
      api_key: "secret",
    });
  });
});

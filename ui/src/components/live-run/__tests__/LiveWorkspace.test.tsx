import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LiveWorkspace } from "../LiveWorkspace";
import type { BackendState, LiveState } from "../../../types";

const backend: BackendState = {
  online: true,
  label: "Online",
  kind: "online",
  accessMode: "public_byok",
  hostedMode: "byok",
  publicOpenAICompatible: false,
};

const inviteBackend: BackendState = {
  ...backend,
  accessMode: "invite_required",
};

const localModelBackend: BackendState = {
  ...backend,
  hostedMode: "local_model",
  localModel: {
    enabled: true,
    ready: true,
    model: "Qwen3-0.6B-Q4_K_M.gguf",
    max_input_chars: 6000,
    timeout_sec: 240,
  },
};

const live: LiveState = {
  runID: "",
  status: "idle",
  message: "",
  events: [],
  artifactItems: [],
};

const qualification = {
  status: "idle" as const,
  message: "",
  result: null,
};

function renderWorkspace(overrides: Partial<Parameters<typeof LiveWorkspace>[0]> = {}) {
  return render(
    <LiveWorkspace
      apiBase="http://127.0.0.1:8080"
      backend={backend}
      live={live}
      qualification={qualification}
      onCreateRun={vi.fn(async () => undefined)}
      onQualify={vi.fn(async () => undefined)}
      onDeleteRun={vi.fn(async () => undefined)}
      onError={vi.fn()}
      {...overrides}
    />,
  );
}

describe("LiveWorkspace", () => {
  it("shows a simplified local-model hosted workspace", () => {
    renderWorkspace({ backend: localModelBackend });

    expect(screen.getByText("Meal Plan Text")).toBeInTheDocument();
    expect(screen.getByLabelText("Meal plan text")).toBeVisible();
    expect(screen.getByText("Local model available")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "CLI/API and custom endpoint usage" })).toHaveAttribute(
      "href",
      "https://github.com/chranama/MealCheck",
    );
    expect(screen.queryByText("Qwen3-0.6B-Q4_K_M.gguf")).not.toBeInTheDocument();
    expect(screen.queryByText("Input")).not.toBeInTheDocument();
    expect(screen.queryByText("Timeout")).not.toBeInTheDocument();
    expect(screen.queryByText("240s")).not.toBeInTheDocument();
    expect(screen.getAllByText(/\/ 6,000 characters/).length).toBeGreaterThan(0);
    expect(screen.queryByText("Model Provider")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Provider")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("API key")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check Eligibility" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create Report" })).toBeEnabled();
  });

  it("submits local-model hosted runs without client provider configuration", async () => {
    const user = userEvent.setup();
    const onCreateRun = vi.fn(async () => undefined);
    renderWorkspace({ backend: localModelBackend, onCreateRun });

    await user.click(screen.getByRole("button", { name: "Create Report" }));

    expect(onCreateRun).toHaveBeenCalledWith(
      "http://127.0.0.1:8080",
      "",
      expect.objectContaining({
        input_mode: "local_model",
        candidate_text: expect.stringContaining("Day 1 breakfast"),
        settings: expect.any(Object),
      }),
    );
    const calls = onCreateRun.mock.calls as unknown as Array<[string, string, Record<string, unknown>]>;
    const payload = calls[0][2];
    expect(payload).not.toHaveProperty("provider");
    expect(payload).not.toHaveProperty("repair_json");
  });

  it("blocks local-model report creation when the model is unavailable", () => {
    renderWorkspace({
      backend: {
        ...localModelBackend,
        localModel: {
          ...localModelBackend.localModel,
          ready: false,
          error: "local model endpoint is not reachable",
        },
      },
    });

    expect(screen.getByText("Local model unavailable")).toBeInTheDocument();
    expect(screen.queryByText("local model endpoint is not reachable")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create Report" })).toBeDisabled();
  });

  it("shows a text-first BYOK workspace with verification settings collapsed", () => {
    renderWorkspace();

    expect(screen.getByText("Meal Plan Text")).toBeInTheDocument();
    expect(screen.getByLabelText("Meal plan text")).toBeVisible();
    expect(screen.getByText("Model Provider")).toBeInTheDocument();
    expect(screen.queryByLabelText("Access code")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Manual" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Targets" })).toHaveClass("is-active");
    expect(screen.getByText("Model provider disclosure")).toBeInTheDocument();
    expect(screen.getByText("Verification Settings").closest("details")).not.toHaveAttribute("open");
    expect(screen.getByLabelText("Days")).not.toBeVisible();
    expect(screen.queryByLabelText("Age")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Diet pattern")).not.toBeInTheDocument();
    expect(screen.queryByText("Shopping list required")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Prep notes")).not.toBeInTheDocument();
  });

  it("shows access code entry when the backend requires invites", () => {
    renderWorkspace({ backend: inviteBackend });

    expect(screen.getByText("Access")).toBeInTheDocument();
    expect(screen.getByLabelText("Access code")).toBeVisible();
    expect(screen.getByRole("button", { name: "Create Report" })).toBeDisabled();
    expect(screen.getAllByText("Enter your access code to start.").length).toBeGreaterThan(0);
  });

  it("keeps targets and constraints configurable behind verification settings", async () => {
    const user = userEvent.setup();
    const onCreateRun = vi.fn(async () => undefined);
    renderWorkspace({ onCreateRun });

    await user.click(screen.getByText("Verification Settings"));
    expect(screen.getByText("Nutrition Targets")).toBeInTheDocument();
    expect(screen.getByLabelText("Calories")).toBeVisible();
    expect(screen.getByLabelText("Protein g")).toBeVisible();
    expect(screen.getByLabelText("Days")).toBeVisible();
    expect(screen.getByText("Advanced constraints")).toBeVisible();
    expect(screen.queryByLabelText("Age")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Sex")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Height cm")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Weight kg")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Activity")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Goal")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Diet pattern")).not.toBeInTheDocument();
    expect(screen.queryByText("Shopping list required")).not.toBeInTheDocument();

    await user.clear(screen.getByLabelText("Calories"));
    await user.type(screen.getByLabelText("Calories"), "2100");
    await user.clear(screen.getByLabelText("Protein g"));
    await user.type(screen.getByLabelText("Protein g"), "120");
    await user.clear(screen.getByLabelText("Days"));
    await user.type(screen.getByLabelText("Days"), "2");
    await user.type(screen.getByLabelText("Model"), "gpt-test");
    await user.type(screen.getByLabelText("API key"), "secret");
    await user.click(screen.getByRole("button", { name: "Create Report" }));

    expect(onCreateRun).toHaveBeenCalledWith(
      "http://127.0.0.1:8080",
      "",
      expect.objectContaining({
        input_mode: "profile_generation",
        settings: expect.objectContaining({
          nutrition_targets: expect.objectContaining({
            calorie_target_kcal: 2100,
            protein_target_g: 120,
          }),
          verification_constraints: expect.objectContaining({
            days: 2,
          }),
        }),
      }),
    );
  });

  it("switches prompt mode to prompt controls", async () => {
    const user = userEvent.setup();
    renderWorkspace();

    await user.click(screen.getByRole("button", { name: "Prompt" }));

    expect(screen.getByRole("button", { name: "Prompt" })).toHaveClass("is-active");
    expect(screen.getByText("Model provider disclosure")).toBeInTheDocument();
    expect(screen.getByText(/Use temporary, scoped, budget-limited keys/)).toBeInTheDocument();
    expect(screen.getByText(/Hosted public mode disables custom OpenAI-compatible endpoints/)).toBeInTheDocument();
    expect(screen.getByText(/run MealCheck locally from the repo/)).toBeInTheDocument();
    expect(screen.getByLabelText("Provider")).toHaveValue("openai");
    expect(screen.getByLabelText("Prompt")).toBeInTheDocument();
    expect(screen.queryByLabelText("Base URL")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Prep notes")).not.toBeInTheDocument();
  });

  it("submits qualification payloads through the parent API boundary", async () => {
    const user = userEvent.setup();
    const onQualify = vi.fn(async () => undefined);
    renderWorkspace({ onQualify });

    await user.click(screen.getByRole("button", { name: "Check Eligibility" }));

    expect(onQualify).toHaveBeenCalledWith(
      "http://127.0.0.1:8080",
      "",
      expect.objectContaining({
        text: expect.stringContaining("Day 1 breakfast"),
      }),
    );
    const calls = onQualify.mock.calls as unknown as Array<[string, string, Record<string, unknown>]>;
    const payload = calls[0][2];
    expect(payload).not.toHaveProperty("provider");
  });

  it("keeps run creation disabled in static mode until live prerequisites exist", () => {
    renderWorkspace({
      apiBase: "",
      backend: {
        online: false,
        label: "Not configured",
        kind: "idle",
        accessMode: "public_byok",
        hostedMode: "byok",
        publicOpenAICompatible: false,
      },
    });

    expect(screen.queryByText("Examples only")).not.toBeInTheDocument();
    expect(screen.queryByText("Service ready")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create Report" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Check Eligibility" })).toBeDisabled();
    expect(screen.getAllByText("Report creation needs a configured MealCheck service.").length).toBeGreaterThan(0);
  });

  it("confirms before deleting an existing live run", async () => {
    const user = userEvent.setup();
    const onDeleteRun = vi.fn(async () => undefined);
    renderWorkspace({
      live: {
        ...live,
        runID: "run-1",
        status: "completed",
        message: "Artifacts ready.",
      },
      onDeleteRun,
    });

    await user.click(screen.getByRole("button", { name: "Delete Report" }));
    expect(screen.getByRole("dialog", { name: "Delete report?" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onDeleteRun).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Delete Report" }));
    await user.click(within(screen.getByRole("dialog", { name: "Delete report?" })).getByRole("button", { name: "Delete Report" }));

    expect(onDeleteRun).toHaveBeenCalledTimes(1);
  });

  it("submits prompt-generation payloads and clears provider keys", async () => {
    const user = userEvent.setup();
    const onCreateRun = vi.fn(async () => undefined);
    const localStorageSetItem = vi.fn();
    const sessionStorageSetItem = vi.fn();
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      value: {
        clear: vi.fn(),
        getItem: vi.fn(),
        key: vi.fn(),
        length: 0,
        removeItem: vi.fn(),
        setItem: localStorageSetItem,
      },
    });
    Object.defineProperty(window, "sessionStorage", {
      configurable: true,
      value: {
        clear: vi.fn(),
        getItem: vi.fn(),
        key: vi.fn(),
        length: 0,
        removeItem: vi.fn(),
        setItem: sessionStorageSetItem,
      },
    });
    renderWorkspace({ onCreateRun });

    await user.click(screen.getByRole("button", { name: "Prompt" }));
    await user.type(screen.getByLabelText("Model"), "gpt-test");
    await user.type(screen.getByLabelText("API key"), "secret");
    await user.click(screen.getByRole("button", { name: "Create Report" }));

    expect(onCreateRun).toHaveBeenCalledWith(
      "http://127.0.0.1:8080",
      "",
      expect.objectContaining({
        input_mode: "prompt_generation",
        provider: expect.objectContaining({
          type: "openai",
          base_url: "",
          model: "gpt-test",
          api_key: "secret",
        }),
      }),
    );
    expect(screen.getByLabelText("API key")).toHaveValue("");
    expect(localStorageSetItem).not.toHaveBeenCalled();
    expect(sessionStorageSetItem).not.toHaveBeenCalled();
  });

  it("submits qualification with BYOK provider config and clears provider keys", async () => {
    const user = userEvent.setup();
    const onQualify = vi.fn(async () => undefined);
    renderWorkspace({ onQualify });

    await user.type(screen.getByLabelText("Model"), "gpt-test");
    await user.type(screen.getByLabelText("API key"), "secret");
    await user.click(screen.getByRole("button", { name: "Check Eligibility" }));

    expect(onQualify).toHaveBeenCalledWith(
      "http://127.0.0.1:8080",
      "",
      expect.objectContaining({
        text: expect.stringContaining("Day 1 breakfast"),
        provider: expect.objectContaining({
          type: "openai",
          base_url: "",
          model: "gpt-test",
          api_key: "secret",
        }),
      }),
    );
    expect(screen.getByLabelText("API key")).toHaveValue("");
  });

  it("renders completed qualification results", () => {
    renderWorkspace({
      qualification: {
        status: "completed",
        message: "Candidate text qualifies for verification.",
        result: {
          schema_version: "0.1",
          status: "eligible_for_verification",
          reason: "ok",
          provider_used: true,
          normalized_plan: {
            schema_version: "0.1",
            plan_id: "normalized",
            description: "Normalized",
            days: [
              {
                day: 1,
                meals: [
                  {
                    name: "breakfast",
                    items: [{ food: "cooked oatmeal", quantity: 1, unit: "cup" }],
                  },
                ],
              },
            ],
            shopping_list: [],
            prep_notes: [],
          },
        },
      },
    });

    expect(screen.getByText("Candidate text qualifies for verification.")).toBeInTheDocument();
    expect(screen.getByText("Eligible For Verification")).toBeInTheDocument();
    expect(screen.getByText("1 day, 1 meal, 1 item")).toBeInTheDocument();
  });

  it("shows inline recovery guidance for vague meal-plan text", () => {
    renderWorkspace({
      qualification: {
        status: "completed",
        message: "Breakfast, lunch, and dinner are named but quantities are missing.",
        result: {
          schema_version: "0.1",
          status: "meal_plan_too_vague",
          reason: "Breakfast, lunch, and dinner are named but quantities are missing.",
          missing_fields: ["quantities"],
          provider_used: false,
        },
      },
    });

    expect(screen.getByText("Add amounts and units")).toBeInTheDocument();
    expect(screen.getByText(/Add quantities such as 1 cup, 4 oz, 2 eggs, or 1 tbsp/)).toBeInTheDocument();
    expect(screen.getByText("Meal Plan Too Vague")).toBeInTheDocument();
  });

  it("shows recovery guidance when local-model normalization fails after queueing", () => {
    renderWorkspace({
      live: {
        ...live,
        status: "failed",
        message: "MealCheck could not normalize this text into a verifiable meal plan. Use clear day labels, meal labels, food names, numeric quantities, and supported units.",
      },
    });

    expect(screen.getByText("MealCheck could not normalize this plan")).toBeInTheDocument();
    expect(screen.getByText(/Use clear Day 1, Day 2 labels/)).toBeInTheDocument();
    expect(screen.getByText(/Put each meal on its own line/)).toBeInTheDocument();
  });

  it("submits selected Gemini providers", async () => {
    const user = userEvent.setup();
    const onCreateRun = vi.fn(async () => undefined);
    renderWorkspace({ onCreateRun });

    await user.click(screen.getByRole("button", { name: "Targets" }));
    await user.selectOptions(screen.getByLabelText("Provider"), "gemini");
    await user.type(screen.getByLabelText("Model"), "gemini-test");
    await user.type(screen.getByLabelText("API key"), "secret");
    await user.click(screen.getByRole("button", { name: "Create Report" }));

    expect(onCreateRun).toHaveBeenCalledWith(
      "http://127.0.0.1:8080",
      "",
      expect.objectContaining({
        input_mode: "profile_generation",
        provider: expect.objectContaining({
          type: "gemini",
          base_url: "",
          model: "gemini-test",
          api_key: "secret",
        }),
      }),
    );
  });
});

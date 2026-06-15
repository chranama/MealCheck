import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LiveWorkspace } from "../LiveWorkspace";
import type { BackendState, LiveState } from "../../../types";

const backend: BackendState = {
  online: true,
  label: "Online",
  kind: "online",
};

const live: LiveState = {
  runID: "",
  status: "idle",
  message: "",
  events: [],
  artifactItems: [],
};

function renderWorkspace(overrides: Partial<Parameters<typeof LiveWorkspace>[0]> = {}) {
  return render(
    <LiveWorkspace
      apiBase="http://127.0.0.1:8080"
      backend={backend}
      live={live}
      onCreateRun={vi.fn(async () => undefined)}
      onDeleteRun={vi.fn(async () => undefined)}
      onError={vi.fn()}
      {...overrides}
    />,
  );
}

describe("LiveWorkspace", () => {
  it("shows manual structured entry by default", () => {
    renderWorkspace();

    expect(screen.getByText("Meal Plan Entry")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Manual" })).toHaveClass("is-active");
    expect(screen.getByLabelText("Prep notes")).toBeInTheDocument();
    expect(screen.queryByText("BYOK provider disclosure")).not.toBeInTheDocument();
  });

  it("switches prompt mode to BYOK provider controls", async () => {
    const user = userEvent.setup();
    renderWorkspace();

    await user.click(screen.getByRole("button", { name: "Prompt" }));

    expect(screen.getByRole("button", { name: "Prompt" })).toHaveClass("is-active");
    expect(screen.getByText("BYOK provider disclosure")).toBeInTheDocument();
    expect(screen.getByLabelText("Prompt")).toBeInTheDocument();
    expect(screen.queryByLabelText("Prep notes")).not.toBeInTheDocument();
  });

  it("submits manual payloads through the parent API boundary", async () => {
    const user = userEvent.setup();
    const onCreateRun = vi.fn(async () => undefined);
    renderWorkspace({ onCreateRun });

    await user.type(screen.getByLabelText("Access code"), "invite-1");
    await user.click(screen.getByRole("button", { name: "Create Report" }));

    expect(onCreateRun).toHaveBeenCalledWith(
      "http://127.0.0.1:8080",
      "invite-1",
      expect.objectContaining({
        input_mode: "manual_structured",
        candidate_plan: expect.objectContaining({
          days: expect.any(Array),
        }),
      }),
    );
  });

  it("keeps run creation disabled in static mode until live prerequisites exist", () => {
    renderWorkspace({
      apiBase: "",
      backend: {
        online: false,
        label: "Static demo",
        kind: "idle",
      },
    });

    expect(screen.queryByText("Examples only")).not.toBeInTheDocument();
    expect(screen.queryByText("Service ready")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create Report" })).toBeDisabled();
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
    renderWorkspace({ onCreateRun });

    await user.click(screen.getByRole("button", { name: "Prompt" }));
    await user.type(screen.getByLabelText("Access code"), "invite-1");
    await user.type(screen.getByLabelText("Model"), "gpt-test");
    await user.type(screen.getByLabelText("API key"), "secret");
    await user.click(screen.getByRole("button", { name: "Create Report" }));

    expect(onCreateRun).toHaveBeenCalledWith(
      "http://127.0.0.1:8080",
      "invite-1",
      expect.objectContaining({
        input_mode: "prompt_generation",
        provider: expect.objectContaining({
          model: "gpt-test",
          api_key: "secret",
        }),
      }),
    );
    expect(screen.getByLabelText("API key")).toHaveValue("");
  });
});

import { render, screen } from "@testing-library/react";
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
      setApiBase={vi.fn()}
      backend={backend}
      live={live}
      onHealthCheck={vi.fn(async () => undefined)}
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

    expect(screen.getByText("Input Mode")).toBeInTheDocument();
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

    await user.type(screen.getByLabelText("Invite Token"), "invite-1");
    await user.click(screen.getByRole("button", { name: "Create Run" }));

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

  it("submits prompt-generation payloads and clears provider keys", async () => {
    const user = userEvent.setup();
    const onCreateRun = vi.fn(async () => undefined);
    renderWorkspace({ onCreateRun });

    await user.click(screen.getByRole("button", { name: "Prompt" }));
    await user.type(screen.getByLabelText("Invite Token"), "invite-1");
    await user.type(screen.getByLabelText("Model"), "gpt-test");
    await user.type(screen.getByLabelText("API key"), "secret");
    await user.click(screen.getByRole("button", { name: "Create Run" }));

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

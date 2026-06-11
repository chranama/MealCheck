import { afterEach, describe, expect, it, vi } from "vitest";
import { configuredApiBase, loadRuntimeConfig } from "../runtime_config";

describe("runtime_config", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    window.history.replaceState(null, "", "/");
    document.head.innerHTML = "";
    window.MEALCHECK_API_BASE_URL = "";
  });

  it("loads config.json and merges the API base into fallback config", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(
      JSON.stringify({ api: { base_url: "http://backend.local/" } }),
      { status: 200 },
    )));

    await expect(loadRuntimeConfig()).resolves.toMatchObject({
      api: {
        base_url: "http://backend.local/",
      },
    });
  });

  it("falls back when config.json is unavailable", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 404 })));

    await expect(loadRuntimeConfig()).resolves.toHaveProperty("api.base_url");
  });

  it("prefers explicit query parameter over runtime, window, and meta config", () => {
    window.history.replaceState(null, "", "/?api=http://query.local/");
    window.MEALCHECK_API_BASE_URL = "http://window.local";
    const meta = document.createElement("meta");
    meta.name = "mealcheck-api-base";
    meta.content = "http://meta.local";
    document.head.appendChild(meta);

    expect(configuredApiBase({ api: { base_url: "http://runtime.local" } })).toBe("http://query.local");
  });

  it("uses runtime config when no query override is present", () => {
    window.MEALCHECK_API_BASE_URL = "http://window.local";

    expect(configuredApiBase({ api: { base_url: "http://runtime.local/" } })).toBe("http://runtime.local");
  });
});

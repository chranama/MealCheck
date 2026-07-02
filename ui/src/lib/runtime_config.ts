import type { RuntimeConfig } from "../types";
import { cleanApiBase } from "./api";
import { parseRuntimeConfig } from "./api_contracts";

export async function loadRuntimeConfig(): Promise<RuntimeConfig> {
  const fallback: RuntimeConfig = {
    api: {
      base_url: import.meta.env.VITE_MEALCHECK_API_BASE_URL || "",
    },
  };

  try {
    const response = await fetch(`/config.json?ts=${Date.now()}`, { cache: "no-store" });
    if (!response.ok) return fallback;
    const config = parseRuntimeConfig(await response.json());
    return {
      ...fallback,
      ...config,
      api: { ...fallback.api, ...(config.api || {}) },
    };
  } catch {
    return fallback;
  }
}

export function configuredApiBase(runtimeConfig: RuntimeConfig): string {
  const params = new URLSearchParams(window.location.search);
  const fromQuery = params.get("api") || "";
  const metaBase = document.querySelector('meta[name="mealcheck-api-base"]')?.getAttribute("content") || "";
  const fromWindow = typeof window !== "undefined" ? window.MEALCHECK_API_BASE_URL || "" : "";
  const fromRuntime = runtimeConfig.api?.base_url || "";
  const fromEnv = import.meta.env.VITE_MEALCHECK_API_BASE_URL || "";
  return cleanApiBase(fromQuery || fromRuntime || fromWindow || fromEnv || metaBase);
}

import type {
  ArtifactListResponse,
  CreateRunResponse,
  HealthResponse,
  QualifyMealPlanPayload,
  QualifyMealPlanResponse,
  ReportArtifacts,
  RunDocument,
  RunEvent,
  RunPayload,
} from "../types";
import { parseSSE } from "./sse";

declare global {
  interface Window {
    MEALCHECK_API_BASE_URL?: string;
  }
}

export class ApiError extends Error {
  status: number;
  bodyText: string;
  bodyJson: unknown;

  constructor(status: number, bodyText: string, bodyJson: unknown = null) {
    super(ApiError.makeMessage(status, bodyText, bodyJson));
    this.name = "ApiError";
    this.status = status;
    this.bodyText = bodyText;
    this.bodyJson = bodyJson;
  }

  private static makeMessage(status: number, bodyText: string, bodyJson: unknown): string {
    if (bodyJson && typeof bodyJson === "object") {
      const root = bodyJson as Record<string, unknown>;
      const errObj = root.error && typeof root.error === "object" ? root.error as Record<string, unknown> : root;
      const code = errObj.code != null ? String(errObj.code) : root.code != null ? String(root.code) : null;
      const message = errObj.message != null
        ? String(errObj.message)
        : errObj.detail != null
          ? typeof errObj.detail === "string" ? errObj.detail : JSON.stringify(errObj.detail)
          : root.message != null ? String(root.message) : null;
      const requestID = root.request_id != null ? String(root.request_id) : null;
      if (code || message) {
        const base = `HTTP ${status}: ${code ?? "error"}${message ? ` - ${message}` : ""}`;
        return requestID ? `${base} (request_id=${requestID})` : base;
      }
    }
    return `HTTP ${status}: ${bodyText}`;
  }
}

export function cleanApiBase(base: unknown): string {
  return String(base || "").trim().replace(/\/$/, "");
}

export function joinUrl(base: string, path: string): string {
  return cleanApiBase(base) ? `${cleanApiBase(base)}${path}` : path;
}

export async function requestJSON<T>(base: string, path: string, options: RequestInit = {}): Promise<T> {
  if (!cleanApiBase(base) && path.startsWith("/api/")) {
    throw new Error("API base URL is required.");
  }
  const response = await fetch(joinUrl(base, path), {
    ...options,
    headers: {
      accept: "application/json",
      ...(options.headers || {}),
    },
  });
  const bodyText = await response.text();
  const preview = bodyText.slice(0, 4000);
  let bodyJson: unknown = null;
  let parsedJSON = false;
  if (bodyText.trim()) {
    try {
      bodyJson = JSON.parse(bodyText);
      parsedJSON = true;
    } catch {
      bodyJson = null;
    }
  }
  if (!response.ok) {
    throw new ApiError(response.status, preview, bodyJson);
  }
  if (!bodyText.trim()) return {} as T;
  if (parsedJSON) return bodyJson as T;
  throw new ApiError(response.status, `Non-JSON response: ${preview}`, null);
}

export async function checkHealth(base: string): Promise<boolean> {
  try {
    await fetchHealth(base);
    return true;
  } catch {
    return false;
  }
}

export async function fetchHealth(base: string): Promise<HealthResponse> {
  if (!cleanApiBase(base)) {
    throw new Error("API base URL is required.");
  }
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 2500);
  try {
    const response = await fetch(joinUrl(base, "/api/health"), {
      signal: controller.signal,
      headers: { accept: "application/json" },
    });
    if (!response.ok) {
      throw new ApiError(response.status, await response.text());
    }
    return await response.json() as HealthResponse;
  } finally {
    window.clearTimeout(timeout);
  }
}

export async function createRun(base: string, inviteToken: string, payload: RunPayload): Promise<CreateRunResponse> {
  return requestJSON<CreateRunResponse>(base, "/api/runs", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...inviteHeader(inviteToken),
    },
    body: JSON.stringify(payload),
  });
}

export async function qualifyMealPlan(base: string, inviteToken: string, payload: QualifyMealPlanPayload): Promise<QualifyMealPlanResponse> {
  return requestJSON<QualifyMealPlanResponse>(base, "/api/qualify", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...inviteHeader(inviteToken),
    },
    body: JSON.stringify(payload),
  });
}

function inviteHeader(inviteToken: string): Record<string, string> {
  const token = inviteToken.trim();
  return token ? { "X-MealCheck-Invite-Token": token } : {};
}

export async function fetchRun(base: string, runID: string): Promise<RunDocument> {
  return requestJSON<RunDocument>(base, `/api/runs/${runID}`);
}

export async function fetchEvents(base: string, runID: string, fallback: RunEvent[]): Promise<RunEvent[]> {
  const response = await fetch(joinUrl(base, `/api/runs/${runID}/events`), {
    headers: { accept: "text/event-stream" },
  });
  if (!response.ok) return fallback;
  return parseSSE(await response.text());
}

export async function fetchArtifact<T>(base: string, runID: string, path: string): Promise<T> {
  return requestJSON<T>(base, `/api/runs/${runID}/artifacts/${path}`);
}

export async function loadLiveArtifacts(base: string, runID: string): Promise<ReportArtifacts> {
  const [decision, report, totals, resolved, unresolved, manifest, citations, artifactList] = await Promise.all([
    fetchArtifact(base, runID, "decision.json"),
    fetchArtifact(base, runID, "report.json"),
    fetchArtifact(base, runID, "daily-totals.json"),
    fetchArtifact(base, runID, "resolved-foods.json"),
    fetchArtifact(base, runID, "unresolved-foods.json"),
    fetchArtifact(base, runID, "manifest.json"),
    fetchArtifact(base, runID, "guideline-pack/citations.json"),
    requestJSON<ArtifactListResponse>(base, `/api/runs/${runID}/artifacts`),
  ]);
  const artifactItems = artifactList.artifacts || [];
  return {
    apiBase: base,
    base: `${base}/api/runs/${runID}/artifacts`,
    decision,
    report,
    totals,
    resolved,
    unresolved,
    manifest,
    pack: null,
    citations,
    artifactItems,
  } as ReportArtifacts;
}

export async function deleteRun(base: string, runID: string): Promise<void> {
  await requestJSON(base, `/api/runs/${runID}`, { method: "DELETE" });
}

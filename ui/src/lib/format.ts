import type { Citations, RunStatus } from "../types";

export function readableID(entry: unknown): string {
  return String(entry || "")
    .replace(/_/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function round(entry: unknown): number {
  return Math.round(Number(entry) * 10) / 10;
}

export function valueText(entry: unknown): string {
  if (entry === undefined || entry === null || entry === "") {
    return "-";
  }
  if (typeof entry === "number") {
    return String(round(entry));
  }
  return String(entry);
}

export function artifactType(path: string): string {
  if (path.endsWith(".jsonl")) return "JSONL";
  if (path.endsWith(".json")) return "JSON";
  if (path.endsWith(".html")) return "HTML";
  if (path.endsWith(".md")) return "Markdown";
  return "File";
}

export function liveStatusTone(status: RunStatus): string {
  if (status === "completed") return "pass";
  if (status === "failed" || status === "deleted") return "block";
  if (status === "queued" || status === "running") return "warn";
  return "info";
}

export function modeLabel(mode: string): string {
  if (mode === "manual_structured") return "Manual";
  if (mode === "profile_generation") return "Profile";
  return "Prompt";
}

export function sourceChip(citations: Citations, sourceID: string): string {
  const source = citations?.sources?.find((candidate) => candidate.source_id === sourceID);
  return source ? source.title : sourceID;
}

export function csvValue(value: unknown): string[] {
  return String(value || "").split(",").map((entry) => entry.trim()).filter(Boolean);
}

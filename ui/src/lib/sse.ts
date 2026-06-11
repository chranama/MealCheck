import type { RunEvent } from "../types";

export function parseSSE(text: string): RunEvent[] {
  return text.split("\n\n").map((chunk) => {
    const dataLine = chunk.split("\n").find((line) => line.startsWith("data: "));
    if (!dataLine) return null;
    try {
      return JSON.parse(dataLine.slice(6)) as RunEvent;
    } catch {
      return null;
    }
  }).filter((entry): entry is RunEvent => Boolean(entry));
}

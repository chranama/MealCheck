import { describe, expect, it } from "vitest";
import { parseSSE } from "../sse";

describe("sse", () => {
  it("parses event-stream data lines and ignores malformed chunks", () => {
    expect(parseSSE([
      'event: status',
      'data: {"type":"queued","message":"Run queued."}',
      '',
      'event: status',
      'data: {"type":"completed","message":"Artifacts ready."}',
      '',
      'event: status',
      'data: not-json',
      '',
    ].join("\n"))).toEqual([
      { type: "queued", message: "Run queued." },
      { type: "completed", message: "Artifacts ready." },
    ]);
  });
});

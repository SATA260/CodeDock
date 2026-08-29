import type { ToolCall } from "./types.ts";

export function decodeText(content: unknown): string {
  if (content == null) {
    return "";
  }
  if (typeof content === "string") {
    const trimmed = content.trim();
    if (trimmed.startsWith("{") || trimmed.startsWith('"')) {
      try {
        return decodeText(JSON.parse(content));
      } catch {
        return content;
      }
    }
    return content;
  }
  if (typeof content === "object" && content !== null && "text" in content) {
    const text = (content as { text: unknown }).text;
    return typeof text === "string" ? text : "";
  }
  return "";
}

export function firstLine(text: string, max = 48): string {
  const line = text.split("\n").find((part) => part.trim()) ?? text;
  const trimmed = line.trim();
  if (trimmed.length <= max) {
    return trimmed;
  }
  return `${trimmed.slice(0, max).trimEnd()}…`;
}

export type ParsedDelta =
  | { kind: "text"; text: string }
  | { kind: "tool"; call: ToolCall }
  | { kind: "unknown" };

export function parseDelta(delta: unknown): ParsedDelta {
  if (!delta || typeof delta !== "object") {
    return { kind: "unknown" };
  }
  const obj = delta as Record<string, unknown>;
  if (typeof obj.name === "string" || typeof obj.id === "string") {
    return {
      kind: "tool",
      call: {
        id: typeof obj.id === "string" ? obj.id : "",
        name: typeof obj.name === "string" ? obj.name : "",
        arguments: obj.arguments,
        attempt: typeof obj.attempt === "number" ? obj.attempt : undefined,
      },
    };
  }
  if (typeof obj.text === "string") {
    return { kind: "text", text: obj.text };
  }
  return { kind: "unknown" };
}

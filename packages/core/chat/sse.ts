import type { AgentEvent } from "./types.ts";

export function parseSSEChunk(buffer: string): { events: AgentEvent[]; rest: string } {
  const parts = buffer.split("\n\n");
  const rest = parts.pop() ?? "";
  const events: AgentEvent[] = [];
  for (const block of parts) {
    const ev = parseSSEBlock(block);
    if (ev) {
      events.push(ev);
    }
  }
  return { events, rest };
}

export function parseSSEBlock(block: string): AgentEvent | null {
  let data = "";
  for (const raw of block.split("\n")) {
    const line = raw.replace(/\r$/, "");
    if (line.startsWith("data:")) {
      data += line.slice(5).trim();
    }
  }
  if (!data) {
    return null;
  }
  return JSON.parse(data) as AgentEvent;
}

export interface WatchEventsOptions {
  baseUrl: string;
  sessionId: string;
  getAfterSeq: () => number;
  onEvent: (event: AgentEvent) => void;
  signal: AbortSignal;
  fetch?: typeof fetch;
  retryDelayMs?: number;
}

export async function watchEvents(options: WatchEventsOptions): Promise<void> {
  const fetchImpl = options.fetch ?? fetch.bind(globalThis);
  const retryDelayMs = options.retryDelayMs ?? 500;
  const baseUrl = options.baseUrl.replace(/\/$/, "");

  while (!options.signal.aborted) {
    const after = options.getAfterSeq();
    const url = `${baseUrl}/sessions/${options.sessionId}/events?after=${after}`;
    try {
      const res = await fetchImpl(url, {
        headers: { Accept: "text/event-stream" },
        signal: options.signal,
      });
      if (!res.ok || !res.body) {
        throw new Error(`sse status ${res.status}`);
      }
      await readSSEStream(res.body, options.onEvent, options.signal);
    } catch (err) {
      if (options.signal.aborted) {
        return;
      }
      const name = err instanceof Error ? err.name : "";
      if (name === "AbortError") {
        return;
      }
    }
    if (options.signal.aborted) {
      return;
    }
    await sleep(retryDelayMs, options.signal);
  }
}

async function readSSEStream(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: AgentEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (!signal.aborted) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      buffer += decoder.decode(value, { stream: true });
      const parsed = parseSSEChunk(buffer);
      buffer = parsed.rest;
      for (const event of parsed.events) {
        onEvent(event);
      }
    }
  } finally {
    reader.releaseLock();
  }
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      clearTimeout(timer);
      resolve();
    };
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

import assert from "node:assert/strict";
import { test } from "node:test";

import { parseSSEBlock } from "./sse.ts";

test("parseSSEBlock reads the AgentEvent data frame", () => {
  const ev = parseSSEBlock(
    [
      "id: 12",
      "event: assistant.completed",
      'data: {"event_id":"e12","session_id":"s","run_id":"r","seq":12,"type":"assistant.completed","version":1,"occurred_at":"t","payload":{"message_id":"m","text":"done"}}',
    ].join("\n"),
  );
  assert.ok(ev);
  assert.equal(ev.seq, 12);
  assert.equal(ev.type, "assistant.completed");
});

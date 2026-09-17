import assert from "node:assert/strict";
import { test } from "node:test";

import { applyCodexEvent, applyOptimisticUser, emptyCodexState, hydrateCodex } from "./reducer.ts";
import { parseSSEBlock } from "./sse.ts";

test("hydrateCodex and applyCodexEvent cover turn, progress, ask, reset", () => {
  let state = hydrateCodex({
    session: { id: "th", thread_id: "th", archived: false, active_turn_id: "t1" },
    progress: [{ kind: "user", item_id: "u1", text: "hi" }],
    asks: [],
    settings: { model: "gpt-5.6" },
  });
  assert.equal(state.items[0]?.text, "hi");
  assert.equal(state.activeTurn?.id, "t1");
  assert.equal(state.settings.model, "gpt-5.6");

  state = applyCodexEvent(state, {
    seq: 1,
    type: "progress",
    session_id: "th",
    progress: { kind: "text", item_id: "m1", text: "hel" },
  });
  state = applyCodexEvent(state, {
    seq: 2,
    type: "progress",
    session_id: "th",
    progress: { kind: "text", item_id: "m1", text: "lo" },
  });
  const message = state.items.find((item) => item.id === "m1");
  assert.equal(message?.text, "hello");

  state = applyCodexEvent(state, {
    seq: 3,
    type: "ask.required",
    session_id: "th",
    ask: { id: "8", kind: "command", command: "ls", external_request_id: "8" },
  });
  assert.equal(state.asks.length, 1);
  state = applyCodexEvent(state, {
    seq: 4,
    type: "ask.resolved",
    session_id: "th",
    ask: { id: "8", kind: "command", command: "ls", external_request_id: "8" },
  });
  assert.equal(state.asks.length, 0);

  state = applyCodexEvent(state, {
    seq: 5,
    type: "turn.completed",
    session_id: "th",
    turn: { id: "t1", session_id: "th", status: "completed" },
  });
  assert.equal(state.activeTurn, null);

  state = applyCodexEvent(state, {
    seq: 6,
    type: "notice",
    session_id: "th",
    notice: "不支持",
  });
  state = applyCodexEvent(state, {
    seq: 7,
    type: "reset",
    session_id: "th",
    notice: "event gap",
  });
  assert.equal(state.reset, true);
  assert.equal(state.lastSeq, 7);
});

test("parseSSEBlock reads Codex event data", () => {
  const ev = parseSSEBlock(
    [
      "id: 3",
      "event: progress",
      'data: {"seq":3,"type":"progress","session_id":"th","progress":{"kind":"text","text":"hi"}}',
    ].join("\n"),
  );
  assert.ok(ev);
  assert.equal(ev.seq, 3);
  assert.equal(ev.type, "progress");
  assert.equal(ev.progress?.text, "hi");
});

test("user progress replaces the optimistic bubble", () => {
  let state = applyOptimisticUser(emptyCodexState(), "hi");
  state = applyCodexEvent(state, {
    seq: 1,
    type: "turn.started",
    session_id: "th",
    turn: { id: "t1", session_id: "th", status: "running" },
    progress: { kind: "user", item_id: "u1", text: "hi" },
  });
  const users = state.items.filter((item) => item.kind === "user");
  assert.equal(users.length, 1);
  assert.equal(users[0]?.id, "u1");
  assert.equal(state.activeTurn?.id, "t1");
});

test("duplicate user progress is ignored", () => {
  let state = applyOptimisticUser(emptyCodexState(), "hi");
  state = applyCodexEvent(state, {
    seq: 1,
    type: "progress",
    session_id: "th",
    progress: { kind: "user", item_id: "u1", text: "hi" },
  });
  state = applyCodexEvent(state, {
    seq: 2,
    type: "progress",
    session_id: "th",
    progress: { kind: "user", item_id: "u2", text: "hi" },
  });
  const users = state.items.filter((item) => item.kind === "user");
  assert.equal(users.length, 1);
  assert.equal(users[0]?.text, "hi");
});

test("emptyCodexState starts idle", () => {
  const state = emptyCodexState();
  assert.equal(state.items.length, 0);
  assert.equal(state.lastSeq, 0);
});

import assert from "node:assert/strict";
import { test } from "node:test";

import { parseSSEChunk } from "./sse.ts";
import type { AgentEvent, Message, TimelineItem } from "./types.ts";
import { decodeText, parseDelta } from "./content.ts";
import {
  applyEvent,
  applyOptimisticUser,
  emptyState,
  hydrate,
} from "./reducer.ts";

function ev(partial: Partial<AgentEvent> & Pick<AgentEvent, "seq" | "type">): AgentEvent {
  return {
    event_id: `e${partial.seq}`,
    session_id: "s1",
    run_id: "r1",
    version: 1,
    occurred_at: "2026-01-01T00:00:00Z",
    payload: {},
    ...partial,
  };
}

function kinds(items: TimelineItem[]): string[] {
  return items.map((item) => item.kind);
}

test("hydrate fills user text from messages and folds events in order", () => {
  const messages: Message[] = [
    {
      id: "m-user",
      session_id: "s1",
      run_id: "r1",
      role: "user",
      content: { text: "ping please" },
      event_seq: 1,
      created_at: "2026-01-01T00:00:00Z",
    },
  ];
  const events: AgentEvent[] = [
    ev({
      seq: 2,
      type: "run.created",
      payload: { trigger_message_id: "m-user", mode: "ask_for_approval", status: "queued" },
    }),
    ev({
      seq: 3,
      type: "run.state_changed",
      payload: { from: "queued", to: "running_llm", reason: "" },
    }),
    ev({
      seq: 4,
      type: "assistant.started",
      payload: { message_id: "m-a" },
    }),
    ev({
      seq: 5,
      type: "assistant.delta",
      payload: { message_id: "m-a", delta: { text: "hello " } },
    }),
    ev({
      seq: 6,
      type: "assistant.delta",
      payload: { message_id: "m-a", delta: { text: "world" } },
    }),
    ev({
      seq: 7,
      type: "assistant.completed",
      payload: { message_id: "m-a", text: "hello world" },
    }),
    ev({
      seq: 8,
      type: "run.completed",
      payload: { status: "completed", stop_reason: "completed" },
    }),
  ];

  const state = hydrate(messages, events);
  assert.equal(state.lastSeq, 8);
  assert.equal(state.activeRunId, null);
  const user = state.items.find((item) => item.kind === "user");
  assert.equal(user?.kind === "user" && user.text, "ping please");
  const assistant = state.items.find((item) => item.kind === "assistant");
  assert.ok(assistant && assistant.kind === "assistant");
  assert.equal(assistant.text, "hello world");
  assert.equal(assistant.streaming, false);
  assert.equal(state.items.some((item) => item.kind === "thinking"), false);
  assert.deepEqual(kinds(state.items), ["user", "assistant", "terminal"]);
});

test("hydrate interleaves turns instead of grouping users then assistants", () => {
  const messages: Message[] = [
    {
      id: "m1",
      session_id: "s1",
      run_id: "r1",
      role: "user",
      content: { text: "first" },
      event_seq: 1,
      created_at: "2026-01-01T00:00:00Z",
    },
    {
      id: "m2",
      session_id: "s1",
      run_id: "r2",
      role: "user",
      content: { text: "second" },
      event_seq: 10,
      created_at: "2026-01-01T00:01:00Z",
    },
  ];
  const events: AgentEvent[] = [
    ev({
      seq: 2,
      run_id: "r1",
      type: "run.created",
      payload: { trigger_message_id: "m1", mode: "yolo", status: "queued" },
    }),
    ev({
      seq: 3,
      run_id: "r1",
      type: "assistant.started",
      payload: { message_id: "a1" },
    }),
    ev({
      seq: 4,
      run_id: "r1",
      type: "assistant.completed",
      payload: { message_id: "a1", text: "reply1" },
    }),
    ev({
      seq: 5,
      run_id: "r1",
      type: "run.completed",
      payload: { status: "completed" },
    }),
    ev({
      seq: 11,
      run_id: "r2",
      type: "run.created",
      payload: { trigger_message_id: "m2", mode: "yolo", status: "queued" },
    }),
    ev({
      seq: 12,
      run_id: "r2",
      type: "assistant.started",
      payload: { message_id: "a2" },
    }),
    ev({
      seq: 13,
      run_id: "r2",
      type: "assistant.completed",
      payload: { message_id: "a2", text: "reply2" },
    }),
    ev({
      seq: 14,
      run_id: "r2",
      type: "run.completed",
      payload: { status: "completed" },
    }),
  ];

  const state = hydrate(messages, events);
  const spoken = state.items
    .filter((item) => item.kind === "user" || item.kind === "assistant")
    .map((item) => (item.kind === "user" || item.kind === "assistant" ? item.text : ""));
  assert.deepEqual(spoken, ["first", "reply1", "second", "reply2"]);
  assert.deepEqual(kinds(state.items), [
    "user",
    "assistant",
    "terminal",
    "user",
    "assistant",
    "terminal",
  ]);
});

test("applyEvent skips duplicate seq on reconnect", () => {
  let state = emptyState();
  const created = ev({
    seq: 1,
    type: "run.created",
    payload: { trigger_message_id: "m1", mode: "yolo", status: "queued" },
  });
  state = applyEvent(state, created);
  state = applyEvent(state, created);
  state = applyEvent(state, ev({ seq: 1, type: "run.completed", payload: { status: "completed" } }));
  assert.equal(state.lastSeq, 1);
  assert.equal(state.items.filter((item) => item.kind === "user").length, 0);
  assert.equal(state.items.some((item) => item.kind === "terminal"), false);
});

test("run.created without message text does not add an empty user bubble", () => {
  const state = applyEvent(
    emptyState(),
    ev({
      seq: 1,
      type: "run.created",
      payload: { trigger_message_id: "m1", mode: "yolo", status: "queued" },
    }),
  );
  assert.equal(state.items.filter((item) => item.kind === "user").length, 0);
});

test("run.created before optimistic keeps a single user bubble with text", () => {
  let state = applyEvent(
    emptyState(),
    ev({
      seq: 1,
      type: "run.created",
      payload: { trigger_message_id: "m-real", mode: "auto_approve", status: "queued" },
    }),
  );
  state = applyOptimisticUser(state, { runId: "r1", text: "hi" });
  const users = state.items.filter((item) => item.kind === "user");
  assert.equal(users.length, 1);
  assert.equal(users[0]?.kind === "user" && users[0].text, "hi");
});

test("run.created after local optimistic uses pending text", () => {
  let state = applyOptimisticUser(emptyState(), { runId: "local", text: "hi" });
  state = applyEvent(
    state,
    ev({
      seq: 1,
      type: "run.created",
      payload: { trigger_message_id: "m-real", mode: "auto_approve", status: "queued" },
    }),
  );
  const users = state.items.filter((item) => item.kind === "user");
  assert.equal(users.length, 1);
  assert.equal(users[0]?.kind === "user" && users[0].messageId, "m-real");
  assert.equal(users[0]?.kind === "user" && users[0].text, "hi");
});

test("assistant deltas concatenate and hide thinking after text", () => {
  let state = applyEvent(
    emptyState(),
    ev({
      seq: 1,
      type: "run.state_changed",
      payload: { from: "queued", to: "running_llm", reason: "" },
    }),
  );
  assert.equal(state.items.some((item) => item.kind === "thinking"), true);
  state = applyEvent(
    state,
    ev({ seq: 2, type: "assistant.started", payload: { message_id: "a1" } }),
  );
  state = applyEvent(
    state,
    ev({ seq: 3, type: "assistant.delta", payload: { message_id: "a1", delta: { text: "A" } } }),
  );
  assert.equal(state.items.some((item) => item.kind === "thinking"), false);
  const assistant = state.items.find((item) => item.kind === "assistant");
  assert.ok(assistant && assistant.kind === "assistant");
  assert.equal(assistant.text, "A");
  assert.equal(assistant.streaming, true);
});

test("tool and approval lifecycle", () => {
  let state = emptyState();
  state = applyEvent(
    state,
    ev({
      seq: 1,
      type: "tool.call_started",
      payload: { call_id: "c1", name: "ping", arguments: { value: 1 } },
    }),
  );
  state = applyEvent(
    state,
    ev({
      seq: 2,
      type: "tool.approval_required",
      payload: {
        approval_id: "ap1",
        tool_calls: [{ id: "c1", name: "ping", arguments: { value: 1 } }],
      },
    }),
  );
  let approval = state.items.find((item) => item.kind === "approval");
  assert.ok(approval && approval.kind === "approval");
  assert.equal(approval.status, "pending");
  assert.equal(approval.toolCalls.length, 1);

  state = applyEvent(
    state,
    ev({
      seq: 3,
      type: "tool.approval_decided",
      payload: {
        approval_id: "ap1",
        status: "approved",
        scope: "once",
        decisions: [{ tool_call_id: "c1", status: "approved" }],
        tool_calls: [{ id: "c1", name: "ping", status: "approved" }],
      },
    }),
  );
  approval = state.items.find((item) => item.kind === "approval");
  assert.ok(approval && approval.kind === "approval");
  assert.equal(approval.status, "approved");

  state = applyEvent(
    state,
    ev({
      seq: 4,
      type: "tool.execution_started",
      payload: { call_id: "c1", name: "ping" },
    }),
  );
  state = applyEvent(
    state,
    ev({
      seq: 5,
      type: "tool.execution_result",
      payload: { call_id: "c1", name: "ping", success: true, output: { pong: true } },
    }),
  );
  const tool = state.items.find((item) => item.kind === "tool");
  assert.ok(tool && tool.kind === "tool");
  assert.equal(tool.state, "completed");
  assert.deepEqual(tool.output, { pong: true });
});

test("denied approval marks the tool denied", () => {
  let state = applyEvent(
    emptyState(),
    ev({
      seq: 1,
      type: "tool.approval_required",
      payload: { approval_id: "ap1", tool_calls: [{ id: "c1", name: "memory_write" }] },
    }),
  );
  state = applyEvent(
    state,
    ev({
      seq: 2,
      type: "tool.approval_decided",
      payload: {
        approval_id: "ap1",
        status: "denied",
        scope: "once",
        decisions: [{ tool_call_id: "c1", status: "denied", reason: "no" }],
        tool_calls: [{ id: "c1", name: "memory_write", status: "denied" }],
      },
    }),
  );
  const tool = state.items.find((item) => item.kind === "tool");
  assert.ok(tool && tool.kind === "tool");
  assert.equal(tool.state, "denied");
});

test("optimistic user is replaced when run.created arrives", () => {
  let state = applyOptimisticUser(emptyState(), { runId: "r1", text: "hi" });
  assert.equal(state.items[0]?.kind === "user" && state.items[0].messageId, "pending:r1");
  state = applyEvent(
    state,
    ev({
      seq: 4,
      type: "run.created",
      payload: { trigger_message_id: "m-real", mode: "auto_approve", status: "queued" },
    }),
  );
  const users = state.items.filter((item) => item.kind === "user");
  assert.equal(users.length, 1);
  assert.equal(users[0]?.kind === "user" && users[0].messageId, "m-real");
  assert.equal(users[0]?.kind === "user" && users[0].text, "hi");
});

test("context compacted becomes a timeline item", () => {
  const state = applyEvent(
    emptyState(),
    ev({
      seq: 9,
      type: "context.compacted",
      payload: { checkpoint_id: "cp1", base_event_seq: 3 },
    }),
  );
  assert.equal(state.items[0]?.kind, "context");
});

test("parseSSEChunk yields events and keeps a partial tail", () => {
  const chunk = [
    "id: 1",
    "event: run.created",
    'data: {"event_id":"e1","session_id":"s1","run_id":"r1","seq":1,"type":"run.created","version":1,"occurred_at":"t","payload":{}}',
    "",
    "id: 2",
    "event: assistant.delta",
    'data: {"event_id":"e2"',
  ].join("\n");
  const parsed = parseSSEChunk(chunk);
  assert.equal(parsed.events.length, 1);
  assert.equal(parsed.events[0]?.seq, 1);
  assert.match(parsed.rest, /assistant.delta/);
});

test("decodeText and parseDelta accept backend payloads", () => {
  assert.equal(decodeText({ text: "hi" }), "hi");
  assert.equal(decodeText('{"text":"hi"}'), "hi");
  assert.equal(decodeText('{"text":""}'), "");
  assert.deepEqual(parseDelta({ text: "x" }), { kind: "text", text: "x" });
  assert.equal(parseDelta({ id: "c1", name: "ping" }).kind, "tool");
});

test("assistant.completed with encoded empty text does not keep a JSON blob", () => {
  let state = applyEvent(
    emptyState(),
    ev({ seq: 1, type: "assistant.started", payload: { message_id: "a1" } }),
  );
  state = applyEvent(
    state,
    ev({
      seq: 2,
      type: "assistant.completed",
      payload: { message_id: "a1", text: '{"text":""}', tool_calls: [{ id: "c1", name: "ping" }] },
    }),
  );
  const assistant = state.items.find((item) => item.kind === "assistant");
  assert.ok(assistant && assistant.kind === "assistant");
  assert.equal(assistant.text, "");
});

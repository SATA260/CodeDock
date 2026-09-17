import assert from "node:assert/strict";
import { test } from "node:test";

import { ClaudeClient, ClaudeClientError } from "./client.ts";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

test("ClaudeClient covers every /claude HTTP route", async () => {
  const calls: { url: string; method: string; body?: unknown }[] = [];
  const client = new ClaudeClient({
    baseUrl: "http://api.test/",
    userId: "local",
    fetch: async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const raw = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body: raw ? JSON.parse(raw) : undefined });
      if (url.endsWith("/claude/status")) {
        return json({ available: true, authorized: true, version: "2.1.0", hint: "" });
      }
      if (url.endsWith("/claude/models")) {
        return json({
          models: [
            { id: "sonnet", efforts: ["low"], default_effort: "medium", hidden: false, is_default: true },
          ],
        });
      }
      if (url.endsWith("/claude/modes")) {
        return json({ modes: [{ id: "default", kind: "permission" }] });
      }
      if (url.endsWith("/claude/commands")) {
        return json({ commands: [{ name: "mcp", action: "hint", hint: "到终端改 Claude 配置" }] });
      }
      if (url.endsWith("/claude/sessions") && method === "GET") {
        return json({
          sessions: [
            { id: "s1", claude_session_id: "", title: "t", active_turn_id: "", archived: false },
          ],
        });
      }
      if (url.endsWith("/claude/sessions") && method === "POST") {
        return json({
          session: { id: "s2", claude_session_id: "", title: "", active_turn_id: "", archived: false },
        });
      }
      if (url.endsWith("/claude/sessions/s1") && method === "GET") {
        return json({
          session: { id: "s1", claude_session_id: "c1", title: "t", active_turn_id: "t1", archived: false },
        });
      }
      if (url.endsWith("/claude/sessions/s1") && method === "PATCH") {
        return json({ ok: true });
      }
      if (url.endsWith("/claude/sessions/s1/archive")) {
        return json({ ok: true });
      }
      if (url.endsWith("/claude/sessions/s1/fork")) {
        return json({
          session: { id: "s3", claude_session_id: "c3", title: "t", active_turn_id: "", archived: false },
        });
      }
      if (url.endsWith("/claude/sessions/s1/settings") && method === "GET") {
        return json({
          model: "sonnet",
          effort: "medium",
          permission_mode: "default",
          cwd: "/repo",
          overridden: [],
        });
      }
      if (url.endsWith("/claude/sessions/s1/settings") && method === "POST") {
        return json({
          model: "opus",
          effort: "high",
          permission_mode: "plan",
          cwd: "/repo",
          overridden: ["model"],
        });
      }
      if (url.endsWith("/claude/sessions/s1/commands")) {
        return json({ hint: "到终端改 Claude 配置" });
      }
      if (url.endsWith("/claude/sessions/s1/mentions") || url.endsWith("/claude/sessions/s1/images")) {
        return json({ ok: true });
      }
      if (url.endsWith("/claude/sessions/s1/turns")) {
        return json({ turn_id: "turn-1" });
      }
      if (url.endsWith("/claude/sessions/s1/transcript")) {
        return json({
          items: [{ kind: "text", text: "hi", command: "", paths: [], diff: "" }],
          usage: { used: 170, window: 200000 },
        });
      }
      if (url.endsWith("/claude/turns/turn-1/cancel") || url.endsWith("/claude/turns/turn-1/continue")) {
        return json({ ok: true });
      }
      if (url.endsWith("/claude/approvals/a1/decision")) {
        return json({ ok: true });
      }
      if (url.endsWith("/claude/asks/r1/reject-unknown")) {
        return json({ ok: true });
      }
      return json({ error: "missing mock" }, 500);
    },
  });

  const status = await client.probe();
  assert.equal(status.available, true);
  const models = await client.listModels();
  assert.equal(models[0]?.id, "sonnet");
  const modes = await client.listModes();
  assert.equal(modes[0]?.id, "default");
  const commands = await client.listCommands();
  assert.equal(commands[0]?.name, "mcp");
  const sessions = await client.listSessions();
  assert.equal(sessions[0]?.id, "s1");
  const created = await client.createSession();
  assert.equal(created.id, "s2");
  const got = await client.getSession("s1");
  assert.equal(got.claude_session_id, "c1");
  await client.renameSession("s1", "hello");
  await client.archiveSession("s1");
  const forked = await client.forkSession("s1");
  assert.equal(forked.id, "s3");
  const settings = await client.getSettings("s1");
  assert.equal(settings.model, "sonnet");
  const applied = await client.applySettings("s1", { model: "opus", effort: "high", permission_mode: "plan" });
  assert.equal(applied.model, "opus");
  const hint = await client.invoke("s1", "mcp", "");
  assert.equal(hint.includes("终端"), true);
  await client.mention("s1", "a.go");
  await client.attachImage("s1", "a.png");
  const turnId = await client.startTurn("s1", {
    content: "hi",
    input: { text: "hi", mentions: ["a.go"], images: [] },
    mode: "start",
  });
  assert.equal(turnId, "turn-1");
  const transcript = await client.hydrate("s1");
  assert.equal(transcript.items[0]?.kind, "text");
  assert.equal(transcript.usage?.used, 170);
  assert.equal(transcript.usage?.window, 200000);
  await client.cancelTurn("turn-1");
  await client.continueTurn("turn-1");
  await client.decide("a1", { approved: true, scope: "once", choice: "a", values: ["n"] });
  await client.rejectUnknown("r1", "s1", "t1");

  const paths = calls.map((call) => `${call.method} ${call.url.replace("http://api.test", "")}`);
  assert.deepEqual(paths, [
    "GET /claude/status",
    "GET /claude/models",
    "GET /claude/modes",
    "GET /claude/commands",
    "GET /claude/sessions",
    "POST /claude/sessions",
    "GET /claude/sessions/s1",
    "PATCH /claude/sessions/s1",
    "POST /claude/sessions/s1/archive",
    "POST /claude/sessions/s1/fork",
    "GET /claude/sessions/s1/settings",
    "POST /claude/sessions/s1/settings",
    "POST /claude/sessions/s1/commands",
    "POST /claude/sessions/s1/mentions",
    "POST /claude/sessions/s1/images",
    "POST /claude/sessions/s1/turns",
    "GET /claude/sessions/s1/transcript",
    "POST /claude/turns/turn-1/cancel",
    "POST /claude/turns/turn-1/continue",
    "POST /claude/approvals/a1/decision",
    "POST /claude/asks/r1/reject-unknown",
  ]);
  assert.deepEqual(calls[5]?.body, { user_id: "local" });
  assert.deepEqual(calls[7]?.body, { title: "hello" });
  assert.deepEqual(calls[11]?.body, {
    model: "opus",
    effort: "high",
    permission_mode: "plan",
    cwd: "",
    overridden: [],
  });
  assert.deepEqual(calls[12]?.body, { name: "mcp", args: "" });
  assert.deepEqual(calls[13]?.body, { path: "a.go" });
  assert.deepEqual(calls[14]?.body, { path: "a.png" });
  assert.deepEqual(calls[15]?.body, {
    content: "hi",
    input: { text: "hi", mentions: ["a.go"], images: [] },
    mode: "start",
  });
  assert.deepEqual(calls[19]?.body, { approved: true, scope: "once", choice: "a", values: ["n"] });
  assert.deepEqual(calls[20]?.body, { session_id: "s1", turn_id: "t1" });
});

test("ClaudeClient surfaces HTTP error", async () => {
  const client = new ClaudeClient({
    baseUrl: "http://api.test",
    userId: "local",
    fetch: async () => json({ error: "unavailable" }, 503),
  });
  await assert.rejects(() => client.probe(), (err: unknown) => {
    assert.ok(err instanceof ClaudeClientError);
    assert.equal(err.status, 503);
    assert.equal(err.message, "unavailable");
    return true;
  });
});

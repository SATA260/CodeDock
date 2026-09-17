import assert from "node:assert/strict";
import { test } from "node:test";

import { CodexClient, CodexClientError } from "./client.ts";

test("CodexClient hits every /codex route", async () => {
  const calls: { url: string; method: string; body?: unknown; headers?: string }[] = [];
  const client = new CodexClient({
    baseUrl: "http://api.test/",
    fetch: async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const raw = typeof init?.body === "string" ? init.body : undefined;
      calls.push({
        url,
        method,
        body: raw ? JSON.parse(raw) : undefined,
        headers: new Headers(init?.headers).get("Last-Event-ID") ?? undefined,
      });
      if (url.endsWith("/codex/status")) {
        return json({ available: true, authorized: true, version: "0.149.0" });
      }
      if (url.endsWith("/codex/models")) {
        return json({ models: [{ id: "gpt-5.6", efforts: ["low"], default_effort: "low", hidden: false, is_default: true }] });
      }
      if (url.endsWith("/codex/modes")) {
        return json({ modes: [{ id: "plan", kind: "collaboration", allowed: true }] });
      }
      if (url.endsWith("/codex/commands")) {
        return json({ commands: [{ name: "mcp", action: "hint", hint: "终端" }] });
      }
      if (url.includes("/codex/sessions?") || url.endsWith("/codex/sessions")) {
        if (method === "POST") {
          return json({ session: { id: "th1", thread_id: "th1", archived: false } }, 201);
        }
        return json({ sessions: [{ id: "th1", thread_id: "th1", archived: false }], next_cursor: "c2" });
      }
      if (url.endsWith("/codex/sessions/th1")) {
        if (method === "PATCH") {
          return json({ ok: "true" });
        }
        return json({
          session: { id: "th1", thread_id: "th1", archived: false },
          progress: [{ kind: "user", text: "hi" }],
          asks: [],
        });
      }
      if (url.endsWith("/fork")) {
        return json({ session: { id: "th2", thread_id: "th2", archived: false } }, 201);
      }
      if (url.endsWith("/settings")) {
        return json({ settings: { model: "gpt-5.6", effort: "high" } });
      }
      if (url.endsWith("/commands")) {
        return json({ handled: true, hint: "终端", action: "hint" });
      }
      if (url.endsWith("/turns")) {
        return json({ turn: { id: "t1", session_id: "th1", status: "running" } }, 202);
      }
      if (url.includes("/asks/") && url.endsWith("/decision")) {
        return json({ ok: "true" });
      }
      if (url.includes("/asks/") && url.endsWith("/expire")) {
        return json({ ok: "true" });
      }
      if (url.endsWith("/asks")) {
        return json({ asks: [{ id: "8", kind: "command", command: "ls", external_request_id: "8" }] });
      }
      if (url.includes("/turns/") && url.endsWith("/interrupt")) {
        return json({ ok: "true" });
      }
      return json({ ok: "true" });
    },
  });

  const status = await client.status();
  assert.equal(status.version, "0.149.0");
  assert.equal((await client.listModels())[0]?.id, "gpt-5.6");
  assert.equal((await client.listModes())[0]?.id, "plan");
  assert.equal((await client.listCommands())[0]?.name, "mcp");
  const page = await client.listSessions({ archived: true, cursor: "abc" });
  assert.equal(page.next_cursor, "c2");
  const created = await client.createSession({ cwd: "/tmp" });
  assert.equal(created.id, "th1");
  const detail = await client.getSession("th1");
  assert.equal(detail.progress[0]?.text, "hi");
  await client.renameSession("th1", "Hi");
  const forked = await client.forkSession("th1");
  assert.equal(forked.id, "th2");
  await client.archiveSession("th1");
  await client.compactSession("th1");
  await client.reviewSession("th1");
  assert.equal((await client.getSettings("th1")).model, "gpt-5.6");
  assert.equal((await client.applySettings("th1", { effort: "high" })).effort, "high");
  assert.equal((await client.invokeCommand("th1", "mcp")).handled, true);
  const turn = await client.startTurn("th1", { content: "hello", mode: "queue" });
  assert.equal(turn.id, "t1");
  await client.mention("th1", "a.go");
  await client.attachImage("th1", "a.png");
  assert.equal((await client.listAsks("th1"))[0]?.id, "8");
  await client.interruptTurn("t1", "th1");
  await client.decideAsk("8", { approved: true, scope: "once" });
  await client.expireAsk("8");
  assert.equal(client.eventsUrl("th1", 3), "http://api.test/codex/sessions/th1/events?after=3");

  const urls = calls.map((call) => `${call.method} ${call.url}`);
  assert.deepEqual(urls, [
    "GET http://api.test/codex/status",
    "GET http://api.test/codex/models",
    "GET http://api.test/codex/modes",
    "GET http://api.test/codex/commands",
    "GET http://api.test/codex/sessions?archived=true&cursor=abc",
    "POST http://api.test/codex/sessions",
    "GET http://api.test/codex/sessions/th1",
    "PATCH http://api.test/codex/sessions/th1",
    "POST http://api.test/codex/sessions/th1/fork",
    "POST http://api.test/codex/sessions/th1/archive",
    "POST http://api.test/codex/sessions/th1/compact",
    "POST http://api.test/codex/sessions/th1/review",
    "GET http://api.test/codex/sessions/th1/settings",
    "POST http://api.test/codex/sessions/th1/settings",
    "POST http://api.test/codex/sessions/th1/commands",
    "POST http://api.test/codex/sessions/th1/turns",
    "POST http://api.test/codex/sessions/th1/attachments/mention",
    "POST http://api.test/codex/sessions/th1/attachments/image",
    "GET http://api.test/codex/sessions/th1/asks",
    "POST http://api.test/codex/turns/t1/interrupt",
    "POST http://api.test/codex/asks/8/decision",
    "POST http://api.test/codex/asks/8/expire",
  ]);
  assert.deepEqual(calls[5]?.body, { settings: { cwd: "/tmp" } });
  assert.deepEqual(calls[7]?.body, { title: "Hi" });
  assert.deepEqual(calls[15]?.body, { content: "hello", input: {}, mode: "queue" });
  assert.deepEqual(calls[19]?.body, { session_id: "th1" });
  assert.deepEqual(calls[20]?.body, { approved: true, scope: "once" });
});

test("CodexClient maps error JSON", async () => {
  const client = new CodexClient({
    baseUrl: "http://api.test",
    fetch: async () =>
      new Response(JSON.stringify({ error: "codex is not authorized" }), {
        status: 401,
        headers: { "Content-Type": "application/json" },
      }),
  });
  await assert.rejects(() => client.createSession(), (err: unknown) => {
    assert.ok(err instanceof CodexClientError);
    assert.equal(err.status, 401);
    assert.equal(err.message, "codex is not authorized");
    return true;
  });
});

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

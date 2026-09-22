import assert from "node:assert/strict";
import { test } from "node:test";

import { BoardClient, BoardClientError } from "./client.ts";

test("BoardClient hits /works /board and inbox routes", async () => {
  const calls: { url: string; method: string; body?: unknown }[] = [];
  const client = new BoardClient({
    baseUrl: "http://api.test/",
    userId: "u1",
    tenantId: "t1",
    fetch: async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const raw = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body: raw ? JSON.parse(raw) : undefined });
      if (url.includes("/board?") && method === "GET") {
        return json({ board: { cards: [{ work: { id: "w1", title: "修登录" }, sessions: [] }], ungrouped: [] } });
      }
      if (url.endsWith("/works") && method === "POST") {
        return json({ work: { id: "w1", title: "修登录", user_id: "u1", tenant_id: "t1" } });
      }
      if (url.includes("/works/w1/sessions")) {
        return json({
          placement: { engine: "native", session_id: "s1", work_id: "w1", checkout: "" },
          session_id: "s1",
          engine: "agent",
        });
      }
      if (url.includes("/works/w1/inbox")) {
        return json({ items: [{ engine: "agent", session_id: "s1", ticket_id: "a1", summary: "tool" }] });
      }
      if (url.includes("/inbox/decision")) {
        return json({ ok: true });
      }
      if (url.includes("/placements?")) {
        return json({ placements: [{ engine: "native", session_id: "s1", work_id: "w1", checkout: "" }] });
      }
      return json({ ok: true });
    },
  });

  const work = await client.createWork("修登录");
  assert.equal(work.id, "w1");
  const board = await client.getBoard();
  assert.equal(board.cards[0]?.work.title, "修登录");
  const started = await client.startSession("w1", { engine: "agent", kind: "talk" });
  assert.equal(started.session_id, "s1");
  const items = await client.listInbox("w1");
  assert.equal(items[0]?.ticket_id, "a1");
  await client.decideInbox({ engine: "agent", ticket_id: "a1", status: "approved", actor_id: "u1" });
  const places = await client.listPlacements();
  assert.equal(places[0]?.engine, "agent");

  assert.equal(calls[0]?.url, "http://api.test/works");
  assert.match(String(calls[1]?.url), /user_id=u1/);
  assert.equal(calls.some((item) => item.url.endsWith("/inbox/decision")), true);
  const saved = await client.replaceLinks("agent", "s1", ["https://github.com/acme/app/pull/8"]);
  assert.equal(saved.pulls.length, 0);
  assert.equal(calls.at(-1)?.method, "PUT");
  assert.deepEqual(calls.at(-1)?.body, { links: ["https://github.com/acme/app/pull/8"] });

  await assert.rejects(
    () =>
      new BoardClient({
        baseUrl: "http://api.test",
        userId: "u1",
        fetch: async () => json({ error: "no" }, 400),
      }).createWork("x"),
    (err: unknown) => err instanceof BoardClientError && err.status === 400,
  );
});

// json 造一条假响应。
function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

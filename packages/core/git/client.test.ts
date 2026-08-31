import assert from "node:assert/strict";
import { test } from "node:test";

import { GitClient, GitClientError } from "./client.ts";

test("GitClient status and mutations hit /git routes", async () => {
  const calls: { url: string; method: string; body?: unknown }[] = [];
  const client = new GitClient({
    baseUrl: "http://api.test/",
    fetch: async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const raw = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body: raw ? JSON.parse(raw) : undefined });
      if (url.endsWith("/git/status")) {
        return json({
          path: "/repo",
          is_repo: true,
          empty: false,
          branch: "main",
          head: "abc",
          detached: false,
          upstream: "",
          ahead: 0,
          behind: 0,
          upstream_gone: false,
          integrating: "",
          default_branch: "",
          files: [],
          remotes: [],
        });
      }
      if (url.endsWith("/git/commit")) {
        return json({
          commit: { id: "def", parents: ["abc"], title: "add a", body: "", author: "t", date: "" },
        });
      }
      if (url.includes("/git/branches")) {
        return json({
          current: "main",
          locals: [],
          remotes: [],
          graph: { nodes: [], edges: [] },
        });
      }
      if (url.includes("/git/log")) {
        return json({
          commits: [{ id: "abc", parents: [], title: "first", body: "", author: "t", date: "2026-08-30T00:00:00Z" }],
        });
      }
      if (url.includes("/git/diff")) {
        return json({
          files: [
            {
              path: "a.txt",
              orig_path: "",
              kind: "modified",
              binary: false,
              patch: "@@ -1 +1 @@\n-old\n+new\n",
            },
          ],
        });
      }
      if (url.includes("/git/commit-message/prompt")) {
        return json({
          presets: [{ id: "conventional", name: "Conventional", system_prompt: "写说明" }],
          selected: "conventional",
          custom: "",
          system_prompt: "写说明",
        });
      }
      if (url.includes("/git/commit-message/generate")) {
        return json({ title: "add a", body: "details" });
      }
      return json({ ok: true });
    },
  });

  const state = await client.status();
  assert.equal(state.branch, "main");
  await client.stage(["a.txt"]);
  await client.unstage(["a.txt"], "/wt");
  await client.discard(["a.txt"]);
  const commit = await client.commit({ message: "add a", paths: ["a.txt"] });
  assert.equal(commit.id, "def");
  await client.push();
  const view = await client.listBranches("/wt");
  assert.equal(view.current, "main");
  await client.createBranch("feature", "origin/feature");
  await client.switchBranch("feature", "/wt");
  const diffs = await client.diff("worktree");
  assert.equal(diffs[0]?.path, "a.txt");
  const commits = await client.log(20, "/wt");
  assert.equal(commits[0]?.title, "first");
  const prompt = await client.messagePrompt("/wt");
  assert.equal(prompt.selected, "conventional");
  const saved = await client.saveMessagePrompt({ selected: "custom", custom: "写短标题" });
  assert.equal(saved.selected, "conventional");
  const draft = await client.generateMessage();
  assert.equal(draft.title, "add a");
  assert.equal(draft.body, "details");

  assert.equal(calls[0]?.url, "http://api.test/git/status");
  assert.equal(calls[1]?.url, "http://api.test/git/stage");
  assert.deepEqual(calls[1]?.body, { paths: ["a.txt"], checkout: "" });
  assert.equal(calls[2]?.url, "http://api.test/git/unstage");
  assert.deepEqual(calls[2]?.body, { paths: ["a.txt"], checkout: "/wt" });
  assert.equal(calls[3]?.url, "http://api.test/git/discard");
  assert.deepEqual(calls[3]?.body, { paths: ["a.txt"], checkout: "" });
  assert.equal(calls[5]?.url, "http://api.test/git/push");
  assert.deepEqual(calls[5]?.body, { checkout: "" });
  assert.equal(calls[6]?.url, "http://api.test/git/branches?checkout=%2Fwt");
  assert.equal(calls[7]?.url, "http://api.test/git/branches");
  assert.deepEqual(calls[7]?.body, { name: "feature", start: "origin/feature", checkout: "" });
  assert.equal(calls[8]?.url, "http://api.test/git/branches/switch");
  assert.deepEqual(calls[8]?.body, { name: "feature", checkout: "/wt" });
  assert.equal(calls[9]?.url, "http://api.test/git/diff?scope=worktree");
  assert.equal(calls[10]?.url, "http://api.test/git/log?limit=20&checkout=%2Fwt");
  assert.equal(calls[11]?.url, "http://api.test/git/commit-message/prompt?checkout=%2Fwt");
  assert.equal(calls[12]?.url, "http://api.test/git/commit-message/prompt");
  assert.equal(calls[12]?.method, "PUT");
  assert.deepEqual(calls[12]?.body, { selected: "custom", custom: "写短标题" });
  assert.equal(calls[13]?.url, "http://api.test/git/commit-message/generate");
  assert.deepEqual(calls[13]?.body, { checkout: "" });
});

test("GitClient maps error JSON", async () => {
  const client = new GitClient({
    baseUrl: "http://api.test",
    fetch: async () =>
      new Response(JSON.stringify({ error: "paths required" }), {
        status: 400,
        headers: { "Content-Type": "application/json" },
      }),
  });
  await assert.rejects(() => client.stage([]), (err: unknown) => {
    assert.ok(err instanceof GitClientError);
    assert.equal(err.status, 400);
    assert.equal(err.message, "paths required");
    return true;
  });
});

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

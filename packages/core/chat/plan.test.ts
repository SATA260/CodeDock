import assert from "node:assert/strict";
import { test } from "node:test";

import { isPlanTool, latestPlanDocIds, normalizePlanName, planPreviewFromTool, planToolDump } from "./plan.ts";

test("isPlanTool recognizes plan_* only", () => {
  assert.equal(isPlanTool("plan_list"), true);
  assert.equal(isPlanTool("plan_read"), true);
  assert.equal(isPlanTool("plan_write"), true);
  assert.equal(isPlanTool("read"), false);
});

test("plan_write previews arguments before output arrives", () => {
  const preview = planPreviewFromTool({
    name: "plan_write",
    arguments: { name: "notes", content: "# 目标\n\n写完预览" },
  });
  assert.deepEqual(preview, {
    kind: "doc",
    name: "notes.md",
    content: "# 目标\n\n写完预览",
    source: "write",
  });
});

test("plan_write prefers output name and content", () => {
  const preview = planPreviewFromTool({
    name: "plan_write",
    arguments: { name: "draft", content: "old" },
    output: { name: "notes.md", content: "new" },
  });
  assert.deepEqual(preview, {
    kind: "doc",
    name: "notes.md",
    content: "new",
    source: "write",
  });
});

test("plan_read uses output and ignores empty arguments", () => {
  const preview = planPreviewFromTool({
    name: "plan_read",
    arguments: { name: "notes.md" },
    output: { name: "notes.md", content: "## 步骤" },
  });
  assert.deepEqual(preview, {
    kind: "doc",
    name: "notes.md",
    content: "## 步骤",
    source: "read",
  });
});

test("plan_read without content still returns a named card", () => {
  const preview = planPreviewFromTool({
    name: "plan_read",
    arguments: { name: "notes.md" },
  });
  assert.deepEqual(preview, {
    kind: "doc",
    name: "notes.md",
    content: "",
    source: "read",
  });
});

test("plan_list is not previewed", () => {
  assert.equal(
    planPreviewFromTool({ name: "plan_list", output: { names: ["a.md", "b.md"] } }),
    null,
  );
});

test("unknown tools have no preview", () => {
  assert.equal(planPreviewFromTool({ name: "read", arguments: { path: "a.ts" } }), null);
});

test("parses JSON string payloads", () => {
  const preview = planPreviewFromTool({
    name: "plan_write",
    arguments: '{"name":"notes.md","content":"hello"}',
  });
  assert.deepEqual(preview, {
    kind: "doc",
    name: "notes.md",
    content: "hello",
    source: "write",
  });
});

test("planToolDump strips document content but keeps other tools intact", () => {
  assert.deepEqual(
    planToolDump({
      name: "plan_write",
      arguments: { name: "notes.md", content: "# long" },
      output: { name: "notes.md", content: "# long" },
    }),
    {
      input: { name: "notes.md" },
      output: { name: "notes.md" },
    },
  );
  const ping = { name: "ping", arguments: { value: 1 }, output: { pong: true } };
  assert.deepEqual(planToolDump(ping), { input: ping.arguments, output: ping.output });
});

test("normalizePlanName adds .md and keeps basename", () => {
  assert.equal(normalizePlanName("notes"), "notes.md");
  assert.equal(normalizePlanName(".cursor/notes.md"), "notes.md");
});

test("write to .cursor/*.md is a plan preview; other writes are not", () => {
  assert.deepEqual(
    planPreviewFromTool({
      name: "write",
      arguments: { path: ".cursor/preview-smoke.md", content: "# 新稿" },
    }),
    {
      kind: "doc",
      name: "preview-smoke.md",
      content: "# 新稿",
      source: "write",
    },
  );
  assert.equal(
    planPreviewFromTool({ name: "write", arguments: { path: "main.go", content: "package main" } }),
    null,
  );
});

test("latestPlanDocIds keeps only the current plan", () => {
  const ids = latestPlanDocIds([
    { id: "t1", name: "plan_write", arguments: { name: "notes.md", content: "v1" } },
    { id: "t2", name: "plan_list", output: { names: ["notes.md", "other.md"] } },
    { id: "t3", name: "plan_write", arguments: { name: "other.md", content: "x" } },
  ]);
  assert.deepEqual([...ids], ["t3"]);
});

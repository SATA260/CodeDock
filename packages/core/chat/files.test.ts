import assert from "node:assert/strict";
import { test } from "node:test";

import {
  collectRunFileGroups,
  compactToolDump,
  diffViewHunks,
  fileChangeFromTool,
  fileKey,
  isFileChangeTool,
  unifiedPatch,
} from "./files.ts";

test("write to a regular file is a file change without computing a patch", () => {
  const got = fileChangeFromTool({
    name: "write",
    arguments: { path: "store.py", content: "print(1)" },
  });
  assert.equal(got?.path, "store.py");
  assert.equal(got?.content, "print(1)");
  assert.equal(got?.action, "write");
  assert.equal(got?.patch, "");
  assert.equal(got?.runId, "");
});

test("write prefers the server patch when details carry one", () => {
  const got = fileChangeFromTool({
    name: "write",
    arguments: { path: "store.py", content: "print(1)" },
    output: { details: { patch: "--- a/store.py\n+++ b/store.py\n@@ -1 +1 @@\n-old\n+print(1)\n" } },
  });
  assert.match(got?.patch ?? "", /^-old$/m);
});

test("write to .cursor plan is not a file change", () => {
  assert.equal(
    fileChangeFromTool({
      name: "write",
      arguments: { path: ".cursor/task.md", content: "# plan" },
    }),
    null,
  );
});

test("edit collects replacement text without computing a patch", () => {
  const got = fileChangeFromTool({
    name: "edit",
    arguments: {
      path: "server.py",
      edits: [{ oldText: "a", newText: "b" }],
    },
  });
  assert.equal(got?.path, "server.py");
  assert.equal(got?.action, "edit");
  assert.equal(got?.content, "b");
  assert.equal(got?.patch, "");
});

test("compactToolDump strips file bodies and patches", () => {
  assert.deepEqual(
    compactToolDump({
      name: "write",
      arguments: { path: "a.py", content: "huge" },
      output: { content: [{ type: "text", text: "ok" }], details: { patch: "@@", extra: 1 } },
    }),
    { input: { path: "a.py" }, output: { details: { extra: 1 } } },
  );
});

test("isFileChangeTool only marks write and edit", () => {
  assert.equal(isFileChangeTool("write"), true);
  assert.equal(isFileChangeTool("plan_write"), false);
});

test("unifiedPatch keeps a shared prefix and suffix", () => {
  const patch = unifiedPatch("n.py", "keep\nold\ntail", "keep\nnew\ntail");
  assert.match(patch, /^--- a\/n\.py$/m);
  assert.match(patch, / keep/);
  assert.match(patch, /-old/);
  assert.match(patch, /\+new/);
  assert.match(patch, / tail/);
});

test("diffViewHunks drops the tool separator line", () => {
  assert.deepEqual(diffViewHunks("=================\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n"), [
    "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b",
  ]);
  assert.deepEqual(diffViewHunks("   "), []);
});

test("collectRunFileGroups keeps one box per run and the last write of a path", () => {
  const groups = collectRunFileGroups([
    { kind: "user", runId: "r1" },
    {
      kind: "tool",
      name: "write",
      runId: "r1",
      arguments: { path: "a.py", content: "one" },
    },
    {
      kind: "tool",
      name: "write",
      runId: "r1",
      arguments: { path: "a.py", content: "two" },
    },
    {
      kind: "tool",
      name: "edit",
      runId: "r1",
      arguments: { path: "b.ts", edits: [{ oldText: "x", newText: "y" }] },
    },
    {
      kind: "tool",
      name: "write",
      runId: "r2",
      arguments: { path: "a.py", content: "later" },
    },
  ]);
  assert.equal(groups.length, 2);
  assert.equal(groups[0]?.runId, "r1");
  assert.deepEqual(
    groups[0]?.files.map((file) => file.path),
    ["a.py", "b.ts"],
  );
  assert.equal(groups[0]?.files[0]?.content, "two");
  assert.equal(groups[1]?.runId, "r2");
  assert.equal(groups[1]?.files[0]?.content, "later");
  assert.equal(fileKey(groups[1]!.files[0]!), "r2:a.py");
});

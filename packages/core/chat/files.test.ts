import assert from "node:assert/strict";
import { test } from "node:test";

import { compactToolDump, fileChangeFromTool, isFileChangeTool } from "./files.ts";

test("write to a regular file is a file change", () => {
  assert.deepEqual(
    fileChangeFromTool({
      name: "write",
      arguments: { path: "store.py", content: "print(1)" },
    }),
    { path: "store.py", content: "print(1)", action: "write" },
  );
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

test("edit collects replacement text", () => {
  const got = fileChangeFromTool({
    name: "edit",
    arguments: {
      path: "server.py",
      edits: [{ oldText: "a", newText: "b" }],
    },
  });
  assert.equal(got?.path, "server.py");
  assert.equal(got?.action, "edit");
  assert.match(got?.content ?? "", /\+ b/);
});

test("compactToolDump strips file bodies", () => {
  assert.deepEqual(
    compactToolDump({
      name: "write",
      arguments: { path: "a.py", content: "huge" },
    }),
    { input: { path: "a.py" }, output: undefined },
  );
});

test("isFileChangeTool only marks write and edit", () => {
  assert.equal(isFileChangeTool("write"), true);
  assert.equal(isFileChangeTool("plan_write"), false);
});

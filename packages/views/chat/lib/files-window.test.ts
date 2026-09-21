import assert from "node:assert/strict";
import { test } from "node:test";

import { FILES_WINDOW_ID, nextFilesWindows, runFileTitle, type FileWindow } from "./files-window.ts";

const files = [
  { path: "a.py", content: "a", action: "write" as const, patch: "+a", runId: "r1" },
  { path: "b.ts", content: "b", action: "edit" as const, patch: "+b", runId: "r1" },
];

test("nextFilesWindows opens a files pane on first create", () => {
  const next = nextFilesWindows([], files, true);
  assert.equal(next.length, 1);
  assert.equal(next[0]?.kind, "file");
  assert.equal(next[0]?.selectedKey, "");
  assert.equal(next[0]?.id, FILES_WINDOW_ID);
});

test("nextFilesWindows does not reopen after the user closed it", () => {
  assert.deepEqual(nextFilesWindows([], files, false), []);
});

test("nextFilesWindows keeps a live selection and clears it when it disappears", () => {
  const current: FileWindow[] = [{ id: FILES_WINDOW_ID, kind: "file", title: "文件", selectedKey: "gone" }];
  const next = nextFilesWindows(current, files, false);
  assert.equal(next[0]?.selectedKey, "");
  const kept = nextFilesWindows(
    [{ id: FILES_WINDOW_ID, kind: "file", title: "文件", selectedKey: "r1:b.ts" }],
    files,
    false,
  );
  assert.equal(kept[0]?.selectedKey, "r1:b.ts");
});

test("runFileTitle uses the user line of that run", () => {
  assert.equal(
    runFileTitle(
      [
        { kind: "user", id: "u", runId: "r1", messageId: "m", text: "加上借还书\n第二行", seq: 1 },
        { kind: "user", id: "u2", runId: "r2", messageId: "m2", text: "别的", seq: 2 },
      ],
      "r1",
    ),
    "加上借还书",
  );
  assert.equal(runFileTitle([], "r9"), "改动");
});

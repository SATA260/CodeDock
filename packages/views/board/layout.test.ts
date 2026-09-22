import assert from "node:assert/strict";
import { test } from "node:test";

import { boardColumns, columnsPerRow, pageColumns, pageCount } from "./layout.ts";

test("columnsPerRow caps at three and never drops below one", () => {
  assert.equal(columnsPerRow(0), 1);
  assert.equal(columnsPerRow(239), 1);
  assert.equal(columnsPerRow(480), 2);
  assert.equal(columnsPerRow(800), 3);
  assert.equal(columnsPerRow(1600), 3);
});

test("boardColumns appends the ungrouped lane and paginates by row", () => {
  const cols = boardColumns({
    cards: [
      {
        work: {
          id: "w1",
          tenant_id: "t",
          user_id: "u",
          title: "A",
          created_at: "",
          updated_at: "",
        },
        info: { work_id: "w1", checkout: "", body: "", updated_at: "" },
        dirs: [],
        sessions: [],
        running: 0,
        pending: 0,
      },
      {
        work: {
          id: "w2",
          tenant_id: "t",
          user_id: "u",
          title: "B",
          created_at: "",
          updated_at: "",
        },
        info: { work_id: "w2", checkout: "", body: "", updated_at: "" },
        dirs: [],
        sessions: [],
        running: 0,
        pending: 0,
      },
    ],
    ungrouped: [{ engine: "agent", session_id: "s1", summary: "闲", checkout: "", running: false, pending: 0, updated_at: "" }],
  });
  assert.equal(cols.length, 3);
  assert.equal(cols[2]?.ungrouped, true);
  assert.equal(pageCount(cols.length, 2), 2);
  assert.deepEqual(
    pageColumns(cols, 1, 2).map((col) => col.id),
    ["ungrouped"],
  );
});

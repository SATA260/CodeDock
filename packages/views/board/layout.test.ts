import assert from "node:assert/strict";
import { test } from "node:test";

import { boardColumns, revealColumnCount, visibleColumnCount } from "./layout.ts";

test("visibleColumnCount fits fixed-width columns and never drops below one", () => {
  assert.equal(visibleColumnCount(0), 1);
  assert.equal(visibleColumnCount(319), 1);
  assert.equal(visibleColumnCount(320), 1);
  assert.equal(visibleColumnCount(648), 2);
  assert.equal(visibleColumnCount(1020), 3);
});

test("revealColumnCount appends a batch and stops at the end", () => {
  assert.equal(revealColumnCount(2, 5, 2), 4);
  assert.equal(revealColumnCount(4, 5, 2), 5);
  assert.equal(revealColumnCount(5, 5, 2), 5);
  assert.equal(revealColumnCount(0, 3, 0), 1);
});

test("boardColumns appends the ungrouped lane", () => {
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
    ],
    ungrouped: [{ engine: "agent", session_id: "s1", summary: "闲", checkout: "", running: false, pending: 0, updated_at: "" }],
  });
  assert.equal(cols.length, 2);
  assert.equal(cols[1]?.ungrouped, true);
  assert.equal(cols[1]?.sessions[0]?.session_id, "s1");
});

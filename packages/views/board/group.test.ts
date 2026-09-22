import assert from "node:assert/strict";
import { test } from "node:test";

import { groupSessionsByWork, sessionKey } from "./group.ts";

test("groupSessionsByWork groups by work then time, leftover goes ungrouped", () => {
  const groups = groupSessionsByWork(
    [
      { id: "s-old", engine: "agent", updated_at: "2026-01-01T00:00:00Z", summary: "旧" },
      { id: "s-new", engine: "agent", updated_at: "2026-02-01T00:00:00Z", summary: "新" },
      { id: "s-free", engine: "claude", updated_at: "2026-03-01T00:00:00Z", summary: "闲" },
    ],
    [
      { engine: "agent", session_id: "s-old", work_id: "w1", checkout: "" },
      { engine: "native", session_id: "s-new", work_id: "w1", checkout: "/repo" },
    ],
    [
      {
        id: "w1",
        tenant_id: "t",
        user_id: "u",
        title: "修登录",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-02-01T00:00:00Z",
      },
    ],
  );
  assert.equal(groups.length, 2);
  assert.equal(groups[0]?.id, "w1");
  assert.deepEqual(
    groups[0]?.sessions.map((item) => item.id),
    ["s-new", "s-old"],
  );
  assert.equal(groups[1]?.id, null);
  assert.equal(groups[1]?.title, "未分组");
  assert.equal(groups[1]?.sessions[0]?.id, "s-free");
  assert.equal(sessionKey("native", "s-new"), "agent:s-new");
});

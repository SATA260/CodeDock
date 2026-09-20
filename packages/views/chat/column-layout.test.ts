import assert from "node:assert/strict";
import { test } from "node:test";

import { fitColumns, LEFT_MIN, MID_MIN, RIGHT_MIN } from "./column-layout.ts";

test("fitColumns keeps both sides when there is room", () => {
  const got = fitColumns(240, 420, 1200);
  assert.equal(got.left, 240);
  assert.equal(got.right, 420);
});

test("fitColumns ignores a collapsed side", () => {
  const leftClosed = fitColumns(240, 420, 800, { left: false, right: true });
  assert.equal(leftClosed.left, 0);
  assert.ok(leftClosed.right >= RIGHT_MIN);

  const bothClosed = fitColumns(240, 420, 400, { left: false, right: false });
  assert.deepEqual(bothClosed, { left: 0, right: 0 });
});

test("fitColumns still leaves the mid pane when only the left is open", () => {
  const got = fitColumns(480, 860, LEFT_MIN + MID_MIN + 20, { left: true, right: false });
  assert.equal(got.right, 0);
  assert.ok(got.left >= LEFT_MIN);
  assert.ok(got.left + MID_MIN <= LEFT_MIN + MID_MIN + 20);
});

import assert from "node:assert/strict";
import { test } from "node:test";

import { DEFAULT_APPROVAL_MODE, DEFAULT_WORK_MODE, parseApprovalMode, parseWorkMode } from "./composer.ts";

test("parseWorkMode keeps a known mode", () => {
  assert.equal(parseWorkMode("plan"), "plan");
  assert.equal(parseWorkMode(" ask "), "ask");
});

test("parseWorkMode falls back on junk", () => {
  assert.equal(parseWorkMode(""), DEFAULT_WORK_MODE);
  assert.equal(parseWorkMode("yolo"), DEFAULT_WORK_MODE);
  assert.equal(parseWorkMode("nope", "ask"), "ask");
});

test("parseApprovalMode keeps a known mode", () => {
  assert.equal(parseApprovalMode("yolo"), "yolo");
  assert.equal(parseApprovalMode(" auto "), "auto");
});

test("parseApprovalMode falls back on junk", () => {
  assert.equal(parseApprovalMode(""), DEFAULT_APPROVAL_MODE);
  assert.equal(parseApprovalMode("manaul"), DEFAULT_APPROVAL_MODE);
  assert.equal(parseApprovalMode("agent", "yolo"), "yolo");
});

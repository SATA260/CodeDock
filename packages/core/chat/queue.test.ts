import assert from "node:assert/strict";
import { test } from "node:test";

import { joinQueuedTexts } from "./queue.ts";

test("joinQueuedTexts trims and joins with single newlines", () => {
  assert.equal(joinQueuedTexts([" a ", "", "b\n", "  "]), "a\nb");
});

test("joinQueuedTexts returns empty when nothing remains", () => {
  assert.equal(joinQueuedTexts([]), "");
  assert.equal(joinQueuedTexts(["  ", "\n"]), "");
});

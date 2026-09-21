import assert from "node:assert/strict";
import { test } from "node:test";

import { terminalStatusCopy } from "./terminal.ts";

test("terminalStatusCopy spells completed and model errors", () => {
  assert.equal(terminalStatusCopy({ status: "completed" }), "Run completed");
  assert.equal(terminalStatusCopy({ status: "cancelled" }), "Cancelled");
  assert.equal(terminalStatusCopy({ status: "failed", stopReason: "model_error" }), "Model request failed");
  assert.equal(
    terminalStatusCopy({
      status: "failed",
      stopReason: "model_error",
      error: "Service is too busy.",
    }),
    "Model request failed: Service is too busy.",
  );
});

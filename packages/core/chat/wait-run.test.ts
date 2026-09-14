import assert from "node:assert/strict";
import { test } from "node:test";

import { waitUntilRunReleased } from "./wait-run.ts";

test("waitUntilRunReleased returns when the session drops the run", async () => {
  let active: string | undefined = "r1";
  let now = 0;
  await waitUntilRunReleased({
    runId: "r1",
    getActiveRunId: async () => {
      const current = active;
      active = undefined;
      return current;
    },
    now: () => now,
    sleep: async () => {
      now += 50;
    },
  });
});

test("waitUntilRunReleased returns immediately if another run is active", async () => {
  await waitUntilRunReleased({
    runId: "r1",
    getActiveRunId: async () => "r2",
    now: () => 0,
    sleep: async () => {
      throw new Error("should not sleep");
    },
  });
});

test("waitUntilRunReleased times out while the same run stays active", async () => {
  await assert.rejects(
    () =>
      waitUntilRunReleased({
        runId: "r1",
        getActiveRunId: async () => "r1",
        timeoutMs: 20,
        intervalMs: 10,
        now: (() => {
          let t = 0;
          return () => {
            const current = t;
            t += 10;
            return current;
          };
        })(),
        sleep: async () => {},
      }),
    /尚未结束/,
  );
});

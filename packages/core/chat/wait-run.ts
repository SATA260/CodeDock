/** 取消后等到会话不再占用该 Run，再允许开下一轮。 */
export async function waitUntilRunReleased(input: {
  runId: string;
  getActiveRunId: () => Promise<string | undefined>;
  timeoutMs?: number;
  intervalMs?: number;
  now?: () => number;
  sleep?: (ms: number) => Promise<void>;
}): Promise<void> {
  const timeoutMs = input.timeoutMs ?? 8000;
  const intervalMs = input.intervalMs ?? 50;
  const now = input.now ?? Date.now;
  const sleep = input.sleep ?? ((ms: number) => new Promise((resolve) => setTimeout(resolve, ms)));
  const deadline = now() + timeoutMs;
  for (;;) {
    const active = await input.getActiveRunId();
    if (!active || active !== input.runId) {
      return;
    }
    if (now() >= deadline) {
      throw new Error("上一轮任务尚未结束");
    }
    await sleep(intervalMs);
  }
}

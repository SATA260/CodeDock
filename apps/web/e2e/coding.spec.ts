import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  createGitWorkspace,
  createSession,
  loadRepoEnv,
  openSession,
  promptAsk,
  promptC01,
  promptCancel,
  promptPlan,
  selectModes,
  sendPrompt,
  waitTerminal,
  writeWorkspaceFile,
} from "./helpers.ts";

test.beforeAll(() => {
  loadRepoEnv();
  const provider = (process.env.LLM_PROVIDER ?? "").trim();
  const key = (process.env.LLM_API_KEY ?? "").trim();
  test.skip(!key || !provider || provider === "fake", "coding e2e requires a live LLM in .env");
});

test.describe.configure({ mode: "serial" });

test("C01_write_read", async ({ page }) => {
  const ws = createGitWorkspace();
  const sessionId = await createSession(ws);
  await openSession(page, sessionId);
  await selectModes(page, "agent", "yolo");
  await sendPrompt(page, promptC01);
  await waitTerminal(page, "Run completed");
  const body = readFileSync(path.join(ws, "hello.txt"), "utf8").trim();
  expect(body).toBe("hi");
});

test("U04_ask_plan_no_model_error", async ({ page }) => {
  const ws = createGitWorkspace();
  writeWorkspaceFile(ws, "hello.txt", "hi\n");
  const sessionId = await createSession(ws);
  await openSession(page, sessionId);
  await selectModes(page, "ask", "yolo");
  await sendPrompt(page, promptAsk);
  await waitTerminal(page, "Run completed");
  await expect(page.getByTestId("run-terminal")).not.toContainText("Model request failed");
  await selectModes(page, "plan", "yolo");
  await sendPrompt(page, promptPlan);
  await waitTerminal(page, /Run completed/);
  await expect(page.getByTestId("run-terminal").last()).not.toContainText("Model request failed");
});

test("L01_U05_cancel_button", async ({ page }) => {
  const ws = createGitWorkspace();
  const sessionId = await createSession(ws);
  await openSession(page, sessionId);
  await selectModes(page, "agent", "yolo");
  await sendPrompt(page, promptCancel);
  await expect(page.getByTestId("composer-cancel")).toBeVisible({ timeout: 60_000 });
  await page.getByTestId("composer-cancel").click();
  await waitTerminal(page, "Cancelled");
  await expect(page.getByTestId("composer-input")).toBeEnabled();
});

test("R04_page_refresh", async ({ page }) => {
  const ws = createGitWorkspace();
  const sessionId = await createSession(ws);
  await openSession(page, sessionId);
  await selectModes(page, "agent", "yolo");
  await sendPrompt(page, promptCancel);
  await expect(page.getByTestId("live-status").or(page.getByTestId("composer-cancel"))).toBeVisible({
    timeout: 60_000,
  });
  await page.reload();
  await expect(page.getByTestId("composer")).toHaveAttribute("data-hydrated", "true");
  await expect(page.getByTestId("timeline")).toBeVisible();
  const cancel = page.getByTestId("composer-cancel");
  const terminal = page.getByTestId("run-terminal");
  await expect(cancel.or(terminal)).toBeVisible({ timeout: 60_000 });
});

import { expect, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

const root = path.resolve(__dirname, "../../..");
const apiBase = (process.env.E2E_API_BASE ?? "http://127.0.0.1:18080").replace(/\/$/, "");

export const promptC01 = "在工作区创建 hello.txt，内容仅一行 hi。必须用工具写盘，不要只口头描述。";
export const promptCancel = "依次创建 f1.txt 到 f8.txt，每个文件写一段不少于 80 字的说明。不要省略。";
export const promptAsk = "只回答：工作区有几个 .txt 文件？不要写文件。";
export const promptPlan = "只写一份带验收项的空计划，字段必须是 id、description、verify_cmd。";

// loadRepoEnv 读仓库根 .env，不覆盖已有环境变量，也不把 Key 打进日志。
export function loadRepoEnv(): Record<string, string> {
  const pairs: Record<string, string> = {};
  let body = "";
  try {
    body = readFileSync(path.join(root, ".env"), "utf8");
  } catch {
    return pairs;
  }
  for (const line of body.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#") || !trimmed.includes("=")) {
      continue;
    }
    const raw = trimmed.startsWith("export ") ? trimmed.slice(7) : trimmed;
    const eq = raw.indexOf("=");
    const key = raw.slice(0, eq).trim();
    let value = raw.slice(eq + 1).trim();
    if ((value.startsWith("\"") && value.endsWith("\"")) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    pairs[key] = value;
    if (!process.env[key]) {
      process.env[key] = value;
    }
  }
  return pairs;
}

// requireLiveEnv 没有真实模型时跳过 E2E。
export function requireLiveEnv(): void {
  loadRepoEnv();
  const provider = (process.env.LLM_PROVIDER ?? "").trim();
  const key = (process.env.LLM_API_KEY ?? "").trim();
  if (!key || !provider || provider === "fake") {
    throw new Error("skip: coding e2e requires a live LLM in .env");
  }
}

// createGitWorkspace 建一个带空提交的临时 Git 目录。
export function createGitWorkspace(): string {
  const dir = mkdtempSync(path.join(tmpdir(), "codedock-e2e-"));
  execFileSync("git", ["init", "-b", "main"], { cwd: dir });
  execFileSync("git", ["config", "user.email", "tester@example.com"], { cwd: dir });
  execFileSync("git", ["config", "user.name", "tester"], { cwd: dir });
  execFileSync("git", ["commit", "--allow-empty", "-m", "init"], { cwd: dir });
  mkdirSync(dir, { recursive: true });
  return dir;
}

// writeWorkspaceFile 预置工作区文件。
export function writeWorkspaceFile(dir: string, name: string, body: string): void {
  writeFileSync(path.join(dir, name), body);
}

// createSession 经 HTTP 冻结工作区并建会话。
export async function createSession(workspace: string): Promise<string> {
  const res = await fetch(`${apiBase}/sessions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user_id: "local", tenant_id: "default", workspace_id: workspace }),
  });
  if (!res.ok) {
    throw new Error(`create session ${res.status} ${await res.text()}`);
  }
  const body = (await res.json()) as { session: { id: string } };
  return body.session.id;
}

// openSession 打开本地会话页并等输入框。
export async function openSession(page: Page, sessionId: string): Promise<void> {
  await page.goto(`/s/${sessionId}`);
  await expect(page.getByTestId("composer")).toHaveAttribute("data-hydrated", "true");
  await expect(page.getByTestId("composer-input")).toBeVisible();
}

// selectModes 切到指定工作模式和审批模式。
export async function selectModes(page: Page, work: string, approval: string): Promise<void> {
  await page.getByTestId("work-mode").click();
  await page.getByTestId(`work-mode-${work}`).click();
  await page.getByTestId("approval-mode").click();
  await page.getByTestId(`approval-mode-${approval}`).click();
}

// sendPrompt 在输入框发一条消息。
export async function sendPrompt(page: Page, text: string): Promise<void> {
  await page.getByTestId("composer-input").fill(text);
  await page.getByTestId("composer-send").click();
}

// waitTerminal 等到新的时间线终态；复审挂起时点「就这样结束」。
export async function waitTerminal(page: Page, match: RegExp | string, timeout = 180_000): Promise<void> {
  const before = await page.getByTestId("run-terminal").count();
  const accept = page.getByTestId("run-accept");
  await expect
    .poll(
      async () => {
        if (await accept.isVisible()) {
          await accept.click();
        }
        return page.getByTestId("run-terminal").count();
      },
      { timeout },
    )
    .toBeGreaterThan(before);
  await expect(page.getByTestId("run-terminal").nth(before)).toContainText(match, { timeout: 60_000 });
}

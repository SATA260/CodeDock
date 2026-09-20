import type { ApprovalMode, WorkMode } from "@codedock/core/chat";

const WORK_MODE_KEY = "codedock.workMode";
const APPROVAL_MODE_KEY = "codedock.approvalMode";
const WORK_MODES: readonly WorkMode[] = ["ask", "plan", "agent"];
const APPROVAL_MODES: readonly ApprovalMode[] = ["manual", "auto", "yolo"];

export const DEFAULT_WORK_MODE: WorkMode = "agent";
export const DEFAULT_APPROVAL_MODE: ApprovalMode = "manual";

// parseWorkMode 只收 ask / plan / agent，其它回默认。
export function parseWorkMode(raw: string | null | undefined, fallback: WorkMode = DEFAULT_WORK_MODE): WorkMode {
  const value = raw?.trim();
  return value && WORK_MODES.includes(value as WorkMode) ? (value as WorkMode) : fallback;
}

// parseApprovalMode 只收 manual / auto / yolo，其它回默认。
export function parseApprovalMode(
  raw: string | null | undefined,
  fallback: ApprovalMode = DEFAULT_APPROVAL_MODE,
): ApprovalMode {
  const value = raw?.trim();
  return value && APPROVAL_MODES.includes(value as ApprovalMode) ? (value as ApprovalMode) : fallback;
}

// readLastWorkMode 读本机上次选的工作模式。
export function readLastWorkMode(): WorkMode {
  return parseWorkMode(readStored(WORK_MODE_KEY), DEFAULT_WORK_MODE);
}

// writeLastWorkMode 把工作模式记到本机。
export function writeLastWorkMode(mode: WorkMode): void {
  writeStored(WORK_MODE_KEY, parseWorkMode(mode));
}

// readLastApprovalMode 读本机上次选的审批模式。
export function readLastApprovalMode(): ApprovalMode {
  return parseApprovalMode(readStored(APPROVAL_MODE_KEY), DEFAULT_APPROVAL_MODE);
}

// writeLastApprovalMode 把审批模式记到本机。
export function writeLastApprovalMode(mode: ApprovalMode): void {
  writeStored(APPROVAL_MODE_KEY, parseApprovalMode(mode));
}

// readStored 读本机字符串；读不到当没有。
function readStored(key: string): string | null {
  if (typeof window === "undefined") {
    return null;
  }
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

// writeStored 把字符串记到本机。
function writeStored(key: string, value: string): void {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // ignore quota / private mode
  }
}

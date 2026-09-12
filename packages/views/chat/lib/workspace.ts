const LAST_WORKSPACE_KEY = "codedock.lastWorkspace";

export function readLastWorkspace(): string {
  if (typeof window === "undefined") {
    return "";
  }
  try {
    return window.localStorage.getItem(LAST_WORKSPACE_KEY) ?? "";
  } catch {
    return "";
  }
}

export function writeLastWorkspace(path: string): void {
  if (typeof window === "undefined") {
    return;
  }
  const trimmed = path.trim();
  if (!trimmed) {
    return;
  }
  try {
    window.localStorage.setItem(LAST_WORKSPACE_KEY, trimmed);
  } catch {
    // ignore quota / private mode
  }
}

export function clearLastWorkspace(): void {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.removeItem(LAST_WORKSPACE_KEY);
  } catch {
    // ignore
  }
}

export function createSessionError(err: unknown, fallback = "无法创建会话"): string {
  const raw = err instanceof Error ? err.message : fallback;
  if (raw.includes("workspace directory not found")) {
    return "工作目录不存在，请重新选择或使用默认仓库";
  }
  if (raw.includes("workspace is not a directory")) {
    return "工作目录必须是文件夹";
  }
  return raw;
}

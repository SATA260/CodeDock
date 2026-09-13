const CURRENT_SESSION_KEY = "codedock.currentSession";

export function rememberSession(sessionId: string): void {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(CURRENT_SESSION_KEY, sessionId);
  } catch {
    // ignore
  }
}

export function readCurrentSession(): string | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }
  try {
    return window.localStorage.getItem(CURRENT_SESSION_KEY) || undefined;
  } catch {
    return undefined;
  }
}

import type { TokenUsage } from "@codedock/core/codex";

export function remainingContext(usage?: TokenUsage): { percent: number; left: number; window: number } | null {
  if (!usage || usage.window <= 0) {
    return null;
  }
  const left = Math.max(0, usage.window - usage.used);
  return { percent: Math.round((left * 100) / usage.window), left, window: usage.window };
}

export function formatTokens(value: number): string {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
  }
  if (value >= 10_000) {
    return `${Math.round(value / 1000)}k`;
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1).replace(/\.0$/, "")}k`;
  }
  return String(value);
}

export function shortId(id: string): string {
  return id.replace(/-/g, "").slice(0, 8);
}

// sessionTitle 对话标题：正文中间可省略，末尾 (n) 必须留下。
export function sessionTitle(session: { id: string; title?: string; preview?: string }): string {
  const text = session.title?.trim() || session.preview?.trim();
  if (!text) {
    return `对话 ${shortId(session.id)}`;
  }
  const match = text.match(/ \((\d+)\)$/);
  const suffix = match && match.index != null ? text.slice(match.index) : "";
  const stem = match && match.index != null ? text.slice(0, match.index) : text;
  if ([...text].length <= 36) {
    return text;
  }
  return `${clipMiddle(stem, Math.max(8, 36 - [...suffix].length))}${suffix}`;
}

// clipMiddle 超长正文留头尾，中间用省略号。
function clipMiddle(text: string, max: number): string {
  const runes = [...text];
  if (runes.length <= max) {
    return text.trim();
  }
  const keep = Math.max(1, max - 1);
  const head = Math.max(1, Math.ceil(keep * 0.55));
  const tail = Math.max(0, keep - head);
  const start = runes.slice(0, head).join("").trimEnd();
  const end = tail > 0 ? runes.slice(-tail).join("").trimStart() : "";
  return end ? `${start}…${end}` : `${start}…`;
}

export function formatStamp(value?: number): string {
  if (!value) {
    return "";
  }
  const ms = value > 1e11 ? value : value * 1000;
  const then = new Date(ms).getTime();
  if (Number.isNaN(then)) {
    return "";
  }
  const minutes = Math.floor((Date.now() - then) / 60_000);
  if (minutes < 1) {
    return "刚刚";
  }
  if (minutes < 60) {
    return `${minutes} 分钟前`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours} 小时前`;
  }
  return new Date(ms).toLocaleDateString();
}

export function shortWorkspace(path: string, max = 42): string {
  const trimmed = path.trim();
  if (!trimmed || trimmed.length <= max) {
    return trimmed;
  }
  const parts = trimmed.split(/[/\\]/).filter(Boolean);
  if (parts.length >= 2) {
    return `…/${parts.slice(-2).join("/")}`;
  }
  return `…${trimmed.slice(-(max - 1))}`;
}

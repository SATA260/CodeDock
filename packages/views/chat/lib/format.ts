import { firstLine } from "@codedock/core/chat";

export function shortId(id: string): string {
  return id.replace(/-/g, "").slice(0, 8);
}

export function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) {
    return "";
  }
  const delta = Date.now() - then;
  const minutes = Math.floor(delta / 60_000);
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
  const days = Math.floor(hours / 24);
  if (days < 7) {
    return `${days} 天前`;
  }
  return new Date(iso).toLocaleDateString();
}

export function sessionTitle(id: string, preview?: string): string {
  if (preview?.trim()) {
    return firstLine(preview, 36);
  }
  return `会话 ${shortId(id)}`;
}

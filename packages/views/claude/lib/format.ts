import type { ClaudeTokenUsage } from "@codedock/core/claude";

// remainingContext 把官方 used / window 收成剩余百分比，窗口未知则不显示。
export function remainingContext(usage?: ClaudeTokenUsage): { percent: number; left: number; window: number } | null {
  if (!usage || usage.window <= 0) {
    return null;
  }
  const left = Math.max(0, usage.window - usage.used);
  return { percent: Math.round((left * 100) / usage.window), left, window: usage.window };
}

// formatTokens 给压缩按钮提示用的短用量。
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

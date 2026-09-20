"use client";

import { cn } from "../lib/cn.ts";

/** 仅当前进行中的状态带扫光；历史状态保持静态文字。 */
export function LiveStatus({
  children,
  active = false,
  className,
}: {
  children: string;
  active?: boolean;
  className?: string;
}) {
  return (
    <span
      className={cn(active && "live-status-active", className)}
      aria-live={active ? "polite" : undefined}
    >
      {children}
    </span>
  );
}

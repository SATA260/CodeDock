"use client";

import type { HTMLAttributes } from "react";

import { cn } from "../lib/cn.ts";

/** 仅当前进行中的状态带扫光；历史状态保持静态文字。 */
export function LiveStatus({
  children,
  active = false,
  className,
  ...props
}: {
  children: string;
  active?: boolean;
} & HTMLAttributes<HTMLSpanElement>) {
  return (
    <span
      className={cn(active && "live-status-active", className)}
      aria-live={active ? "polite" : undefined}
      {...props}
    >
      {children}
    </span>
  );
}

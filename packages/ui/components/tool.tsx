"use client";

import { ChevronDownIcon, WrenchIcon } from "lucide-react";
import { useState, type ReactNode } from "react";

import { cn } from "../lib/cn.ts";
import { formatJSON } from "../lib/json.ts";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "./ui/collapsible.tsx";

export type ToolState = "pending" | "running" | "completed" | "error" | "denied";

const stateLabel: Record<ToolState, string> = {
  pending: "待执行",
  running: "执行中",
  completed: "已完成",
  error: "失败",
  denied: "已拒绝",
};

export function Tool({
  className,
  defaultOpen = false,
  children,
}: {
  className?: string;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className={cn("w-full rounded-lg border border-border bg-accent/60", className)}
    >
      {children}
    </Collapsible>
  );
}

export function ToolHeader({
  type,
  state,
  className,
}: {
  type: string;
  state: ToolState;
  className?: string;
}) {
  const name = type.startsWith("tool-") ? type.slice(5) : type;
  return (
    <CollapsibleTrigger
      className={cn(
        "flex items-center gap-2 px-3 py-2 text-xs text-muted-foreground hover:text-accent-foreground",
        className,
      )}
    >
      <WrenchIcon className="size-3.5" />
      <span className="font-mono text-accent-foreground">{name}</span>
      <span className="text-muted-foreground">{stateLabel[state]}</span>
      <ChevronDownIcon className="ml-auto size-3.5" />
    </CollapsibleTrigger>
  );
}

export function ToolContent({ children, className }: { children: ReactNode; className?: string }) {
  return <CollapsibleContent className={cn("space-y-2 px-3 pb-3", className)}>{children}</CollapsibleContent>;
}

export function ToolInput({ input }: { input: unknown }) {
  if (input == null) {
    return null;
  }
  return (
    <pre className="overflow-x-auto rounded-md bg-background/50 p-2 font-mono text-xs leading-5 text-muted-foreground">
      {formatJSON(input)}
    </pre>
  );
}

export function ToolOutput({
  output,
  errorText,
}: {
  output?: unknown;
  errorText?: string;
}) {
  if (errorText) {
    return <p className="text-xs text-destructive">{errorText}</p>;
  }
  if (output == null) {
    return null;
  }
  return (
    <pre className="overflow-x-auto rounded-md bg-background/50 p-2 font-mono text-xs leading-5 text-foreground/80">
      {formatJSON(output)}
    </pre>
  );
}

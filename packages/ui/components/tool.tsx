"use client";

import { ChevronDownIcon, WrenchIcon } from "lucide-react";
import { createContext, useContext, useMemo, useState, type ReactNode } from "react";

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

type OpenContextValue = {
  isOpen: boolean;
};

const ToolGroupContext = createContext<OpenContextValue | null>(null);
const ToolContext = createContext<OpenContextValue | null>(null);

export function ToolGroup({
  className,
  defaultOpen = false,
  children,
}: {
  className?: string;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const value = useMemo(() => ({ isOpen: open }), [open]);
  return (
    <ToolGroupContext.Provider value={value}>
      <Collapsible
        open={open}
        onOpenChange={setOpen}
        className={cn("w-full text-sm leading-5 text-muted-foreground", className)}
      >
        {children}
      </Collapsible>
    </ToolGroupContext.Provider>
  );
}

export function ToolGroupHeader({
  count,
  state,
  className,
}: {
  count: number;
  state: ToolState;
  className?: string;
}) {
  const ctx = useContext(ToolGroupContext);
  return (
    <CollapsibleTrigger
      className={cn(
        "flex items-center gap-2 text-muted-foreground transition-colors hover:text-accent-foreground",
        className,
      )}
    >
      <WrenchIcon className="size-3.5" />
      <span>{count === 1 ? "调用了 1 个工具" : `调用了 ${count} 个工具`}</span>
      <span className="text-muted-foreground/70">{stateLabel[state]}</span>
      <ChevronDownIcon
        className={cn("size-3.5 transition-transform", ctx?.isOpen && "rotate-180")}
      />
    </CollapsibleTrigger>
  );
}

export function ToolGroupContent({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <CollapsibleContent className={cn("mt-1 space-y-0.5 pl-6", className)}>
      {children}
    </CollapsibleContent>
  );
}

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
  const value = useMemo(() => ({ isOpen: open }), [open]);
  return (
    <ToolContext.Provider value={value}>
      <Collapsible open={open} onOpenChange={setOpen} className={cn("w-full", className)}>
        {children}
      </Collapsible>
    </ToolContext.Provider>
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
  const ctx = useContext(ToolContext);
  return (
    <CollapsibleTrigger
      className={cn(
        "flex items-center gap-2 py-0.5 text-xs leading-4 text-muted-foreground transition-colors hover:text-accent-foreground",
        className,
      )}
    >
      <span className="font-mono text-accent-foreground">{name}</span>
      <span>{stateLabel[state]}</span>
      <ChevronDownIcon
        className={cn("size-3.5 transition-transform", ctx?.isOpen && "rotate-180")}
      />
    </CollapsibleTrigger>
  );
}

export function ToolContent({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <CollapsibleContent className={cn("space-y-1.5 pb-1.5 pt-1", className)}>{children}</CollapsibleContent>
  );
}

export function ToolInput({ input }: { input: unknown }) {
  if (input == null) {
    return <p className="text-[11px] leading-4 text-muted-foreground/70">无参数</p>;
  }
  return (
    <div className="space-y-1">
      <div className="text-[11px] leading-4 text-muted-foreground/70">参数</div>
      <pre className="overflow-x-auto rounded-md bg-muted/60 p-2 font-mono text-xs leading-5 text-muted-foreground">
        {formatJSON(input)}
      </pre>
    </div>
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
    return (
      <div className="space-y-1">
        <div className="text-[11px] leading-4 text-destructive">错误</div>
        <p className="text-xs leading-5 text-destructive">{errorText}</p>
      </div>
    );
  }
  if (output == null) {
    return null;
  }
  return (
    <div className="space-y-1">
      <div className="text-[11px] leading-4 text-muted-foreground/70">输出</div>
      <pre className="overflow-x-auto rounded-md bg-muted/60 p-2 font-mono text-xs leading-5 text-foreground/80">
        {formatJSON(output)}
      </pre>
    </div>
  );
}

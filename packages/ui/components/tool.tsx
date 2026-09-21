"use client";

import { ChevronDownIcon, WrenchIcon } from "lucide-react";
import { createContext, useContext, useMemo, useState, type ReactNode } from "react";

import { LiveStatus } from "./live-status.tsx";
import { cn } from "../lib/cn.ts";
import { formatJSON } from "../lib/json.ts";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "./ui/collapsible.tsx";

export type ToolState = "pending" | "running" | "completed" | "error" | "denied";

// isLiveToolState 判断工具是否仍在排队或执行，需要动态状态文案。
function isLiveToolState(state: ToolState): boolean {
  return state === "pending" || state === "running";
}

const stateLabel: Record<ToolState, string> = {
  pending: "Pending",
  running: "Running",
  completed: "Done",
  error: "Failed",
  denied: "Denied",
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
  live = false,
  className,
}: {
  count: number;
  state: ToolState;
  live?: boolean;
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
      <span>{count === 1 ? "Used 1 tool" : `Used ${count} tools`}</span>
      <span className="text-muted-foreground/70">
        <LiveStatus active={live && isLiveToolState(state)}>{stateLabel[state]}</LiveStatus>
      </span>
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
  live = false,
  className,
}: {
  type: string;
  state: ToolState;
  live?: boolean;
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
      <LiveStatus active={live && isLiveToolState(state)}>{stateLabel[state]}</LiveStatus>
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
    return <p className="text-[11px] leading-4 text-muted-foreground/70">No arguments</p>;
  }
  return (
    <div className="space-y-1">
      <div className="text-[11px] leading-4 text-muted-foreground/70">Arguments</div>
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
        <div className="text-[11px] leading-4 text-destructive">Error</div>
        <p className="text-xs leading-5 text-destructive">{errorText}</p>
      </div>
    );
  }
  if (output == null) {
    return null;
  }
  return (
    <div className="space-y-1">
      <div className="text-[11px] leading-4 text-muted-foreground/70">Output</div>
      <pre className="overflow-x-auto rounded-md bg-muted/60 p-2 font-mono text-xs leading-5 text-foreground/80">
        {formatJSON(output)}
      </pre>
    </div>
  );
}

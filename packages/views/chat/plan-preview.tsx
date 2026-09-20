"use client";

import type { PlanPreview, ToolItemState } from "@codedock/core/chat";
import {
  cn,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  LiveStatus,
  MessageResponse,
} from "@codedock/ui";
import { ChevronDownIcon, FileText } from "lucide-react";
import { useState } from "react";

const stateLabel: Record<ToolItemState, Partial<Record<"write" | "read" | "doc", string>>> = {
  pending: { write: "Writing", read: "Reading", doc: "Working" },
  running: { write: "Writing", read: "Reading", doc: "Working" },
  completed: { write: "Wrote", read: "Read", doc: "Done" },
  error: { write: "Write failed", read: "Read failed", doc: "Failed" },
  denied: { write: "Denied", read: "Denied", doc: "Denied" },
};

export function PlanPreviewCard({
  preview,
  state = "completed",
  live = false,
  error,
  className,
}: {
  preview: PlanPreview;
  state?: ToolItemState;
  live?: boolean;
  error?: string;
  className?: string;
}) {
  const [open, setOpen] = useState(true);
  const status = stateLabel[state][preview.source] ?? stateLabel[state].doc ?? state;

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className={cn(
        "mx-3 overflow-hidden rounded-xl border border-border bg-muted",
        className,
      )}
    >
      <article
        data-plan-preview="doc"
        data-plan-name={preview.name}
        data-plan-state={state}
      >
        <CollapsibleTrigger className="flex items-center gap-2 px-5 py-2 text-xs leading-4 text-muted-foreground hover:text-accent-foreground">
          <FileText className="size-3.5 shrink-0" />
          <span>Plan</span>
          <span className="min-w-0 truncate font-mono text-accent-foreground">{preview.name}</span>
          <span className="ml-auto shrink-0 text-muted-foreground/70">
            <LiveStatus active={live && (state === "pending" || state === "running")}>{status}</LiveStatus>
          </span>
          <ChevronDownIcon
            className={cn("size-3.5 shrink-0 transition-transform", open && "rotate-180")}
          />
        </CollapsibleTrigger>
        <CollapsibleContent className="border-t border-border px-5 py-2.5">
          {error ? <p className="text-xs leading-5 text-destructive">{error}</p> : null}
          <PlanDocBody
            content={preview.content}
            emptyHint={emptyDocHint(preview.source, state)}
            animating={live && preview.source === "write" && (state === "pending" || state === "running")}
          />
        </CollapsibleContent>
      </article>
    </Collapsible>
  );
}

// PlanDocBody 渲染计划 Markdown，给对话卡片和右侧窗口共用。
export function PlanDocBody({
  content,
  emptyHint,
  animating,
}: {
  content: string;
  emptyHint: string;
  animating: boolean;
}) {
  if (!content.trim()) {
    return (
      <p className="text-xs leading-5 text-muted-foreground">
        <LiveStatus active={animating}>{emptyHint}</LiveStatus>
      </p>
    );
  }
  return (
    <MessageResponse isAnimating={animating} className="text-sm">
      {content}
    </MessageResponse>
  );
}

function emptyDocHint(source: "write" | "read", state: ToolItemState): string {
  if (source === "read" && (state === "pending" || state === "running")) {
    return "Reading";
  }
  if (source === "write" && (state === "pending" || state === "running")) {
    return "Writing";
  }
  return "Empty plan";
}

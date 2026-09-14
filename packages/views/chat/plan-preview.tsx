"use client";

import type { PlanPreview, ToolItemState } from "@codedock/core/chat";
import {
  cn,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  MessageResponse,
} from "@codedock/ui";
import { ChevronDownIcon, FileText } from "lucide-react";
import { useState } from "react";

const stateLabel: Record<ToolItemState, Partial<Record<"write" | "read" | "doc", string>>> = {
  pending: { write: "正在写入", read: "正在读取", doc: "处理中" },
  running: { write: "正在写入", read: "正在读取", doc: "处理中" },
  completed: { write: "已写入", read: "已读取", doc: "已完成" },
  error: { write: "写入失败", read: "读取失败", doc: "失败" },
  denied: { write: "已拒绝", read: "已拒绝", doc: "已拒绝" },
};

export function PlanPreviewCard({
  preview,
  state = "completed",
  error,
  className,
}: {
  preview: PlanPreview;
  state?: ToolItemState;
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
          <span>计划</span>
          <span className="min-w-0 truncate font-mono text-accent-foreground">{preview.name}</span>
          <span className="ml-auto shrink-0 text-muted-foreground/70">{status}</span>
          <ChevronDownIcon
            className={cn("size-3.5 shrink-0 transition-transform", open && "rotate-180")}
          />
        </CollapsibleTrigger>
        <CollapsibleContent className="border-t border-border px-5 py-2.5">
          {error ? <p className="text-xs leading-5 text-destructive">{error}</p> : null}
          <PlanDocBody
            content={preview.content}
            emptyHint={emptyDocHint(preview.source, state)}
            animating={preview.source === "write" && (state === "pending" || state === "running")}
          />
        </CollapsibleContent>
      </article>
    </Collapsible>
  );
}

function PlanDocBody({
  content,
  emptyHint,
  animating,
}: {
  content: string;
  emptyHint: string;
  animating: boolean;
}) {
  if (!content.trim()) {
    return <p className="text-xs leading-5 text-muted-foreground">{emptyHint}</p>;
  }
  return (
    <MessageResponse isAnimating={animating} className="text-sm">
      {content}
    </MessageResponse>
  );
}

function emptyDocHint(source: "write" | "read", state: ToolItemState): string {
  if (source === "read" && (state === "pending" || state === "running")) {
    return "正在读取…";
  }
  if (source === "write" && (state === "pending" || state === "running")) {
    return "正在写入…";
  }
  return "空计划";
}

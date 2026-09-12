"use client";

import {
  planPreviewFromTool,
  type ApprovalDecision,
  type ApprovalStatus,
  type ApprovalToolCall,
  type TimelineItem,
} from "@codedock/core/chat";
import { Button, cn, formatJSON, MessageResponse } from "@codedock/ui";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

type ApprovalItem = Extract<TimelineItem, { kind: "approval" }>;

type ApprovalPage = {
  approvalId: string;
  call: ApprovalToolCall;
};

type Choice = "approved" | "denied";

export function ApprovalDock({
  items,
  onDecide,
}: {
  items: ApprovalItem[];
  onDecide: (approvalId: string, decisions: ApprovalDecision[]) => Promise<void>;
}) {
  const pages = useMemo(() => pagesFrom(items), [items]);
  const [index, setIndex] = useState(0);
  const [choices, setChoices] = useState<Record<string, Choice>>({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    setIndex((current) => {
      if (pages.length === 0) {
        return 0;
      }
      return Math.min(current, pages.length - 1);
    });
  }, [pages]);

  useEffect(() => {
    if (pages.length === 0) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        setIndex((current) => Math.max(0, current - 1));
      }
      if (event.key === "ArrowRight") {
        event.preventDefault();
        setIndex((current) => Math.min(pages.length - 1, current + 1));
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [pages.length]);

  if (pages.length === 0) {
    return null;
  }

  const page = pages[Math.min(index, pages.length - 1)];
  if (!page) {
    return null;
  }
  const choiceKey = pageKey(page);
  const currentChoice = choiceOf(page, choices);
  const planPreview = planPreviewFromTool(page.call);

  const go = (next: number) => {
    if (next < 0 || next >= pages.length) {
      return;
    }
    setIndex(next);
  };

  const decideCurrent = async (status: Choice) => {
    const nextChoices = { ...choices, [choiceKey]: status };
    setChoices(nextChoices);
    const group = pages.filter((item) => item.approvalId === page.approvalId);
    const ready = group.every((item) => choiceOf(item, nextChoices));
    if (!ready) {
      const later = pages.findIndex(
        (item, cursor) => cursor > index && !choiceOf(item, nextChoices),
      );
      setIndex(later >= 0 ? later : Math.min(index + 1, pages.length - 1));
      return;
    }
    setSubmitting(true);
    try {
      await onDecide(
        page.approvalId,
        group.map((item) => ({
          tool_call_id: item.call.id,
          status: choiceOf(item, nextChoices) ?? status,
        })),
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex justify-center px-4 pb-1.5">
      <div
        className="w-full max-w-3xl rounded-xl border border-white/10 bg-zinc-950/40 px-2.5 py-1.5 shadow-[0_12px_40px_-16px_rgba(0,0,0,0.45)] backdrop-blur-md"
        role="dialog"
        aria-label="工具审批"
      >
        <div className="flex items-center justify-between gap-2">
          <div className="text-[10px] leading-3.5 font-medium text-muted-foreground">需要批准才能继续</div>
          <div className="flex items-center gap-0.5 text-[10px] leading-3.5 text-muted-foreground">
            <Button
              size="sm"
              variant="ghost"
              className="size-6 px-0"
              disabled={index <= 0}
              aria-label="上一条"
              onClick={() => go(index - 1)}
            >
              <ChevronLeft className="size-3.5" />
            </Button>
            <span className="min-w-8 text-center tabular-nums">
              {index + 1} / {pages.length}
            </span>
            <Button
              size="sm"
              variant="ghost"
              className="size-6 px-0"
              disabled={index >= pages.length - 1}
              aria-label="下一条"
              onClick={() => go(index + 1)}
            >
              <ChevronRight className="size-3.5" />
            </Button>
          </div>
        </div>
        {pages.length > 1 ? (
          <div className="mt-1 flex flex-wrap gap-1">
            {pages.map((item, cursor) => {
              const status = choiceOf(item, choices);
              return (
                <button
                  key={pageKey(item)}
                  type="button"
                  className={cn(
                    "rounded-full border px-1.5 py-px text-[10px] leading-3.5 transition-colors",
                    cursor === index
                      ? "border-white/25 bg-white/10 text-foreground"
                      : "border-white/10 bg-white/5 text-muted-foreground hover:text-foreground",
                  )}
                  onClick={() => go(cursor)}
                >
                  <span className="font-mono">{item.call.name || "工具"}</span>
                  <span className={cn("ml-1", statusTone(status))}>{statusLabel(status)}</span>
                </button>
              );
            })}
          </div>
        ) : null}
        <div className="mt-1 rounded-lg border border-white/10 bg-zinc-950/25 px-2 py-1">
          <div className="flex items-center justify-between gap-2">
            <div className="font-mono text-[11px] leading-4 text-foreground">{page.call.name || "工具调用"}</div>
            <div className={cn("text-[10px] leading-3.5", statusTone(currentChoice))}>{statusLabel(currentChoice)}</div>
          </div>
          {planPreview?.content ? (
            <div className="mt-1 max-h-48 overflow-auto">
              <MessageResponse className="text-[13px]">{planPreview.content}</MessageResponse>
            </div>
          ) : page.call.arguments != null ? (
            <pre className="mt-0.5 max-h-16 overflow-auto font-mono text-[11px] leading-4 text-muted-foreground">
              {formatJSON(page.call.arguments)}
            </pre>
          ) : null}
        </div>
        <div className="mt-1.5 flex justify-end gap-1.5">
          <Button
            size="sm"
            variant={currentChoice === "denied" ? "secondary" : "outline"}
            className="h-6 px-2 text-[11px]"
            disabled={submitting}
            onClick={() => void decideCurrent("denied")}
          >
            {currentChoice === "denied" ? "已拒绝" : "拒绝"}
          </Button>
          <Button
            size="sm"
            variant={currentChoice === "approved" ? "default" : "secondary"}
            className="h-6 px-2 text-[11px]"
            disabled={submitting}
            onClick={() => void decideCurrent("approved")}
          >
            {currentChoice === "approved" ? "已允许" : "允许"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function pagesFrom(items: ApprovalItem[]): ApprovalPage[] {
  const pages: ApprovalPage[] = [];
  for (const item of items) {
    const calls = item.toolCalls.filter((call) => call.id);
    if (calls.length === 0) {
      pages.push({
        approvalId: item.approvalId,
        call: item.toolCalls[0] ?? { id: "", name: "工具调用" },
      });
      continue;
    }
    for (const call of calls) {
      pages.push({ approvalId: item.approvalId, call });
    }
  }
  return pages;
}

function pageKey(page: ApprovalPage): string {
  return `${page.approvalId}:${page.call.id}`;
}

function choiceOf(page: ApprovalPage, choices: Record<string, Choice>): Choice | undefined {
  return choices[pageKey(page)] ?? asChoice(page.call.status);
}

function asChoice(status?: ApprovalStatus): Choice | undefined {
  if (status === "approved" || status === "denied") {
    return status;
  }
  return undefined;
}

function statusLabel(status?: Choice): string {
  if (status === "approved") {
    return "已允许";
  }
  if (status === "denied") {
    return "已拒绝";
  }
  return "待批";
}

function statusTone(status?: Choice): string {
  if (status === "approved") {
    return "text-emerald-300";
  }
  if (status === "denied") {
    return "text-red-300";
  }
  return "text-muted-foreground";
}

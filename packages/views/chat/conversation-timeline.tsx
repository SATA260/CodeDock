"use client";

import type { SessionState, ThinkingPhase, TimelineItem } from "@codedock/core/chat";
import {
  cn,
  Conversation,
  ConversationContent,
  ConversationEmptyState,
  Message,
  MessageContent,
  MessageResponse,
  Reasoning,
  ReasoningContent,
  ReasoningTrigger,
  Tool,
  ToolContent,
  ToolGroup,
  ToolGroupContent,
  ToolGroupHeader,
  ToolHeader,
  ToolInput,
  ToolOutput,
  type ToolState,
} from "@codedock/ui";
import { useEffect, useLayoutEffect, useRef, useState } from "react";

const thinkingCopy: Record<ThinkingPhase, string> = {
  queued: "排队中",
  loading_context: "正在装载上下文",
  running_llm: "正在思考",
};

export function ConversationTimeline({
  state,
  loading = false,
  scrollKey,
}: {
  state: SessionState;
  loading?: boolean;
  scrollKey?: string;
}) {
  const items = state.items.filter(
    (item) =>
      !(item.kind === "approval" && item.status === "pending") &&
      !(item.kind === "user" && !item.text.trim()) &&
      !(item.kind === "assistant" && !item.text.trim() && !item.streaming),
  );
  if (items.length === 0) {
    if (loading) {
      return (
        <Conversation>
          <ConversationContent scrollKey={scrollKey} />
        </Conversation>
      );
    }
    return (
      <Conversation>
        <ConversationEmptyState />
      </Conversation>
    );
  }
  const rows = groupTimeline(items);
  const sections = sectionizeTimeline(rows);
  const lastRow = rows[rows.length - 1];
  const followKey = lastRow ? (lastRow.kind === "tools" ? lastRow.id : lastRow.item.id) : undefined;
  const streaming = items.some(
    (item) => item.kind === "thinking" || (item.kind === "assistant" && item.streaming),
  );

  return (
    <Conversation>
      <ConversationContent scrollKey={scrollKey} followKey={followKey} streaming={streaming}>
        {sections.map((section, sectionIndex) => (
          <section key={sectionKey(section)} className="flex w-full min-w-0 flex-col gap-5">
            {section.map((row, rowIndex) => {
              const latest = sectionIndex === sections.length - 1 && rowIndex === section.length - 1;
              return row.kind === "tools" ? (
                <ToolCallsRow key={row.id} tools={row.tools} latest={latest} />
              ) : (
                <TimelineRow key={row.item.id} item={row.item} latest={latest} />
              );
            })}
          </section>
        ))}
      </ConversationContent>
    </Conversation>
  );
}

function TimelineRow({
  item,
  latest = false,
}: {
  item: Exclude<TimelineItem, { kind: "tool" }>;
  latest?: boolean;
}) {
  const latestProps = latest ? { "data-conversation-latest": "" } : {};
  switch (item.kind) {
    case "user":
      return <UserRow item={item} latest={latest} />;
    case "thinking":
      return (
        <div {...latestProps}>
          <Reasoning isStreaming>
            <ReasoningTrigger />
            <ReasoningContent>{thinkingCopy[item.phase]}</ReasoningContent>
          </Reasoning>
        </div>
      );
    case "assistant":
      return (
        <Message from="assistant" {...latestProps}>
          <MessageContent>
            <MessageResponse isAnimating={item.streaming}>
              {item.text || (item.streaming ? "…" : "")}
            </MessageResponse>
          </MessageContent>
        </Message>
      );
    case "approval":
      return (
        <div className="text-xs leading-4 text-muted-foreground" {...latestProps}>
          {item.status === "denied" ? "已拒绝工具调用" : "已批准工具调用"}
        </div>
      );
    case "context":
      return (
        <div className="text-xs leading-4 text-muted-foreground" {...latestProps}>
          上下文已压缩
          <span className="ml-2 font-mono text-muted-foreground/60">seq {item.baseEventSeq}</span>
        </div>
      );
    case "terminal":
      return (
        <div className="text-xs leading-4 text-muted-foreground" {...latestProps}>
          {item.status === "completed"
            ? "本轮完成"
            : item.status === "cancelled"
              ? "已取消"
              : `运行结束：${item.stopReason ?? item.status}`}
        </div>
      );
    default:
      return null;
  }
}

type ToolItem = Extract<TimelineItem, { kind: "tool" }>;

type TimelineRowModel =
  | { kind: "item"; item: Exclude<TimelineItem, { kind: "tool" }> }
  | { kind: "tools"; id: string; tools: ToolItem[] };

function groupTimeline(items: TimelineItem[]): TimelineRowModel[] {
  const rows: TimelineRowModel[] = [];
  let index = 0;
  while (index < items.length) {
    const item = items[index];
    if (item?.kind === "tool") {
      const tools: ToolItem[] = [];
      while (index < items.length) {
        const next = items[index];
        if (next?.kind !== "tool") {
          break;
        }
        tools.push(next);
        index += 1;
      }
      const first = tools[0];
      if (first) {
        rows.push({ kind: "tools", id: `tools:${first.id}`, tools });
      }
      continue;
    }
    if (item) {
      rows.push({ kind: "item", item });
    }
    index += 1;
  }
  return rows;
}

function rollupToolState(tools: ToolItem[]): ToolState {
  const order: ToolState[] = ["running", "error", "pending", "denied", "completed"];
  return order.find((state) => tools.some((tool) => tool.state === state)) ?? "completed";
}

function ToolCallsRow({ tools, latest = false }: { tools: ToolItem[]; latest?: boolean }) {
  return (
    <div {...(latest ? { "data-conversation-latest": "" } : {})}>
      <ToolGroup>
        <ToolGroupHeader count={tools.length} state={rollupToolState(tools)} />
        <ToolGroupContent>
          {tools.map((item) => (
            <Tool key={item.id}>
              <ToolHeader type={`tool-${item.name}`} state={item.state} />
              <ToolContent>
                <ToolInput input={item.arguments} />
                <ToolOutput output={item.output} errorText={item.error} />
              </ToolContent>
            </Tool>
          ))}
        </ToolGroupContent>
      </ToolGroup>
    </div>
  );
}

function sectionizeTimeline(rows: TimelineRowModel[]): TimelineRowModel[][] {
  const sections: TimelineRowModel[][] = [];
  let current: TimelineRowModel[] = [];
  for (const row of rows) {
    if (row.kind === "item" && row.item.kind === "user" && current.length > 0) {
      sections.push(current);
      current = [row];
      continue;
    }
    current.push(row);
  }
  if (current.length > 0) {
    sections.push(current);
  }
  return sections;
}

function sectionKey(section: TimelineRowModel[]): string {
  const first = section[0];
  if (!first) {
    return "section";
  }
  return first.kind === "tools" ? first.id : first.item.id;
}

function scrollParent(node: HTMLElement | null): HTMLElement | null {
  let current = node?.parentElement ?? null;
  while (current) {
    const overflowY = getComputedStyle(current).overflowY;
    if (overflowY === "auto" || overflowY === "scroll") {
      return current;
    }
    current = current.parentElement;
  }
  return null;
}

function UserRow({
  item,
  latest = false,
}: {
  item: Extract<TimelineItem, { kind: "user" }>;
  latest?: boolean;
}) {
  const sentinelRef = useRef<HTMLDivElement>(null);
  const headerRef = useRef<HTMLDivElement>(null);
  const clipRef = useRef<HTMLDivElement>(null);
  const [stuck, setStuck] = useState(false);
  const [open, setOpen] = useState(false);
  const [overflow, setOverflow] = useState(false);
  const compact = !open;

  useEffect(() => {
    const sentinel = sentinelRef.current;
    const header = headerRef.current;
    if (!sentinel || !header) {
      return;
    }
    const root = scrollParent(sentinel);
    const update = () => {
      if (!root) {
        setStuck(false);
        return;
      }
      const rootTop = root.getBoundingClientRect().top;
      const headerBox = header.getBoundingClientRect();
      const pinned =
        sentinel.getBoundingClientRect().bottom < headerBox.top - 1 &&
        Math.abs(headerBox.top - rootTop) <= 2;
      setStuck(pinned);
    };
    update();
    root?.addEventListener("scroll", update, { passive: true });
    const observer = new ResizeObserver(update);
    if (root) {
      observer.observe(root);
    }
    return () => {
      root?.removeEventListener("scroll", update);
      observer.disconnect();
    };
  }, []);

  useLayoutEffect(() => {
    const el = clipRef.current;
    if (!el || open) {
      setOverflow(false);
      return;
    }
    setOverflow(el.scrollHeight > el.clientHeight + 1);
  }, [item.text, open]);

  return (
    <>
      <div ref={sentinelRef} className="h-px w-full shrink-0" aria-hidden />
      <div
        ref={headerRef}
        data-stuck={stuck ? "true" : "false"}
        {...(latest ? { "data-conversation-latest": "" } : {})}
        className={cn("relative sticky top-0 z-20 bg-background pt-1.5", stuck && "pb-1")}
      >
        <Message
          from="user"
          className={cn(overflow && "cursor-pointer")}
          role={overflow || open ? "button" : undefined}
          tabIndex={overflow || open ? 0 : undefined}
          aria-expanded={overflow || open ? open : undefined}
          onClick={() => {
            if (overflow || open) {
              setOpen((current) => !current);
            }
          }}
          onKeyDown={(event) => {
            if (!overflow && !open) {
              return;
            }
            if (event.key === "Enter" || event.key === " ") {
              event.preventDefault();
              setOpen((current) => !current);
            }
          }}
        >
          <MessageContent>
            {item.queued ? (
              <div className="mb-1.5 text-[11px] text-muted-foreground">排队，当前轮结束后发送</div>
            ) : null}
            <div
              ref={clipRef}
              className={cn("relative", compact && "max-h-20 overflow-hidden")}
            >
              <p className="whitespace-pre-wrap break-words">{item.text}</p>
            </div>
          </MessageContent>
        </Message>
        {stuck ? (
          <div
            aria-hidden
            className="pointer-events-none absolute inset-x-0 top-full h-6 bg-gradient-to-b from-background to-transparent"
          />
        ) : null}
      </div>
    </>
  );
}

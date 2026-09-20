"use client";

import {
  compactToolDump,
  fileChangeFromTool,
  latestPlanDocIds,
  planPreviewFromTool,
  type FileChangePreview,
  type PlanPreview,
  type SessionState,
  type ThinkingPhase,
  type TimelineItem,
} from "@codedock/core/chat";
import {
  cn,
  Conversation,
  ConversationContent,
  ConversationEmptyState,
  LiveStatus,
  Message,
  MessageContent,
  MessageResponse,
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

import { FileText, FileCode2 } from "lucide-react";

const thinkingCopy: Record<ThinkingPhase, string> = {
  queued: "Queued",
  loading_context: "Loading context",
  running_llm: "Thinking",
  executing_tools: "Running tools",
  waiting_approval: "Waiting for approval",
  verifying: "Verifying",
  evaluating: "Reviewing",
  cancelling: "Cancelling",
};

type ToolItem = Extract<TimelineItem, { kind: "tool" }>;

// isLiveTimelineItem 判断这条是否仍是进行中的工作状态。
function isLiveTimelineItem(item: TimelineItem): boolean {
  switch (item.kind) {
    case "thinking":
      return true;
    case "assistant":
      return item.streaming;
    case "verify":
    case "evaluate":
      return item.status === "started";
    case "tool":
      return item.state === "pending" || item.state === "running";
    default:
      return false;
  }
}

export function ConversationTimeline({
  state,
  loading = false,
  scrollKey,
  emptyDescription,
  onOpenPlan,
  onOpenFile,
}: {
  state: SessionState;
  loading?: boolean;
  scrollKey?: string;
  emptyDescription?: string;
  onOpenPlan?: (preview: PlanPreview, extra?: { toolState?: ToolItem["state"]; error?: string }) => void;
  onOpenFile?: (change: FileChangePreview) => void;
}) {
  const items = state.items.filter(
    (item) =>
      !(item.kind === "approval" && item.status === "pending") &&
      !(item.kind === "user" && !item.text.trim()) &&
      !(item.kind === "assistant" && !item.text.trim()),
  );
  if (items.length === 0) {
    if (loading) {
      return (
        <Conversation key={scrollKey ?? "agent-draft"}>
          <ConversationContent scrollKey={scrollKey} />
        </Conversation>
      );
    }
    return (
      <Conversation key={scrollKey ?? "agent-draft"}>
        <ConversationEmptyState description={emptyDescription} />
      </Conversation>
    );
  }
  const rows = groupTimeline(items);
  const sections = sectionizeTimeline(rows);
  const latestDocs = latestPlanDocIds(items.filter((item): item is ToolItem => item.kind === "tool"));
  const lastRow = rows[rows.length - 1];
  const followKey = lastRow
    ? lastRow.kind === "tools"
      ? toolsFollowKey(lastRow)
      : lastRow.item.id
    : undefined;
  const streaming = items.some(
    (item) => item.kind === "thinking" || (item.kind === "assistant" && item.streaming),
  );

  return (
    <Conversation key={scrollKey ?? "agent-draft"}>
      <ConversationContent scrollKey={scrollKey} followKey={followKey} streaming={streaming}>
        {sections.map((section, sectionIndex) => (
          <section key={sectionKey(section)} className="flex w-full min-w-0 flex-col gap-5">
            {section.map((row, rowIndex) => {
              const latest = sectionIndex === sections.length - 1 && rowIndex === section.length - 1;
              return row.kind === "tools" ? (
                <ToolCallsRow
                  key={row.id}
                  tools={row.tools}
                  latest={latest}
                  live={latest && row.tools.some(isLiveTimelineItem)}
                  latestDocIds={latestDocs}
                  onOpenPlan={onOpenPlan}
                  onOpenFile={onOpenFile}
                />
              ) : (
                <TimelineRow
                  key={row.item.id}
                  item={row.item}
                  latest={latest}
                  live={latest && isLiveTimelineItem(row.item)}
                />
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
  live = false,
}: {
  item: Exclude<TimelineItem, { kind: "tool" }>;
  latest?: boolean;
  live?: boolean;
}) {
  const latestProps = latest ? { "data-conversation-latest": "" } : {};
  switch (item.kind) {
    case "user":
      return <UserRow item={item} latest={latest} />;
    case "thinking":
      return (
        <div className="text-sm leading-5 text-muted-foreground" {...latestProps}>
          <LiveStatus active={live}>{thinkingCopy[item.phase]}</LiveStatus>
        </div>
      );
    case "assistant":
      return (
        <Message from="assistant" {...latestProps}>
          <MessageContent>
            <MessageResponse isAnimating={item.streaming}>{item.text}</MessageResponse>
          </MessageContent>
        </Message>
      );
    case "approval":
      return (
        <div className="text-xs leading-4 text-muted-foreground" {...latestProps}>
          {item.approvalKind === "verify" || item.approvalKind === "evaluate"
            ? item.status === "denied"
              ? "Rolled back and cancelled"
              : "Override resolved"
            : item.status === "denied"
              ? "Tool calls denied"
              : "Tool calls approved"}
        </div>
      );
    case "verify":
      return (
        <div className="text-xs leading-4 text-muted-foreground" {...latestProps}>
          <LiveStatus active={live && item.status === "started"}>
            {item.status === "started"
              ? "Verifying"
              : item.status === "skipped"
                ? "No verify rules configured; treated as passed"
                : item.status === "passed"
                  ? "Verification passed"
                  : `Verification failed${item.output ? `: ${item.output.slice(0, 120)}` : ""}`}
          </LiveStatus>
        </div>
      );
    case "evaluate":
      return (
        <div className="text-xs leading-4 text-muted-foreground" {...latestProps}>
          <LiveStatus active={live && item.status === "started"}>
            {item.status === "started"
              ? "Reviewing"
              : item.status === "pass"
                ? "Review passed"
                : item.summary
                  ? `Review rejected: ${item.summary}`
                  : "Review requested more work"}
          </LiveStatus>
        </div>
      );
    case "context":
      return (
        <div className="text-xs leading-4 text-muted-foreground" {...latestProps}>
          Context compacted
          <span className="ml-2 font-mono text-muted-foreground/60">seq {item.baseEventSeq}</span>
        </div>
      );
    case "terminal":
      return (
        <div className="text-xs leading-4 text-muted-foreground" {...latestProps}>
          {item.status === "completed"
            ? "Run completed"
            : item.status === "cancelled"
              ? "Cancelled"
              : `Run ended: ${item.stopReason ?? item.status}`}
        </div>
      );
    default:
      return null;
  }
}

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

function ToolCallsRow({
  tools,
  latest = false,
  live = false,
  latestDocIds,
  onOpenPlan,
  onOpenFile,
}: {
  tools: ToolItem[];
  latest?: boolean;
  live?: boolean;
  latestDocIds: Set<string>;
  onOpenPlan?: (preview: PlanPreview, extra?: { toolState?: ToolItem["state"]; error?: string }) => void;
  onOpenFile?: (change: FileChangePreview) => void;
}) {
  const artifacts = tools.flatMap((item) => {
    const preview = planPreviewFromTool(item);
    if (preview) {
      return [{ item, preview, change: null }];
    }
    const change = fileChangeFromTool(item);
    if (change) {
      return [{ item, preview: null, change }];
    }
    return [];
  });
  return (
    <div
      className={artifacts.length > 0 ? "flex flex-col gap-2" : undefined}
      {...(latest ? { "data-conversation-latest": "" } : {})}
    >
      <ToolGroup>
        <ToolGroupHeader count={tools.length} state={rollupToolState(tools)} live={live} />
        <ToolGroupContent>
          {tools.map((item) => {
            if (item.name === "explore") {
              return <ExploreCard key={item.id} item={item} live={live} />;
            }
            const dump = compactToolDump(item);
            return (
              <Tool key={item.id}>
                <ToolHeader type={`tool-${item.name}`} state={item.state} live={live} />
                <ToolContent>
                  <ToolInput input={dump.input} />
                  <ToolOutput output={dump.output} errorText={item.error} />
                </ToolContent>
              </Tool>
            );
          })}
        </ToolGroupContent>
      </ToolGroup>
      {artifacts.length > 0 ? (
        <div className="flex flex-wrap gap-1.5 px-3">
          {artifacts.map(({ item, preview, change }) =>
            preview ? (
              <ArtifactChip
                key={`plan:${item.id}`}
                icon="plan"
                label={latestDocIds.has(item.id) ? `Plan ${preview.name}` : preview.name}
                onClick={
                  onOpenPlan
                    ? () => onOpenPlan(preview, { toolState: item.state, error: item.error })
                    : undefined
                }
              />
            ) : change ? (
              <ArtifactChip
                key={`file:${item.id}`}
                icon="file"
                label={change.path}
                onClick={onOpenFile ? () => onOpenFile(change) : undefined}
              />
            ) : null,
          )}
        </div>
      ) : null}
    </div>
  );
}

// ArtifactChip 把计划和文件改动收成一行入口，点开后到右侧窗口。
function ArtifactChip({
  icon,
  label,
  onClick,
}: {
  icon: "plan" | "file";
  label: string;
  onClick?: () => void;
}) {
  return (
    <button
      type="button"
      disabled={!onClick}
      onClick={onClick}
      className="inline-flex max-w-full items-center gap-1.5 rounded-md border border-border bg-muted px-2 py-1 text-[11px] text-muted-foreground hover:border-foreground/20 hover:text-foreground disabled:opacity-60"
    >
      {icon === "plan" ? <FileText className="size-3 shrink-0" /> : <FileCode2 className="size-3 shrink-0" />}
      <span className="truncate font-mono">{label}</span>
    </button>
  );
}

/** 折叠展示 explore 子代理结论，避免把长原文铺进主时间线。 */
function ExploreCard({ item, live = false }: { item: ToolItem; live?: boolean }) {
  const dump = compactToolDump(item);
  return (
    <Tool>
      <ToolHeader type="tool-explore" state={item.state} live={live} />
      <ToolContent>
        <div className="px-2 py-1 text-[11px] leading-4 text-muted-foreground">Explore subagent</div>
        <ToolInput input={dump.input} />
        <ToolOutput output={dump.output} errorText={item.error} />
      </ToolContent>
    </Tool>
  );
}

function toolsFollowKey(row: Extract<TimelineRowModel, { kind: "tools" }>): string {
  const parts = row.tools.map((tool) => {
    const preview = planPreviewFromTool(tool);
    if (!preview) {
      return tool.state;
    }
    return `${tool.state}:${preview.content.length}`;
  });
  return `${row.id}:${parts.join(",")}`;
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

/** 折叠时前三行保持清晰，第四行是渐隐区。 */
const USER_FOLD_LINES = 3;

function lineHeightPx(el: HTMLElement): number {
  const { lineHeight, fontSize } = getComputedStyle(el);
  const parsed = Number.parseFloat(lineHeight);
  if (Number.isFinite(parsed) && parsed > 0) {
    return parsed;
  }
  const size = Number.parseFloat(fontSize);
  return Number.isFinite(size) && size > 0 ? size * 1.25 : 20;
}

function userTextOverflows(clip: HTMLElement): boolean {
  const text = clip.querySelector("p");
  if (!text) {
    return false;
  }
  return text.scrollHeight > lineHeightPx(text) * USER_FOLD_LINES + 0.5;
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
  const [overflow, setOverflow] = useState(() => item.text.split("\n").length > USER_FOLD_LINES);
  const compact = !open;
  const folded = compact && overflow;
  const expandable = overflow || open;

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
    if (!el) {
      return;
    }
    const measure = () => {
      if (open) {
        return;
      }
      setOverflow(userTextOverflows(el));
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    const text = el.querySelector("p");
    if (text) {
      observer.observe(text);
    }
    void document.fonts?.ready.then(measure);
    return () => observer.disconnect();
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
          data-folded={folded ? "true" : "false"}
          className={cn(expandable && "cursor-pointer")}
          role={expandable ? "button" : undefined}
          tabIndex={expandable ? 0 : undefined}
          aria-expanded={expandable ? open : undefined}
          title={folded ? "消息已折叠，点击展开" : open ? "点击收起" : undefined}
          aria-label={folded ? "用户消息已折叠，点击展开" : undefined}
          onClick={() => {
            if (expandable) {
              setOpen((current) => !current);
            }
          }}
          onKeyDown={(event) => {
            if (!expandable) {
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
              <div className="mb-1.5 text-[11px] text-muted-foreground">Queued; will send after this run</div>
            ) : null}
            <div
              ref={clipRef}
              className={cn(
                "relative",
                compact &&
                  "max-h-[4lh] overflow-hidden [mask-image:linear-gradient(to_bottom,black_3lh,transparent_4lh)] [-webkit-mask-image:linear-gradient(to_bottom,black_3lh,transparent_4lh)]",
              )}
            >
              <p className="whitespace-pre-wrap break-words">{item.text}</p>
              {folded ? (
                <div
                  aria-hidden
                  className="pointer-events-none absolute inset-x-0 bottom-0 h-[1lh] bg-gradient-to-b from-muted/0 to-muted backdrop-blur-[3px]"
                />
              ) : null}
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

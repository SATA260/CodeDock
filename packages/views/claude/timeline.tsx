"use client";

import type { ClaudeTimelineItem } from "@codedock/core/claude";
import {
  cn,
  Conversation,
  ConversationContent,
  ConversationEmptyState,
  LiveStatus,
  Message,
  MessageContent,
  MessageResponse,
  Reasoning,
  ReasoningContent,
  ReasoningTrigger,
  Tool,
  ToolContent,
  ToolHeader,
  ToolOutput,
} from "@codedock/ui";
import { GitFork } from "lucide-react";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";

// ClaudeTimeline 用 Conversation 的 ResizeObserver 跟随最新一条，不轮询量高。
export function ClaudeTimeline({
  items,
  loading = false,
  scrollKey,
  canFork = false,
  onFork,
}: {
  items: ClaudeTimelineItem[];
  loading?: boolean;
  scrollKey?: string;
  canFork?: boolean;
  onFork?: () => Promise<void>;
}) {
  const visible = items.filter((item) => item.kind !== "notice" || Boolean(item.text));
  const last = visible[visible.length - 1];
  const followKey = last?.id;
  const streaming = visible.some((item) => item.streaming);
  if (visible.length === 0) {
    if (loading) {
      return (
        <Conversation key={scrollKey ?? "claude-draft"}>
          <ConversationContent scrollKey={scrollKey} />
        </Conversation>
      );
    }
    return (
      <Conversation key={scrollKey ?? "claude-draft"}>
        <ConversationEmptyState
          title="开始一段 Claude 对话"
          description="在输入栏下方选择模型、强度和权限档，点选即生效。左侧 + 挂文件，回复下可 fork。"
        />
      </Conversation>
    );
  }
  return (
    <Conversation key={scrollKey ?? "claude-draft"}>
      <ConversationContent scrollKey={scrollKey} followKey={followKey} streaming={streaming}>
        {visible.map((item, index) => (
          <TimelineRow
            key={`${item.id}-${index}`}
            item={item}
            latest={index === visible.length - 1}
            canFork={canFork}
            onFork={onFork}
          />
        ))}
      </ConversationContent>
    </Conversation>
  );
}

// TimelineRow 按实录种类落到与 Local / Codex 相同的瀑布组件。
function TimelineRow({
  item,
  latest = false,
  canFork,
  onFork,
}: {
  item: ClaudeTimelineItem;
  latest?: boolean;
  canFork: boolean;
  onFork?: () => Promise<void>;
}) {
  const latestProps = latest ? { "data-conversation-latest": "" } : {};
  switch (item.kind) {
    case "user":
      return (
        <div {...latestProps}>
          <Message from="user">
            <MessageContent>
              <Fold watch={item.text}>
                <p className="whitespace-pre-wrap break-words">{item.text}</p>
              </Fold>
            </MessageContent>
          </Message>
        </div>
      );
    case "text":
      return (
        <div {...latestProps}>
          <Message from="assistant">
            <MessageContent>
              <Fold watch={item.text} disabled={item.streaming}>
                <MessageResponse isAnimating={item.streaming}>
                  {item.text || (item.streaming ? "" : "")}
                </MessageResponse>
                {!item.text && item.streaming ? <LiveStatus active>Writing</LiveStatus> : null}
              </Fold>
            </MessageContent>
          </Message>
          <ForkAction disabled={!canFork} onFork={onFork} />
        </div>
      );
    case "reasoning":
      return (
        <div {...latestProps}>
          <Reasoning isStreaming={item.streaming}>
            <ReasoningTrigger />
            <ReasoningContent>
              {item.text ? item.text : <LiveStatus active={item.streaming}>Thinking</LiveStatus>}
            </ReasoningContent>
          </Reasoning>
        </div>
      );
    case "command":
      return (
        <div {...latestProps}>
          <Tool defaultOpen>
            <ToolHeader type={`tool-${item.command || "command"}`} state={item.streaming ? "running" : "completed"} />
            <ToolContent>
              <Fold watch={item.text || item.command}>
                <ToolOutput output={item.text || item.command} />
              </Fold>
            </ToolContent>
          </Tool>
        </div>
      );
    case "file_change":
      return (
        <div className="rounded-lg border border-border bg-card px-3 py-2 text-xs" {...latestProps}>
          <div className="font-medium text-foreground">改文件 {item.paths?.join(", ")}</div>
          {item.diff ? (
            <Fold watch={item.diff}>
              <pre className="mt-1 overflow-x-auto font-mono text-[11px] text-muted-foreground">{item.diff}</pre>
            </Fold>
          ) : null}
        </div>
      );
    case "plan":
      return (
        <div {...latestProps}>
          <Fold watch={item.text}>
            <div className="whitespace-pre-wrap text-sm text-muted-foreground">{item.text}</div>
          </Fold>
        </div>
      );
    case "notice":
      return (
        <div className="text-xs text-muted-foreground" {...latestProps}>
          {item.text}
        </div>
      );
    default:
      return (
        <div {...latestProps}>
          <Message from="assistant">
            <MessageContent>
              <MessageResponse>{item.text || item.kind}</MessageResponse>
            </MessageContent>
          </Message>
        </div>
      );
  }
}

// ForkAction 只挂在 AI 回复下，按官方 --fork-session 开出独立副本。
function ForkAction({
  disabled,
  onFork,
}: {
  disabled: boolean;
  onFork?: () => Promise<void>;
}) {
  if (!onFork) {
    return null;
  }
  return (
    <div className="mt-1 flex justify-start">
      <button
        type="button"
        disabled={disabled}
        className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
        onClick={() => void onFork()}
      >
        <GitFork className="size-3" />
        fork
      </button>
    </div>
  );
}

// Fold 用 ResizeObserver 判断是否溢出，不按定时器量高。
function Fold({
  watch,
  disabled = false,
  children,
}: {
  watch?: string;
  disabled?: boolean;
  children: ReactNode;
}) {
  const clipRef = useRef<HTMLDivElement>(null);
  const innerRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [overflow, setOverflow] = useState(false);
  const clipped = !open && !disabled;

  useLayoutEffect(() => {
    const clip = clipRef.current;
    const inner = innerRef.current;
    if (!clip || !inner || !clipped) {
      return;
    }
    const measure = () => {
      setOverflow(clip.scrollHeight > clip.clientHeight + 1);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(inner);
    return () => observer.disconnect();
  }, [clipped, watch]);

  return (
    <div>
      <div
        ref={clipRef}
        className={cn(
          "relative",
          clipped && "max-h-[12lh] overflow-hidden",
          clipped && overflow && "cursor-pointer",
          clipped &&
            overflow &&
            "[mask-image:linear-gradient(to_bottom,black_9lh,transparent_12lh)] [-webkit-mask-image:linear-gradient(to_bottom,black_9lh,transparent_12lh)]",
        )}
        onClick={(event) => {
          if (!clipped || !overflow) {
            return;
          }
          if ((event.target as HTMLElement).closest("a, button, input, textarea, select")) {
            return;
          }
          setOpen(true);
        }}
      >
        <div ref={innerRef}>{children}</div>
      </div>
      {clipped && overflow ? (
        <button
          type="button"
          className="mt-1 text-[11px] text-muted-foreground hover:text-foreground"
          onClick={() => setOpen(true)}
        >
          展开
        </button>
      ) : null}
      {open ? (
        <button
          type="button"
          className="mt-1 text-[11px] text-muted-foreground hover:text-foreground"
          onClick={() => setOpen(false)}
        >
          收起
        </button>
      ) : null}
    </div>
  );
}

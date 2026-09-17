"use client";

import type { CodexViewState, TimelineItem } from "@codedock/core/codex";
import {
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
  ToolHeader,
  ToolOutput,
} from "@codedock/ui";
import { GitFork } from "lucide-react";

export function CodexTimeline({
  state,
  loading = false,
  scrollKey,
  canFork = false,
  onFork,
}: {
  state: CodexViewState;
  loading?: boolean;
  scrollKey?: string;
  canFork?: boolean;
  onFork?: () => Promise<void>;
}) {
  const items = state.items.filter((item) => item.kind !== "turn" || Boolean(item.text));
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
        <ConversationEmptyState
          title="开始一段 Codex 对话"
          description="在输入栏下方选择模型、强度、模式和权限，点选即生效。左侧 + 挂文件，消息下可分叉。"
        />
      </Conversation>
    );
  }
  return (
    <Conversation>
      <ConversationContent scrollKey={scrollKey}>
        {items.map((item, index) => (
          <TimelineRow
            key={`${item.id}-${index}`}
            item={item}
            canFork={canFork}
            onFork={onFork}
          />
        ))}
      </ConversationContent>
    </Conversation>
  );
}

function TimelineRow({
  item,
  canFork,
  onFork,
}: {
  item: TimelineItem;
  canFork: boolean;
  onFork?: () => Promise<void>;
}) {
  switch (item.kind) {
    case "user":
      return (
        <div>
          <Message from="user">
            <MessageContent>
              <p className="whitespace-pre-wrap break-words">{item.text}</p>
            </MessageContent>
          </Message>
          <ForkAction align="end" disabled={!canFork} onFork={onFork} />
        </div>
      );
    case "text":
      return (
        <div>
          <Message from="assistant">
            <MessageContent>
              <MessageResponse isAnimating={item.streaming}>{item.text || (item.streaming ? "…" : "")}</MessageResponse>
            </MessageContent>
          </Message>
          <ForkAction align="start" disabled={!canFork} onFork={onFork} />
        </div>
      );
    case "reasoning":
      return (
        <Reasoning isStreaming={item.streaming}>
          <ReasoningTrigger />
          <ReasoningContent>{item.text || "思考中"}</ReasoningContent>
        </Reasoning>
      );
    case "command":
      return (
        <Tool defaultOpen>
          <ToolHeader type={`tool-${item.command || "command"}`} state={item.status === "completed" ? "completed" : "running"} />
          <ToolContent>
            <ToolOutput output={item.text} />
          </ToolContent>
        </Tool>
      );
    case "file_change":
      return (
        <div className="rounded-lg border border-border bg-card px-3 py-2 text-xs">
          <div className="font-medium text-foreground">改文件 {item.paths?.join(", ")}</div>
          {item.diff ? (
            <pre className="mt-1 max-h-40 overflow-auto font-mono text-[11px] text-muted-foreground">{item.diff}</pre>
          ) : null}
        </div>
      );
    case "plan":
      return <div className="text-sm text-muted-foreground whitespace-pre-wrap">{item.text}</div>;
    case "notice":
    case "turn":
      return <div className="text-xs text-muted-foreground">{item.text}</div>;
    default:
      return null;
  }
}

function ForkAction({
  align,
  disabled,
  onFork,
}: {
  align: "start" | "end";
  disabled: boolean;
  onFork?: () => Promise<void>;
}) {
  if (!onFork) {
    return null;
  }
  return (
    <div className={align === "end" ? "mt-1 flex justify-end" : "mt-1 flex justify-start"}>
      <button
        type="button"
        disabled={disabled}
        className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
        onClick={() => void onFork()}
      >
        <GitFork className="size-3" />
        分叉
      </button>
    </div>
  );
}

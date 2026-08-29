"use client";

import type { SessionState, ThinkingPhase, TimelineItem } from "@codedock/core/chat";
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
  ToolInput,
  ToolOutput,
} from "@codedock/ui";

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
  return (
    <Conversation>
      <ConversationContent scrollKey={scrollKey}>
        {items.map((item) => (
          <TimelineRow key={item.id} item={item} />
        ))}
      </ConversationContent>
    </Conversation>
  );
}

function TimelineRow({ item }: { item: TimelineItem }) {
  switch (item.kind) {
    case "user":
      return (
        <Message from="user">
          <MessageContent>
            <p className="whitespace-pre-wrap break-words">{item.text}</p>
          </MessageContent>
        </Message>
      );
    case "thinking":
      return (
        <Reasoning isStreaming>
          <ReasoningTrigger />
          <ReasoningContent>{thinkingCopy[item.phase]}</ReasoningContent>
        </Reasoning>
      );
    case "assistant":
      return (
        <Message from="assistant">
          <MessageContent>
            <MessageResponse isAnimating={item.streaming}>
              {item.text || (item.streaming ? "…" : "")}
            </MessageResponse>
          </MessageContent>
        </Message>
      );
    case "tool":
      return (
        <Tool defaultOpen={item.state === "running" || item.state === "error"}>
          <ToolHeader type={`tool-${item.name}`} state={item.state} />
          <ToolContent>
            <ToolInput input={item.arguments} />
            <ToolOutput output={item.output} errorText={item.error} />
          </ToolContent>
        </Tool>
      );
    case "approval":
      return (
        <div className="text-xs text-muted-foreground">
          {item.status === "denied" ? "已拒绝工具调用" : "已批准工具调用"}
        </div>
      );
    case "context":
      return (
        <div className="text-xs text-muted-foreground">
          上下文已压缩
          <span className="ml-2 font-mono text-muted-foreground/60">seq {item.baseEventSeq}</span>
        </div>
      );
    case "terminal":
      return (
        <div className="text-xs text-muted-foreground">
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

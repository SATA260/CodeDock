"use client";

import type { SessionState, ThinkingPhase, TimelineItem } from "@codedock/core/chat";
import {
  Button,
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
import { useEffect, useState } from "react";

const thinkingCopy: Record<ThinkingPhase, string> = {
  queued: "排队中",
  loading_context: "正在装载上下文",
  running_llm: "正在思考",
};

export function ConversationTimeline({
  state,
  loading = false,
  scrollKey,
  onEditQueued,
}: {
  state: SessionState;
  loading?: boolean;
  scrollKey?: string;
  onEditQueued?: (messageId: string, text: string) => Promise<void>;
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
          <TimelineRow key={item.id} item={item} onEditQueued={onEditQueued} />
        ))}
      </ConversationContent>
    </Conversation>
  );
}

function TimelineRow({
  item,
  onEditQueued,
}: {
  item: TimelineItem;
  onEditQueued?: (messageId: string, text: string) => Promise<void>;
}) {
  switch (item.kind) {
    case "user":
      return <UserRow item={item} onEditQueued={onEditQueued} />;
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

function UserRow({
  item,
  onEditQueued,
}: {
  item: Extract<TimelineItem, { kind: "user" }>;
  onEditQueued?: (messageId: string, text: string) => Promise<void>;
}) {
  const persisted = !item.messageId.startsWith("pending:");
  const canEdit = Boolean(item.queued && onEditQueued && persisted);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(item.text);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!editing) {
      setDraft(item.text);
    }
  }, [item.text, editing]);

  useEffect(() => {
    if (!item.queued && editing) {
      setEditing(false);
    }
  }, [item.queued, editing]);

  const save = async () => {
    const next = draft.trim();
    if (!next || !onEditQueued || next === item.text) {
      setEditing(false);
      setDraft(item.text);
      return;
    }
    setSaving(true);
    try {
      await onEditQueued(item.messageId, next);
      setEditing(false);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Message from="user">
      <MessageContent>
        {item.queued ? (
          <div className="mb-1.5 text-[11px] text-muted-foreground">排队，当前轮结束后发送</div>
        ) : null}
        {editing ? (
          <div className="space-y-2">
            <textarea
              value={draft}
              disabled={saving}
              rows={3}
              className="w-full resize-none rounded-md border border-border bg-background px-2.5 py-2 text-sm leading-6 text-foreground outline-none focus:border-ring"
              onChange={(event) => setDraft(event.currentTarget.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  void save();
                }
                if (event.key === "Escape") {
                  setEditing(false);
                  setDraft(item.text);
                }
              }}
            />
            <div className="flex justify-end gap-1.5">
              <Button
                size="sm"
                variant="ghost"
                disabled={saving}
                onClick={() => {
                  setEditing(false);
                  setDraft(item.text);
                }}
              >
                取消
              </Button>
              <Button size="sm" disabled={saving || !draft.trim()} onClick={() => void save()}>
                保存
              </Button>
            </div>
          </div>
        ) : (
          <div>
            <p className="whitespace-pre-wrap break-words">{item.text}</p>
            {canEdit ? (
              <button
                type="button"
                className="mt-1.5 text-[11px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
                onClick={() => setEditing(true)}
              >
                改内容
              </button>
            ) : null}
          </div>
        )}
      </MessageContent>
    </Message>
  );
}

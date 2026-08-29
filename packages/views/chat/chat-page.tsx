"use client";

import type { AgentMode, TimelineItem } from "@codedock/core/chat";
import { useState } from "react";

import { useAgent } from "../provider.tsx";
import { ApprovalDock } from "./approval-dock.tsx";
import { ConversationTimeline } from "./conversation-timeline.tsx";
import { useSessionList } from "./hooks/use-session-list.ts";
import { useSessionTimeline } from "./hooks/use-session-timeline.ts";
import { PromptBar } from "./prompt-bar.tsx";
import { SessionSidebar } from "./session-sidebar.tsx";

export type ChatPageProps = {
  sessionId?: string;
  onOpenSession: (id: string) => void;
  onNewConversation: () => void;
  brandSrc?: string;
};

export function ChatPage({
  sessionId,
  onOpenSession,
  onNewConversation,
  brandSrc,
}: ChatPageProps) {
  const { client } = useAgent();
  const list = useSessionList();
  const timeline = useSessionTimeline(sessionId);
  const [starting, setStarting] = useState(false);
  const [composerError, setComposerError] = useState<string | null>(null);

  const pendingApproval = timeline.state.items.find(
    (item): item is Extract<TimelineItem, { kind: "approval" }> =>
      item.kind === "approval" && item.status === "pending",
  );

  const onSend = async (text: string, mode: AgentMode) => {
    setComposerError(null);
    if (sessionId) {
      await timeline.send(text, mode);
      await list.refresh();
      return;
    }
    setStarting(true);
    try {
      const session = await list.createSession();
      await client.startRun(session.id, { content: text, mode });
      await list.refresh();
      onOpenSession(session.id);
    } catch (err) {
      setComposerError(err instanceof Error ? err.message : "发送失败");
      await list.refresh();
    } finally {
      setStarting(false);
    }
  };

  return (
    <div className="flex h-dvh overflow-hidden bg-background text-foreground">
      <SessionSidebar
        sessions={list.sessions}
        currentId={sessionId}
        busy={list.busy}
        error={list.error}
        onCreate={onNewConversation}
        onSelect={onOpenSession}
        brandSrc={brandSrc}
      />
      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-12 items-center border-b border-border px-4 text-sm text-muted-foreground">
          {sessionId ? "对话" : "新对话"}
        </header>
        {timeline.error || composerError ? (
          <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-red-300">
            {timeline.error ?? composerError}
          </div>
        ) : null}
        <ConversationTimeline
          state={timeline.state}
          loading={timeline.loading}
          scrollKey={sessionId}
        />
        <ApprovalDock item={pendingApproval} onDecide={timeline.decide} />
        <PromptBar
          running={timeline.running}
          sending={timeline.sending || starting}
          onSend={onSend}
          onCancel={timeline.cancel}
        />
      </main>
    </div>
  );
}

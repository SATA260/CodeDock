"use client";

import type { AgentMode, TimelineItem } from "@codedock/core/chat";
import { Button } from "@codedock/ui";
import { useState, type ReactNode } from "react";

import { useAgent } from "../provider.tsx";
import { ApprovalDock } from "./approval-dock.tsx";
import { ConversationTimeline } from "./conversation-timeline.tsx";
import { PendingDock } from "./pending-dock.tsx";
import { useSessionList } from "./hooks/use-session-list.ts";
import { useSessionTimeline } from "./hooks/use-session-timeline.ts";
import { shortWorkspace } from "./lib/format.ts";
import {
  clearLastWorkspace,
  createSessionError,
  readLastWorkspace,
  writeLastWorkspace,
} from "./lib/workspace.ts";
import { PromptBar } from "./prompt-bar.tsx";
import { SessionSidebar } from "./session-sidebar.tsx";
import { WorkspacePicker } from "./workspace-picker.tsx";

export type ChatPageProps = {
  sessionId?: string;
  onOpenSession: (id: string) => void;
  onNewConversation: () => void;
  brandSrc?: string;
  headerActions?: ReactNode;
};

export function ChatPage({
  sessionId,
  onOpenSession,
  onNewConversation,
  brandSrc,
  headerActions,
}: ChatPageProps) {
  const { client, listDirectories } = useAgent();
  const list = useSessionList();
  const timeline = useSessionTimeline(sessionId);
  const [starting, setStarting] = useState(false);
  const [composerError, setComposerError] = useState<string | null>(null);
  const [workspaceDraft, setWorkspaceDraft] = useState(readLastWorkspace);
  const [pickerOpen, setPickerOpen] = useState(false);

  const frozenWorkspace =
    list.sessions.find((session) => session.id === sessionId)?.workspace_id ?? "";
  const workspaceTitle = sessionId ? frozenWorkspace : workspaceDraft.trim() || "默认仓库目录";
  const workspaceLabel = sessionId
    ? frozenWorkspace
      ? shortWorkspace(frozenWorkspace)
      : ""
    : shortWorkspace(workspaceTitle);

  const pendingApprovals = timeline.state.items.filter(
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
      const session = await list.createSession(workspaceDraft);
      writeLastWorkspace(session.workspace_id);
      setWorkspaceDraft(session.workspace_id);
      await client.startRun(session.id, { content: text, mode });
      await list.refresh();
      onOpenSession(session.id);
    } catch (err) {
      setComposerError(createSessionError(err, "发送失败"));
      await list.refresh();
    } finally {
      setStarting(false);
    }
  };

  return (
    <div className="flex h-full overflow-hidden bg-background text-foreground">
      <SessionSidebar
        sessions={list.sessions}
        currentId={sessionId}
        busy={list.busy}
        error={list.error}
        onCreate={onNewConversation}
        onSelect={onOpenSession}
        onRecover={async (runId) => {
          await timeline.recover(runId);
          await list.refresh();
        }}
        onDelete={async (session) => {
          const deletedId = session.id;
          await list.removeSession(session);
          if (sessionId === deletedId) {
            onNewConversation();
          }
        }}
        canRecoverCurrent={timeline.canRecover}
        brandSrc={brandSrc}
      />
      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-10 items-center gap-3 border-b border-border px-4 text-sm leading-5 text-muted-foreground">
          <span>{sessionId ? "对话" : "新对话"}</span>
          {workspaceLabel ? (
            <span className="min-w-0 truncate font-mono text-[11px]" title={workspaceTitle}>
              {workspaceLabel}
            </span>
          ) : null}
          {timeline.canRecover ? (
            <Button
              size="sm"
              variant="secondary"
              onClick={async () => {
                await timeline.recover();
                await list.refresh();
              }}
            >
              恢复
            </Button>
          ) : null}
          {headerActions ? <div className="ml-auto flex items-center gap-2">{headerActions}</div> : null}
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
        <div className="relative z-30 shrink-0">
          <PendingDock
            items={timeline.pending}
            editingId={timeline.editingId}
            onBeginEdit={timeline.beginEditPending}
            onCancelEdit={timeline.cancelEditPending}
            onSave={timeline.savePending}
            onDelete={timeline.deletePending}
            onSendNow={timeline.sendNow}
          />
          <ApprovalDock items={pendingApprovals} onDecide={timeline.decide} />
          <PromptBar
            running={timeline.running}
            sending={timeline.sending || starting}
            workspace={sessionId ? undefined : workspaceDraft}
            onPickWorkspace={
              sessionId || !listDirectories
                ? undefined
                : async () => {
                    setPickerOpen(true);
                  }
            }
            onClearWorkspace={
              sessionId
                ? undefined
                : () => {
                    clearLastWorkspace();
                    setWorkspaceDraft("");
                  }
            }
            onSend={onSend}
            onCancel={timeline.cancel}
          />
        </div>
      </main>
      {pickerOpen && listDirectories ? (
        <WorkspacePicker
          initialPath={workspaceDraft || undefined}
          listDirectories={listDirectories}
          onCancel={() => setPickerOpen(false)}
          onSelect={(path) => {
            writeLastWorkspace(path);
            setWorkspaceDraft(path);
            setPickerOpen(false);
          }}
        />
      ) : null}
    </div>
  );
}

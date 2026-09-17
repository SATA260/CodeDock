"use client";

import type { ApprovalMode, Session, TimelineItem, WorkMode } from "@codedock/core/chat";
import type { Session as CodexSession } from "@codedock/core/codex";
import { Button } from "@codedock/ui";
import { FolderOpen } from "lucide-react";
import { useMemo, useState, useEffect, type ReactNode } from "react";

import { CodexPane } from "../codex/codex-pane.tsx";
import { useCodexSessionList } from "../codex/hooks/use-session-list.ts";
import { useCodex } from "../codex/provider.tsx";
import { useAgent } from "../provider.tsx";
import { ApprovalDock } from "./approval-dock.tsx";
import { ConversationTimeline } from "./conversation-timeline.tsx";
import { useSessionList } from "./hooks/use-session-list.ts";
import { useSessionTimeline } from "./hooks/use-session-timeline.ts";
import { shortWorkspace } from "./lib/format.ts";
import {
  clearLastWorkspace,
  createSessionError,
  readLastWorkspace,
  writeLastWorkspace,
} from "./lib/workspace.ts";
import { PendingDock } from "./pending-dock.tsx";
import { PromptBar } from "./prompt-bar.tsx";
import { SessionSidebar, type SidebarSession } from "./session-sidebar.tsx";

export type SessionEngine = "agent" | "codex";

export type ChatPageProps = {
  sessionId?: string;
  engine?: SessionEngine;
  onOpenSession: (id: string, engine?: SessionEngine) => void;
  onNewConversation: () => void;
  brandSrc?: string;
  headerActions?: ReactNode;
};

export function ChatPage({
  sessionId,
  engine,
  onOpenSession,
  onNewConversation,
  brandSrc,
  headerActions,
}: ChatPageProps) {
  const { client, pickDirectory, pickFiles } = useAgent();
  const { client: codexClient } = useCodex();
  const list = useSessionList();
  const codexList = useCodexSessionList();
  const [draftEngine, setDraftEngine] = useState<SessionEngine>(engine ?? "agent");
  const activeEngine: SessionEngine = sessionId ? (engine ?? "agent") : draftEngine;
  useEffect(() => {
    if (engine) {
      setDraftEngine(engine);
    }
  }, [engine]);
  const timeline = useSessionTimeline(activeEngine === "agent" ? sessionId : undefined);
  const [starting, setStarting] = useState(false);
  const [composerError, setComposerError] = useState<string | null>(null);
  const [workspaceDraft, setWorkspaceDraft] = useState("");
  const [pickingWorkspace, setPickingWorkspace] = useState(false);
  useEffect(() => {
    setWorkspaceDraft(readLastWorkspace());
  }, []);

  const sessions = useMemo(
    () => mergeSessions(list.sessions, codexList.sessions),
    [codexList.sessions, list.sessions],
  );
  const currentKey = sessionId ? `${activeEngine}:${sessionId}` : undefined;
  const current = sessions.find((session) => `${session.engine}:${session.id}` === currentKey);

  const frozenWorkspace =
    activeEngine === "agent"
      ? (timeline.workspaceId ?? current?.workspace_id ?? "")
      : (current?.workspace_id ?? "");
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

  const onSend = async (text: string, mode: WorkMode, approval: ApprovalMode) => {
    setComposerError(null);
    if (sessionId) {
      await timeline.send(text, mode, approval);
      await list.refresh();
      return;
    }
    setStarting(true);
    try {
      const session = await list.createSession(workspaceDraft);
      writeLastWorkspace(session.workspace_id);
      setWorkspaceDraft(session.workspace_id);
      await client.startRun(session.id, { content: text, mode, approval });
      await list.refresh();
      onOpenSession(session.id, "agent");
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
        sessions={sessions}
        currentId={currentKey}
        busy={list.busy || codexList.busy}
        error={activeEngine === "codex" ? codexList.error : list.error}
        hasMore={codexList.hasMore}
        onLoadMore={codexList.hasMore ? () => void codexList.loadMore() : undefined}
        onCreate={onNewConversation}
        onSelect={(id, nextEngine) => onOpenSession(id, nextEngine ?? "agent")}
        onRecover={async (runId) => {
          await timeline.recover(runId);
          await list.refresh();
        }}
        onDelete={async (session) => {
          const deletedId = session.id;
          const deletedEngine = session.engine ?? "agent";
          if (session.engine === "codex") {
            await codexClient.archiveSession(session.id);
            await codexList.refresh();
          } else {
            await list.removeSession(session);
          }
          if (sessionId === deletedId && (engine ?? "agent") === deletedEngine) {
            onNewConversation();
          }
        }}
        canRecoverCurrent={activeEngine === "agent" && timeline.canRecover}
        canArchive={activeEngine === "codex" && Boolean(sessionId)}
        onArchive={
          activeEngine === "codex" && sessionId
            ? async () => {
                await codexClient.archiveSession(sessionId);
                await codexList.refresh();
                onNewConversation();
              }
            : undefined
        }
        brandSrc={brandSrc}
      />
      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-10 items-center gap-3 border-b border-border px-4 text-sm leading-5 text-muted-foreground">
          <span className="shrink-0">
            {sessionId ? (activeEngine === "codex" ? "Codex 对话" : "对话") : "新对话"}
          </span>
          {sessionId ? null : (
            <div className="flex shrink-0 items-center rounded-md border border-border p-0.5" role="group" aria-label="会话模式">
              <EngineToggle
                active={draftEngine === "agent"}
                onClick={() => {
                  setDraftEngine("agent");
                  setComposerError(null);
                }}
              >
                Agent
              </EngineToggle>
              <EngineToggle
                active={draftEngine === "codex"}
                onClick={() => {
                  setDraftEngine("codex");
                  setComposerError(null);
                }}
              >
                Codex
              </EngineToggle>
            </div>
          )}
          {sessionId ? (
            workspaceLabel ? (
              <>
                <span className="text-border">·</span>
                <span
                  className="min-w-0 truncate font-mono text-xs text-muted-foreground/80"
                  title={workspaceTitle}
                  data-workspace-path={workspaceTitle}
                >
                  {workspaceLabel}
                </span>
              </>
            ) : null
          ) : (
            <div className="flex min-w-0 items-center gap-1.5">
              <button
                data-workspace-pick=""
                type="button"
                disabled={!pickDirectory || pickingWorkspace}
                title={workspaceDraft.trim() || "打开系统目录选择框"}
                className="flex min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-left text-xs leading-5 text-foreground hover:bg-muted disabled:opacity-50"
                onClick={() => {
                  if (!pickDirectory || pickingWorkspace) {
                    return;
                  }
                  setComposerError(null);
                  setPickingWorkspace(true);
                  void pickDirectory({ start: workspaceDraft.trim() || undefined })
                    .then((path) => {
                      if (!path) {
                        return;
                      }
                      setWorkspaceDraft(path);
                      writeLastWorkspace(path);
                    })
                    .catch((err: unknown) => {
                      setComposerError(err instanceof Error ? err.message : "无法选择目录");
                    })
                    .finally(() => {
                      setPickingWorkspace(false);
                    });
                }}
              >
                <FolderOpen className="size-3.5 shrink-0 text-muted-foreground" />
                <span
                  className="min-w-0 truncate font-mono"
                  title={workspaceTitle}
                  data-workspace-path={workspaceTitle}
                >
                  {workspaceDraft.trim() ? workspaceLabel : "选择目录（默认仓库）"}
                </span>
              </button>
              {workspaceDraft.trim() ? (
                <button
                  type="button"
                  className="shrink-0 text-xs text-muted-foreground hover:text-foreground"
                  onClick={() => {
                    setWorkspaceDraft("");
                    clearLastWorkspace();
                  }}
                >
                  默认
                </button>
              ) : null}
            </div>
          )}
          {activeEngine === "agent" && timeline.canRecover ? (
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
        {activeEngine === "codex" ? (
          <>
            {composerError ? (
              <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-red-300">
                {composerError}
              </div>
            ) : null}
            <CodexPane
              sessionId={sessionId}
              workspace={sessionId ? frozenWorkspace : workspaceDraft}
              pickFiles={pickFiles}
              onOpenSession={(id) => onOpenSession(id, "codex")}
              onNewConversation={onNewConversation}
              onListChange={codexList.refresh}
            />
          </>
        ) : (
          <>
            {timeline.error || composerError ? (
              <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-red-300">
                {timeline.error ?? composerError}
              </div>
            ) : null}
            <ConversationTimeline
              state={timeline.state}
              loading={timeline.loading}
              scrollKey={sessionId}
              emptyDescription={
                sessionId ? undefined : "选择会话模式与工作目录，或直接发送以使用默认仓库目录。"
              }
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
                onSend={onSend}
                onCancel={timeline.cancel}
              />
            </div>
          </>
        )}
      </main>
    </div>
  );
}

function EngineToggle({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      className={
        active
          ? "h-6 rounded px-2 text-xs font-medium text-foreground bg-muted"
          : "h-6 rounded px-2 text-xs text-muted-foreground hover:text-foreground"
      }
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function mergeSessions(agent: Session[], codex: CodexSession[]): SidebarSession[] {
  const mapped: SidebarSession[] = [
    ...agent.map((session) => ({ ...session, engine: "agent" as const })),
    ...codex.map(asSidebarSession),
  ];
  const seen = new Map<string, SidebarSession>();
  for (const session of mapped) {
    const key = `${session.engine ?? "agent"}:${session.id}`;
    const prev = seen.get(key);
    if (!prev || prev.updated_at < session.updated_at) {
      seen.set(key, session);
    }
  }
  return [...seen.values()].sort((left, right) => (left.updated_at < right.updated_at ? 1 : -1));
}

function asSidebarSession(session: CodexSession): SidebarSession {
  return {
    id: session.id,
    tenant_id: "",
    user_id: "",
    agent_id: "codex",
    workspace_id: session.cwd ?? "",
    status: session.archived ? "archived" : "active",
    last_event_seq: 1,
    compaction_seq: 0,
    summary: session.title || session.preview,
    created_at: stampToIso(session.created_at),
    updated_at: stampToIso(session.updated_at),
    engine: "codex",
  };
}

function stampToIso(value?: number): string {
  if (!value) {
    return new Date(0).toISOString();
  }
  const ms = value > 1e11 ? value : value * 1000;
  const date = new Date(ms);
  if (Number.isNaN(date.getTime())) {
    return new Date(0).toISOString();
  }
  return date.toISOString();
}

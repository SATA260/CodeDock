"use client";

import type { ApprovalMode, Session, TimelineItem, WorkMode } from "@codedock/core/chat";
import type { Session as CodexSession } from "@codedock/core/codex";
import { Button } from "@codedock/ui";
import { useEffect, useMemo, useState, type ReactNode } from "react";

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
import { NewConversation } from "./new-conversation.tsx";
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
  codexIconSrc?: string;
  headerActions?: ReactNode;
};

// ChatPage 组合会话侧栏、时间线与输入条。
export function ChatPage({
  sessionId,
  engine,
  onOpenSession,
  onNewConversation,
  brandSrc,
  codexIconSrc,
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

  // 上次目录只在本机 localStorage，等 hydration 后再读，避免 SSR 文本对不上。
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

  const pickWorkspace = () => {
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
  };

  const hideSession = async (session: SidebarSession) => {
    const hiddenId = session.id;
    const hiddenEngine = session.engine ?? "agent";
    if (session.engine === "codex") {
      await codexClient.archiveSession(session.id);
      await codexList.refresh();
    } else {
      await list.removeSession(session);
    }
    if (sessionId === hiddenId && (engine ?? "agent") === hiddenEngine) {
      onNewConversation();
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
        canRecoverCurrent={activeEngine === "agent" && timeline.canRecover}
        onArchive={async (session) => {
          await hideSession(session);
        }}
        brandSrc={brandSrc}
        codexIconSrc={codexIconSrc}
      />
      <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        {sessionId ? (
          <header className="flex h-10 items-center gap-3 border-b border-border px-4 text-sm leading-5 text-muted-foreground">
            <span className="shrink-0">
              {activeEngine === "codex" ? "Codex 对话" : "Local 对话"}
            </span>
            {workspaceLabel ? (
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
            ) : null}
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
        ) : headerActions ? (
          <header className="flex h-10 items-center justify-end border-b border-border px-4">
            <div className="flex items-center gap-2">{headerActions}</div>
          </header>
        ) : null}
        {sessionId ? (
          activeEngine === "codex" ? (
            <>
              {composerError ? (
                <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-red-300">
                  {composerError}
                </div>
              ) : null}
              <CodexPane
                sessionId={sessionId}
                workspace={frozenWorkspace}
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
          )
        ) : (
          <>
            {composerError ? (
              <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-red-300">
                {composerError}
              </div>
            ) : null}
            <NewConversation
              brandSrc={brandSrc}
              codexIconSrc={codexIconSrc}
              engine={draftEngine}
              onEngine={(next) => {
                setDraftEngine(next);
                setComposerError(null);
              }}
              workspaceLabel={workspaceLabel}
              workspaceTitle={workspaceTitle}
              picking={pickingWorkspace}
              canPick={Boolean(pickDirectory)}
              onPick={pickWorkspace}
              canClear={Boolean(workspaceDraft.trim())}
              onClear={() => {
                setWorkspaceDraft("");
                clearLastWorkspace();
              }}
            >
              {draftEngine === "codex" ? (
                <CodexPane
                  composeOnly
                  workspace={workspaceDraft}
                  pickFiles={pickFiles}
                  onOpenSession={(id) => onOpenSession(id, "codex")}
                  onNewConversation={onNewConversation}
                  onListChange={codexList.refresh}
                />
              ) : (
                <PromptBar
                  className="mx-0 max-w-none px-0 pb-0"
                  running={false}
                  sending={starting}
                  onSend={onSend}
                  onCancel={async () => undefined}
                />
              )}
            </NewConversation>
          </>
        )}
      </main>
    </div>
  );
}

// mergeSessions 把 Agent 与 Codex 会话按更新时间合成侧栏列表，同引擎同 ID 只留更新的一条。
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

// asSidebarSession 把 Codex 会话收成侧栏条目，目录用 cwd，标题优先 title。
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

// stampToIso 把秒或毫秒时间戳收成 ISO 字符串，无效值用纪元。
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

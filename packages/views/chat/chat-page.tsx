"use client";

import type { ApprovalMode, Session, TimelineItem, WorkMode } from "@codedock/core/chat";
import type { ClaudeSession } from "@codedock/core/claude";
import type { Session as CodexSession } from "@codedock/core/codex";
import { Button } from "@codedock/ui";
import { PanelLeft, PanelLeftClose, PanelRight, PanelRightClose, PlusIcon } from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";

import { ClaudePane } from "../claude/claude-pane.tsx";
import { useClaudeSessionList } from "../claude/hooks/use-session-list.ts";
import { useClaude } from "../claude/provider.tsx";
import { CodexPane } from "../codex/codex-pane.tsx";
import { useCodexSessionList } from "../codex/hooks/use-session-list.ts";
import { useCodex } from "../codex/provider.tsx";
import { useAgent } from "../provider.tsx";
import { ApprovalDock } from "./approval-dock.tsx";
import { useColumnLayout } from "./column-layout.ts";
import { ColumnSash } from "./column-sash.tsx";
import { ConversationTimeline } from "./conversation-timeline.tsx";
import { SideDock } from "./side-dock.tsx";
import { collectDockArtifacts, fileWindowId, planWindowId, useWorkbench } from "./workbench.ts";
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

export type SessionEngine = "agent" | "codex" | "claude";

export type ChatPageProps = {
  sessionId?: string;
  engine?: SessionEngine;
  onOpenSession: (id: string, engine?: SessionEngine) => void;
  onNewConversation: () => void;
  brandSrc?: string;
  codexIconSrc?: string;
  claudeIconSrc?: string;
  headerActions?: ReactNode;
};

// ChatPage 组合会话列表、对话和右侧多窗口栏。
export function ChatPage({
  sessionId,
  engine,
  onOpenSession,
  onNewConversation,
  brandSrc,
  codexIconSrc,
  claudeIconSrc,
  headerActions,
}: ChatPageProps) {
  const { client, pickDirectory, pickFiles } = useAgent();
  const { client: codexClient } = useCodex();
  const { client: claudeClient } = useClaude();
  const list = useSessionList();
  const codexList = useCodexSessionList();
  const claudeList = useClaudeSessionList();
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
  const workbench = useWorkbench();
  const columns = useColumnLayout();

  useEffect(() => {
    workbench.reset();
  }, [sessionId, activeEngine, workbench.reset]);

  // 上次目录只在本机 localStorage，等 hydration 后再读，避免 SSR 文本对不上。
  useEffect(() => {
    setWorkspaceDraft(readLastWorkspace());
  }, []);

  const sessions = useMemo(
    () => mergeSessions(list.sessions, codexList.sessions, claudeList.sessions),
    [claudeList.sessions, codexList.sessions, list.sessions],
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

  const artifacts = useMemo(
    () => collectDockArtifacts(timeline.state.items),
    [timeline.state.items],
  );

  useEffect(() => {
    for (const plan of artifacts.plans) {
      workbench.refreshOpen({
        id: planWindowId(plan.name),
        kind: "plan",
        title: plan.name || "Plan",
        name: plan.name,
        content: plan.content,
        toolState: plan.toolState,
        error: plan.error,
      });
    }
    for (const file of artifacts.files) {
      workbench.refreshOpen({
        id: fileWindowId(file.path),
        kind: "file",
        title: file.path.split("/").pop() || file.path,
        path: file.path,
        content: file.content,
        action: file.action,
      });
    }
  }, [artifacts, workbench.refreshOpen]);

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

  // hideSession 按引擎归档或删除，当前打开的那条会回到新建页。
  const hideSession = async (session: SidebarSession) => {
    const hiddenId = session.id;
    const hiddenEngine = session.engine ?? "agent";
    try {
      if (session.engine === "codex") {
        await codexClient.archiveSession(session.id);
        await codexList.refresh();
      } else if (session.engine === "claude") {
        await claudeClient.archiveSession(session.id);
        await claudeList.refresh();
      } else {
        await list.removeSession(session);
      }
    } catch (err) {
      setComposerError(err instanceof Error ? err.message : "归档失败");
      throw err;
    }
    if (sessionId === hiddenId && (engine ?? "agent") === hiddenEngine) {
      onNewConversation();
    }
  };

  // openPlan 打开计划窗口；右侧若收起则先展开。
  const openPlan = (
    preview: Parameters<typeof workbench.openPlan>[0],
    extra?: Parameters<typeof workbench.openPlan>[1],
  ) => {
    columns.setRightOpen(true);
    workbench.openPlan(preview, extra);
  };
  // openFile 打开文件窗口；右侧若收起则先展开。
  const openFile = (change: Parameters<typeof workbench.openFile>[0]) => {
    columns.setRightOpen(true);
    workbench.openFile(change);
  };
  // createDock 从窗口栏新建；右侧若收起则先展开。
  const createDock = (
    kind: Parameters<typeof workbench.createKind>[0],
    seed?: Parameters<typeof workbench.createKind>[1],
  ) => {
    columns.setRightOpen(true);
    workbench.createKind(kind, seed);
  };

  return (
    <div ref={columns.rowRef} className="flex h-full overflow-hidden bg-background text-foreground">
      {columns.leftOpen ? (
        <>
          <SessionSidebar
            width={columns.left}
            sessions={sessions}
            currentId={currentKey}
            busy={list.busy || codexList.busy || claudeList.busy}
            error={
              activeEngine === "codex"
                ? codexList.error
                : activeEngine === "claude"
                  ? claudeList.error
                  : list.error
            }
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
            claudeIconSrc={claudeIconSrc}
          />
          <ColumnSash
            label="调整会话列表宽度"
            onMove={columns.moveLeft}
            onCollapse={() => columns.setLeftOpen(false)}
          />
        </>
      ) : null}
      <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-2 text-sm leading-5 text-muted-foreground">
          <SidebarToggle
            label={columns.leftOpen ? "收起会话列表" : "展开会话列表"}
            onClick={columns.toggleLeft}
          >
            {columns.leftOpen ? <PanelLeftClose className="size-3.5" /> : <PanelLeft className="size-3.5" />}
          </SidebarToggle>
          {!columns.leftOpen ? (
            <Button size="sm" variant="secondary" onClick={onNewConversation}>
              <PlusIcon className="size-3.5" />
              新对话
            </Button>
          ) : null}
          {sessionId ? (
            <>
              <span className="shrink-0">
                {activeEngine === "codex"
                  ? "Codex 对话"
                  : activeEngine === "claude"
                    ? "Claude 对话"
                    : "Local 对话"}
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
            </>
          ) : null}
          <div className="ml-auto flex items-center gap-2">
            {headerActions}
            <SidebarToggle
              label={columns.rightOpen ? "收起右侧窗口" : "展开右侧窗口"}
              onClick={columns.toggleRight}
            >
              {columns.rightOpen ? <PanelRightClose className="size-3.5" /> : <PanelRight className="size-3.5" />}
            </SidebarToggle>
          </div>
        </header>
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
          ) : activeEngine === "claude" ? (
            <>
              {composerError ? (
                <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-red-300">
                  {composerError}
                </div>
              ) : null}
              <ClaudePane
                sessionId={sessionId}
                workspace={frozenWorkspace || workspaceDraft}
                pickFiles={pickFiles}
                onOpenSession={(id) => onOpenSession(id, "claude")}
                onNewConversation={onNewConversation}
                onListChange={claudeList.refresh}
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
                onOpenPlan={openPlan}
                onOpenFile={openFile}
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
              claudeIconSrc={claudeIconSrc}
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
              ) : draftEngine === "claude" ? (
                <ClaudePane
                  composeOnly
                  workspace={workspaceDraft}
                  pickFiles={pickFiles}
                  onOpenSession={(id) => onOpenSession(id, "claude")}
                  onNewConversation={onNewConversation}
                  onListChange={claudeList.refresh}
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
      {columns.rightOpen ? (
        <>
          <ColumnSash
            label="调整右侧窗口宽度"
            onMove={(delta, persist) => columns.moveRight(-delta, persist)}
            onCollapse={() => columns.setRightOpen(false)}
          />
          <SideDock
            width={columns.right}
            windows={workbench.windows}
            activeId={workbench.activeId}
            plans={artifacts.plans}
            files={artifacts.files}
            onSelect={workbench.setActiveId}
            onClose={workbench.closeWindow}
            onCreate={createDock}
          />
        </>
      ) : null}
    </div>
  );
}

// SidebarToggle 收起或展开一侧栏。
function SidebarToggle({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Button size="sm" variant="ghost" className="px-1.5" title={label} aria-label={label} onClick={onClick}>
      {children}
    </Button>
  );
}

// mergeSessions 把 Local / Codex / Claude 会话按更新时间合成侧栏列表，去掉已归档，同引擎同 ID 只留更新的一条。
function mergeSessions(
  agent: Session[],
  codex: CodexSession[],
  claude: ClaudeSession[],
): SidebarSession[] {
  const mapped: SidebarSession[] = [
    ...agent.map((session) => ({ ...session, engine: "agent" as const })),
    ...codex.map(asCodexSidebarSession),
    ...claude.map(asClaudeSidebarSession),
  ];
  const seen = new Map<string, SidebarSession>();
  for (const session of mapped) {
    const key = `${session.engine ?? "agent"}:${session.id}`;
    const prev = seen.get(key);
    if (!prev || prev.updated_at < session.updated_at) {
      seen.set(key, session);
    }
  }
  return [...seen.values()]
    .filter((session) => session.status !== "archived")
    .sort((left, right) => (left.updated_at < right.updated_at ? 1 : -1));
}

// asCodexSidebarSession 把 Codex 会话收成侧栏条目，目录用 cwd，标题优先 title。
function asCodexSidebarSession(session: CodexSession): SidebarSession {
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

// asClaudeSidebarSession 把 Claude 会话收成侧栏条目；时间用本机实录的 Unix 秒。
function asClaudeSidebarSession(session: ClaudeSession): SidebarSession {
  return {
    id: session.id,
    tenant_id: "",
    user_id: "",
    agent_id: "claude",
    workspace_id: "",
    status: session.archived ? "archived" : "active",
    last_event_seq: 1,
    compaction_seq: 0,
    summary: session.title || session.claude_session_id,
    created_at: stampToIso(session.created_at),
    updated_at: stampToIso(session.updated_at),
    engine: "claude",
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

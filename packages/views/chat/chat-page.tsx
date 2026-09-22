"use client";

import type { Placement, Work } from "@codedock/core/board";
import type { ApprovalMode, Session, TimelineItem, WorkMode } from "@codedock/core/chat";
import type { ClaudeSession } from "@codedock/core/claude";
import type { Session as CodexSession } from "@codedock/core/codex";
import { Button, cn } from "@codedock/ui";
import { PanelLeft, PanelLeftClose, PanelRight, PanelRightClose, PlusIcon, Settings } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { BoardGrid } from "../board/board-grid.tsx";
import { workTitleError } from "../board/work-column.tsx";
import { groupSessionsByWork } from "../board/group.ts";
import { useBoard } from "../board/provider.tsx";
import { ComposeFloat, SessionFloat, type ComposeDraft, type FloatSession } from "../board/session-float.tsx";
import { SessionLinkEditor } from "../board/session-links.tsx";
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
import { collectDockArtifacts, planWindowId, useWorkbench } from "./workbench.ts";
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

const BOARD_DOT = 8;
const BOARD_COVER_MS = 480;
const BOARD_REVEAL_MS = 260;

type BoardCover = {
  x: number;
  y: number;
  /** 盖住视口四角所需的放大倍数。 */
  scale: number;
  phase: "grow" | "covered" | "reveal";
  /** 黑点铺满后要去的那一态。 */
  to: "board" | "chat";
};

// boardCoverScale 算出黑点要放大多少倍才能盖住视口四角。
function boardCoverScale(x: number, y: number) {
  const dx = Math.max(x, window.innerWidth - x);
  const dy = Math.max(y, window.innerHeight - y);
  return (Math.hypot(dx, dy) / (BOARD_DOT / 2)) * 1.08;
}

export type ChatPageProps = {
  sessionId?: string;
  engine?: SessionEngine;
  boardMode?: boolean;
  onOpenSession: (id: string, engine?: SessionEngine) => void;
  onNewConversation: () => void;
  onOpenBoard?: () => void;
  onLeaveBoard?: () => void;
  brandSrc?: string;
  codexIconSrc?: string;
  claudeIconSrc?: string;
  headerActions?: ReactNode;
};

// ChatPage 组两态：会话三栏，看板藏中间对话、右侧仍是 Plan / 文件 / Git。左上角 logo 和切换按钮两边都在，看板时旁边可一键新建分组。来回切换都从按钮长出黑点盖住屏幕，再褪开露出另一态。
export function ChatPage({
  sessionId,
  engine,
  boardMode = false,
  onOpenSession,
  onNewConversation,
  onOpenBoard,
  onLeaveBoard,
  brandSrc,
  codexIconSrc,
  claudeIconSrc,
  headerActions,
}: ChatPageProps) {
  const { client, pickDirectory, pickFiles } = useAgent();
  const { client: boardClient } = useBoard();
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
  const [works, setWorks] = useState<Work[]>([]);
  const [placements, setPlacements] = useState<Placement[]>([]);
  const [float, setFloat] = useState<FloatSession | null>(null);
  const [floatLinksOpen, setFloatLinksOpen] = useState(false);
  const [compose, setCompose] = useState<ComposeDraft | null>(null);
  const [boardRevision, setBoardRevision] = useState(0);
  const [creatingGroup, setCreatingGroup] = useState(false);
  const [groupError, setGroupError] = useState<string | null>(null);
  const [pendingWorkId, setPendingWorkId] = useState<string | null>(null);
  const [linksOpen, setLinksOpen] = useState(false);
  const [draftLinks, setDraftLinks] = useState<string[]>([]);
  const draftLinksRef = useRef(draftLinks);
  draftLinksRef.current = draftLinks;
  const [cover, setCover] = useState<BoardCover | null>(null);
  const coverRef = useRef<HTMLDivElement>(null);
  const openBoardRef = useRef(onOpenBoard);
  const leaveBoardRef = useRef(onLeaveBoard);
  const newConversationRef = useRef(onNewConversation);
  openBoardRef.current = onOpenBoard;
  leaveBoardRef.current = onLeaveBoard;
  newConversationRef.current = onNewConversation;
  const wasBoard = useRef(boardMode);

  // openBoardFromSidebar 从按钮中心铺开黑点，铺满后再换到另一态。减动效时直接切换。
  const openBoardFromSidebar = (button: HTMLButtonElement) => {
    if (cover) {
      return;
    }
    const to = boardMode ? "chat" : "board";
    if (to === "board" && !onOpenBoard) {
      return;
    }
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduced) {
      if (to === "chat") {
        if (onLeaveBoard) {
          onLeaveBoard();
        } else {
          onNewConversation();
        }
      } else {
        onOpenBoard?.();
      }
      return;
    }
    const rect = button.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    setCover({ x, y, scale: boardCoverScale(x, y), phase: "grow", to });
  };

  // 黑点铺满后才换页，盖层先留着，避免切页时露出另一态。
  useEffect(() => {
    const node = coverRef.current;
    if (!node || cover?.phase !== "grow") {
      return;
    }
    let settled = false;
    const finish = () => {
      if (settled) {
        return;
      }
      settled = true;
      const to = cover.to;
      setCover((current) => (current?.phase === "grow" ? { ...current, phase: "covered" } : current));
      if (to === "chat") {
        if (leaveBoardRef.current) {
          leaveBoardRef.current();
        } else {
          newConversationRef.current();
        }
      } else {
        openBoardRef.current?.();
      }
    };
    const anim = node.animate(
      [
        { transform: "translate(-50%, -50%) scale(1)" },
        { transform: `translate(-50%, -50%) scale(${cover.scale})` },
      ],
      { duration: BOARD_COVER_MS, easing: "cubic-bezier(0.7, 0, 0.15, 1)", fill: "forwards" },
    );
    anim.onfinish = finish;
    const timer = window.setTimeout(finish, BOARD_COVER_MS + 40);
    return () => {
      settled = true;
      anim.onfinish = null;
      window.clearTimeout(timer);
      anim.cancel();
    };
  }, [cover?.phase, cover?.scale, cover?.to]);

  // 目标那一态挂上后再褪开黑点。
  useEffect(() => {
    if (!cover || cover.phase !== "covered") {
      return;
    }
    const arrived = cover.to === "board" ? boardMode : !boardMode;
    if (!arrived) {
      return;
    }
    const frame = requestAnimationFrame(() => {
      setCover((current) => (current?.phase === "covered" ? { ...current, phase: "reveal" } : current));
    });
    return () => cancelAnimationFrame(frame);
  }, [boardMode, cover]);

  // 黑点透明度走完后卸掉盖层。
  useEffect(() => {
    const node = coverRef.current;
    if (!node || cover?.phase !== "reveal") {
      return;
    }
    let settled = false;
    const finish = () => {
      if (settled) {
        return;
      }
      settled = true;
      setCover(null);
    };
    const anim = node.animate(
      [{ opacity: 1 }, { opacity: 0 }],
      { duration: BOARD_REVEAL_MS, easing: "ease-out", fill: "forwards" },
    );
    anim.onfinish = finish;
    const timer = window.setTimeout(finish, BOARD_REVEAL_MS + 40);
    return () => {
      settled = true;
      anim.onfinish = null;
      window.clearTimeout(timer);
      anim.cancel();
    };
  }, [cover?.phase]);

  // createGroup 不填标题，由服务端取最小的未命名序号，然后重拉看板。
  const createGroup = () => {
    setCreatingGroup(true);
    setGroupError(null);
    void boardClient
      .createWork("")
      .then(() => boardClient.listWorks())
      .then((nextWorks) => {
        setWorks(nextWorks);
        setBoardRevision((value) => value + 1);
      })
      .catch((err: unknown) => {
        setGroupError(workTitleError(err));
      })
      .finally(() => {
        setCreatingGroup(false);
      });
  };

  // 看板里会新建、归组、绑目录；回到会话时重拉左侧列表，避免还显示离开前的分组。
  useEffect(() => {
    const leaving = wasBoard.current && !boardMode;
    wasBoard.current = boardMode;
    if (!leaving) {
      return;
    }
    void list.refresh();
    void codexList.refresh();
    void claudeList.refresh();
    void Promise.all([boardClient.listWorks(), boardClient.listPlacements()])
      .then(([nextWorks, nextPlaces]) => {
        setWorks(nextWorks);
        setPlacements(nextPlaces);
      })
      .catch(() => undefined);
  }, [boardClient, boardMode, claudeList.refresh, codexList.refresh, list.refresh]);

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
  const sidebarSessions = useMemo(() => {
    if (activeEngine !== "agent" || !sessionId) {
      return sessions;
    }
    return sessions.map((session) => {
      if (session.engine !== "agent" || session.id !== sessionId) {
        return session;
      }
      return { ...session, running: timeline.loading ? session.running : timeline.running };
    });
  }, [activeEngine, sessionId, sessions, timeline.running]);
  const someoneRunning = sidebarSessions.some((session) => session.running);
  useEffect(() => {
    if (boardMode || !someoneRunning) {
      return;
    }
    const timer = window.setInterval(() => {
      void list.refresh();
      void codexList.refresh();
      void claudeList.refresh();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [boardMode, claudeList.refresh, codexList.refresh, list.refresh, someoneRunning]);
  useEffect(() => {
    void Promise.all([boardClient.listWorks(), boardClient.listPlacements()])
      .then(([nextWorks, nextPlaces]) => {
        setWorks(nextWorks);
        setPlacements(nextPlaces);
      })
      .catch(() => undefined);
  }, [boardClient, sessions]);
  const groups = useMemo(
    () => groupSessionsByWork(sidebarSessions, placements, works),
    [placements, sidebarSessions, works],
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
    workbench.syncFiles(artifacts.files, false);
  }, [artifacts, workbench.refreshOpen, workbench.syncFiles]);

  const pendingApprovals = timeline.state.items.filter(
    (item): item is Extract<TimelineItem, { kind: "approval" }> =>
      item.kind === "approval" && item.status === "pending",
  );

  // commitDraftLinks 还没有会话时，保存非空 Issue/PR 就建会话并打开，不必先发消息。
  const commitDraftLinks = async (links: string[]) => {
    draftLinksRef.current = links;
    setDraftLinks(links);
    if (sessionId || links.length === 0) {
      return;
    }
    setComposerError(null);
    setStarting(true);
    let createdId = "";
    try {
      if (activeEngine === "codex") {
        const session = await codexClient.createSession(workspaceDraft.trim() ? { cwd: workspaceDraft.trim() } : {});
        createdId = session.id;
      } else if (activeEngine === "claude") {
        const session = await claudeClient.createSession();
        createdId = session.id;
        if (workspaceDraft.trim()) {
          await claudeClient.applySettings(createdId, { cwd: workspaceDraft.trim() });
        }
      } else {
        const session = await list.createSession(workspaceDraft);
        createdId = session.id;
        writeLastWorkspace(session.workspace_id);
        setWorkspaceDraft(session.workspace_id);
      }
      if (workspaceDraft.trim()) {
        await boardClient.bindDirectory(activeEngine, createdId, workspaceDraft.trim());
      }
      await boardClient.replaceLinks(activeEngine, createdId, links);
      draftLinksRef.current = [];
      setDraftLinks([]);
      if (activeEngine === "codex") {
        await codexList.refresh();
      } else if (activeEngine === "claude") {
        await claudeList.refresh();
      } else {
        await list.refresh();
      }
      try {
        await placePending(createdId, activeEngine);
      } catch (err) {
        setComposerError(createSessionError(err, "会话已创建，但没能放到分组"));
      }
      onOpenSession(createdId, activeEngine);
    } catch (err) {
      if (createdId) {
        try {
          if (activeEngine === "codex") {
            await codexClient.archiveSession(createdId);
          } else if (activeEngine === "claude") {
            await claudeClient.archiveSession(createdId);
          } else {
            await client.archiveSession(createdId);
          }
        } catch {
          // 链接没挂上时尽量清掉空会话
        }
      }
      const message = createSessionError(err, "无法创建会话");
      setComposerError(message);
      throw new Error(message);
    } finally {
      setStarting(false);
    }
  };

  // attachDraftLinks 把对话开始前记下的 Issue/PR 挂到刚建好的会话上。
  const attachDraftLinks = async (id: string, engine: SessionEngine) => {
    const links = draftLinksRef.current.map((row) => row.trim()).filter(Boolean);
    if (links.length === 0) {
      return;
    }
    await boardClient.replaceLinks(engine, id, links);
    draftLinksRef.current = [];
    setDraftLinks([]);
  };

  const onSend = async (text: string, mode: WorkMode, approval: ApprovalMode) => {
    setComposerError(null);
    if (sessionId) {
      await timeline.send(text, mode, approval);
      await list.refresh();
      return;
    }
    setStarting(true);
    try {
      const chosenDirectory = workspaceDraft.trim();
      const session = await list.createSession(workspaceDraft);
      writeLastWorkspace(session.workspace_id);
      setWorkspaceDraft(session.workspace_id);
      if (chosenDirectory) {
        await boardClient.bindDirectory("agent", session.id, chosenDirectory);
      }
      try {
        await attachDraftLinks(session.id, "agent");
      } catch (err) {
        setComposerError(createSessionError(err, "Issue 或 PR 没挂上"));
      }
      await client.startRun(session.id, { content: text, mode, approval });
      await list.refresh();
      try {
        await placePending(session.id, "agent");
      } catch (err) {
        setComposerError(createSessionError(err, "会话已发出，但没能放到分组"));
      }
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

  // placePending 把刚发出消息的会话挂到加号选中的 Work 上。没有待挂分组时什么都不做。
  const placePending = async (id: string, engine: SessionEngine) => {
    const workId = pendingWorkId;
    if (!workId) {
      return;
    }
    await boardClient.attachPlacement(workId, engine, id);
    setPendingWorkId(null);
    setPlacements(await boardClient.listPlacements());
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
  // openGit 打开 Git 窗口；右侧若收起则先展开。
  const openGit = () => {
    columns.setRightOpen(true);
    workbench.openGit();
  };
  // createDock 从窗口栏新建；右侧若收起则先展开。
  const createDock = (
    kind: Parameters<typeof workbench.createKind>[0],
    seed?: Parameters<typeof workbench.createKind>[1],
  ) => {
    columns.setRightOpen(true);
    workbench.createKind(kind, seed);
  };

  const rightDock = columns.rightOpen ? (
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
  ) : null;

  const boardCover = cover ? (
    <div className="fixed inset-0 z-[80]" aria-hidden>
      <div
        ref={coverRef}
        className="absolute rounded-full bg-black"
        style={{
          left: cover.x,
          top: cover.y,
          width: BOARD_DOT,
          height: BOARD_DOT,
          transform:
            cover.phase === "grow"
              ? "translate(-50%, -50%) scale(1)"
              : `translate(-50%, -50%) scale(${cover.scale})`,
        }}
      />
    </div>
  ) : null;

  const sessionSidebar = (
    <SessionSidebar
      boardOpen={boardMode}
      width={columns.left}
      sessions={sidebarSessions}
      groups={groups}
      works={works.map((work) => ({ id: work.id, title: work.title }))}
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
      onCreate={() => {
        setPendingWorkId(null);
        onNewConversation();
      }}
      pendingWorkId={pendingWorkId}
      onCreateInWork={(workId) => {
        setPendingWorkId(workId);
        onNewConversation();
      }}
      onOpenBoard={onOpenBoard ? openBoardFromSidebar : undefined}
      onCreateGroup={boardMode ? createGroup : undefined}
      creatingGroup={creatingGroup}
      onAttach={async (session, workId) => {
        await boardClient.attachPlacement(workId, session.engine ?? "agent", session.id);
        const [nextWorks, nextPlaces] = await Promise.all([
          boardClient.listWorks(),
          boardClient.listPlacements(),
        ]);
        setWorks(nextWorks);
        setPlacements(nextPlaces);
      }}
      onSelect={(id, nextEngine) => {
        setPendingWorkId(null);
        onOpenSession(id, nextEngine ?? "agent");
      }}
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
  );

  if (boardMode) {
    return (
      <div ref={columns.rowRef} className="flex h-full overflow-hidden bg-background text-foreground">
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <div className="flex shrink-0 items-center border-b border-border text-sm leading-5 text-muted-foreground">
            {sessionSidebar}
            <div className="ml-auto flex h-11 items-center gap-2 pr-2">
              {headerActions}
              <SidebarToggle
                label={columns.rightOpen ? "收起右侧窗口" : "展开右侧窗口"}
                onClick={columns.toggleRight}
              >
                {columns.rightOpen ? <PanelRightClose className="size-3.5" /> : <PanelRight className="size-3.5" />}
              </SidebarToggle>
            </div>
          </div>
          <BoardGrid
            revision={boardRevision}
            notice={groupError}
            onOpenSession={(id, nextEngine) => {
              setCompose(null);
              setFloatLinksOpen(false);
              setFloat({ id, engine: nextEngine });
            }}
            onDraftSession={(workId, nextEngine, directory, title) => {
              setFloat(null);
              setCompose({ workId, engine: nextEngine, directory, title });
            }}
            onArchived={(id, nextEngine) => {
              if (float?.id === id && float.engine === nextEngine) {
                setFloat(null);
              }
            }}
          />
        </div>
        <div className="flex h-full shrink-0">{rightDock}</div>
        {boardCover}
        {compose ? (
          <ComposeFloat
            draft={compose}
            onClose={() => setCompose(null)}
            onSent={(id, nextEngine, source) => {
              setCompose(null);
              setFloatLinksOpen(source === "links");
              setFloat({ id, engine: nextEngine });
              setBoardRevision((value) => value + 1);
              void boardClient.listPlacements().then(setPlacements).catch(() => undefined);
            }}
          />
        ) : float ? (
          <SessionFloat
            key={`${float.engine}:${float.id}`}
            initialLinksOpen={floatLinksOpen}
            session={float}
            onClose={() => setFloat(null)}
            onOpenPlan={openPlan}
            onOpenFile={openFile}
            onOpenGit={openGit}
          />
        ) : null}
      </div>
    );
  }

  return (
    <div ref={columns.rowRef} className="flex h-full overflow-hidden bg-background text-foreground">
      {columns.leftOpen ? (
        <>
          {sessionSidebar}
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
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                setPendingWorkId(null);
                onNewConversation();
              }}
            >
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
                  data-testid="run-recover"
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
            <Button
              size="sm"
              variant="ghost"
              className={cn("px-1.5", linksOpen && "bg-accent text-accent-foreground")}
              title="设置"
              aria-label="设置"
              aria-pressed={linksOpen}
              onClick={() => setLinksOpen((open) => !open)}
            >
              <Settings className="size-3.5" />
            </Button>
            <SidebarToggle
              label={columns.rightOpen ? "收起右侧窗口" : "展开右侧窗口"}
              onClick={columns.toggleRight}
            >
              {columns.rightOpen ? <PanelRightClose className="size-3.5" /> : <PanelRight className="size-3.5" />}
            </SidebarToggle>
          </div>
        </header>
        {linksOpen ? (
          <SessionLinkEditor
            engine={activeEngine}
            sessionId={sessionId}
            draft={draftLinks}
            onDraft={commitDraftLinks}
          />
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
                onOpenGit={openGit}
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
              placedIn={works.find((work) => work.id === pendingWorkId)?.title}
            >
              {draftEngine === "codex" ? (
                <CodexPane
                  composeOnly
                  workspace={workspaceDraft}
                  pickFiles={pickFiles}
                  onOpenSession={(id) => onOpenSession(id, "codex")}
                  onPrepare={(id) => attachDraftLinks(id, "codex")}
                  onCreated={async (id) => {
                    try {
                      if (workspaceDraft.trim()) {
                        await boardClient.bindDirectory("codex", id, workspaceDraft.trim());
                      }
                      await placePending(id, "codex");
                    } catch (err) {
                      setComposerError(createSessionError(err, "会话已发出，但没能放到分组"));
                    }
                  }}
                  onNewConversation={onNewConversation}
                  onListChange={codexList.refresh}
                />
              ) : draftEngine === "claude" ? (
                <ClaudePane
                  composeOnly
                  workspace={workspaceDraft}
                  pickFiles={pickFiles}
                  onOpenSession={(id) => onOpenSession(id, "claude")}
                  onPrepare={(id) => attachDraftLinks(id, "claude")}
                  onCreated={async (id) => {
                    try {
                      if (workspaceDraft.trim()) {
                        await boardClient.bindDirectory("claude", id, workspaceDraft.trim());
                      }
                      await placePending(id, "claude");
                    } catch (err) {
                      setComposerError(createSessionError(err, "会话已发出，但没能放到分组"));
                    }
                  }}
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
      <div className="flex h-full shrink-0">{rightDock}</div>
      {boardCover}
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
    ...agent.map((session) => ({
      ...session,
      engine: "agent" as const,
      running: Boolean(session.active_run_id) && !session.needs_recover,
    })),
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
    running: Boolean(session.active_turn_id),
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
    running: Boolean(session.active_turn_id),
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

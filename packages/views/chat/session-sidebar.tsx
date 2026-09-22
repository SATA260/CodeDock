"use client";

import type { Session } from "@codedock/core/chat";
import { Button, cn } from "@codedock/ui";
import { Archive, ChevronRight, FolderKanban, LayoutGrid, PlusIcon } from "lucide-react";
import { useState } from "react";

import type { WorkGroup } from "../board/group.ts";
import type { SessionEngine } from "./chat-page.tsx";
import { compactAge, sessionTitle, sessionTitleParts } from "./lib/format.ts";

export type SidebarSession = Session & {
  engine?: SessionEngine;
  /** 这路会话正在跑，侧栏用扫光标出来。 */
  running?: boolean;
  /** 卡在待审批。标题用琥珀呼吸，不算进行中。 */
  awaiting?: boolean;
};

// SessionSidebar 按 Work 分组、组内按时间；未挂卡的进未归组。
export function SessionSidebar({
  sessions,
  groups,
  works = [],
  currentId,
  busy,
  error,
  hasMore = false,
  onLoadMore,
  onCreate,
  onCreateInWork,
  pendingWorkId,
  onSelect,
  onRecover,
  onArchive,
  onOpenBoard,
  onCreateGroup,
  creatingGroup = false,
  boardOpen = false,
  onAttach,
  canRecoverCurrent = false,
  brandSrc,
  codexIconSrc,
  claudeIconSrc,
  width,
}: {
  sessions: SidebarSession[];
  groups?: WorkGroup<SidebarSession>[];
  works?: { id: string; title: string }[];
  currentId?: string;
  busy: boolean;
  error: string | null;
  hasMore?: boolean;
  onLoadMore?: () => void;
  onCreate: () => void;
  /** 记下要挂上的 Work，等用户发出第一条消息再开会话。未分组没有这个入口。 */
  onCreateInWork?: (workId: string) => void;
  /** 下一条新对话要挂上的 Work。 */
  pendingWorkId?: string | null;
  onSelect: (id: string, engine?: SessionEngine) => void;
  onRecover?: (runId: string) => Promise<void>;
  onArchive?: (session: SidebarSession) => Promise<void>;
  /** 点开看板。origin 是按钮本身，黑点从它的中心铺开。看板已开时再点一次回到会话列表。 */
  onOpenBoard?: (origin: HTMLButtonElement) => void;
  /** 看板已开时，在切换按钮旁一键建分组，标题由服务端取空号。 */
  onCreateGroup?: () => void;
  creatingGroup?: boolean;
  /** 看板态只留这排顶栏，logo 和切换按钮停在会话列表时的位置。 */
  boardOpen?: boolean;
  onAttach?: (session: SidebarSession, workId: string) => Promise<void>;
  canRecoverCurrent?: boolean;
  brandSrc?: string;
  codexIconSrc?: string;
  claudeIconSrc?: string;
  width?: number;
}) {
  const rendered = groups ?? [{ id: null, title: "未分组", updatedAt: "", sessions }];
  const boardLabel = boardOpen ? "返回会话" : "打开看板";
  return (
    <aside
      className={cn("flex shrink-0 flex-col bg-background", boardOpen ? "h-11" : "h-full")}
      style={
        width
          ? boardOpen
            ? { minWidth: width }
            : { width, minWidth: width, maxWidth: width }
          : { width: "15rem" }
      }
    >
      <div className="flex h-11 shrink-0 items-center gap-3 px-3">
        <div className="flex min-w-0 items-center gap-2">
          {brandSrc ? (
            <img src={brandSrc} alt="" className="size-6 shrink-0" />
          ) : null}
          <div className="truncate text-sm font-semibold tracking-tight">CodeDock</div>
        </div>
        {onOpenBoard ? (
          <Button
            size="sm"
            variant="ghost"
            className={cn("px-1.5", boardOpen && "bg-accent text-accent-foreground")}
            title={boardLabel}
            aria-label={boardLabel}
            aria-pressed={boardOpen}
            onClick={(event) => onOpenBoard?.(event.currentTarget)}
          >
            <LayoutGrid className="size-3.5" />
          </Button>
        ) : null}
        {boardOpen && onCreateGroup ? (
          <Button size="sm" variant="secondary" className="shrink-0" disabled={creatingGroup} onClick={onCreateGroup}>
            <PlusIcon className="size-3.5" />
            新建分组
          </Button>
        ) : null}
        {boardOpen ? null : (
          <Button className="ml-auto" size="sm" variant="secondary" disabled={busy} onClick={onCreate}>
            <PlusIcon className="size-3.5" />
            新对话
          </Button>
        )}
      </div>
      {boardOpen ? null : (
        <>
          {error ? <p className="px-3 pb-2 text-xs text-destructive">{error}</p> : null}
          <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
            {rendered.every((group) => group.id === null && group.sessions.length === 0) ? (
              <p className="px-2 py-6 text-xs text-muted-foreground">还没有会话</p>
            ) : (
              rendered.map((group) => (
                <WorkGroupSection
                  key={group.id ?? "ungrouped"}
                  group={group}
                  currentId={currentId}
                  works={works}
                  busy={busy}
                  onSelect={onSelect}
                  onRecover={onRecover}
                  onArchive={onArchive}
                  onAttach={onAttach}
                  onCreateInWork={onCreateInWork}
                  pendingWorkId={pendingWorkId}
                  canRecoverCurrent={canRecoverCurrent}
                  brandSrc={brandSrc}
                  codexIconSrc={codexIconSrc}
                  claudeIconSrc={claudeIconSrc}
                />
              ))
            )}
            {hasMore && onLoadMore ? (
              <Button className="mt-2 w-full" size="sm" variant="ghost" onClick={onLoadMore}>
                加载更多
              </Button>
            ) : null}
          </nav>
        </>
      )}
    </aside>
  );
}

// WorkGroupSection 一组 Work：标题可收起下面的会话，组内已按时间从新到旧排好。
function WorkGroupSection({
  group,
  currentId,
  works,
  busy,
  onSelect,
  onRecover,
  onArchive,
  onAttach,
  onCreateInWork,
  pendingWorkId,
  canRecoverCurrent,
  brandSrc,
  codexIconSrc,
  claudeIconSrc,
}: {
  group: WorkGroup<SidebarSession>;
  currentId?: string;
  works: { id: string; title: string }[];
  busy: boolean;
  onSelect: (id: string, engine?: SessionEngine) => void;
  onRecover?: (runId: string) => Promise<void>;
  onArchive?: (session: SidebarSession) => Promise<void>;
  onAttach?: (session: SidebarSession, workId: string) => Promise<void>;
  onCreateInWork?: (workId: string) => void;
  pendingWorkId?: string | null;
  canRecoverCurrent: boolean;
  brandSrc?: string;
  codexIconSrc?: string;
  claudeIconSrc?: string;
}) {
  const [open, setOpen] = useState(true);
  const pending = Boolean(group.id && pendingWorkId === group.id);
  return (
    <section className="mb-3">
      <h2 className="flex items-center gap-1 pr-1">
        <button
          type="button"
          aria-expanded={open}
          className="flex min-w-0 flex-1 items-center gap-1 px-2 pb-1 text-left text-[11px] font-medium uppercase tracking-wide text-muted-foreground hover:text-foreground"
          onClick={() => setOpen((value) => !value)}
        >
          <ChevronRight className={cn("size-3 shrink-0 transition-transform", open && "rotate-90")} />
          {group.id ? <FolderKanban className="size-3 shrink-0" aria-hidden /> : null}
          <span className="min-w-0 truncate">{group.title}</span>
        </button>
        {group.id && onCreateInWork ? (
          <button
            type="button"
            title="创建会话"
            aria-label={`在${group.title}下创建会话`}
            aria-pressed={pending}
            className={cn(
              "inline-flex size-4 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-foreground",
              pending && "bg-accent text-foreground",
            )}
            onClick={() => {
              const workId = group.id;
              if (!workId) {
                return;
              }
              setOpen(true);
              onCreateInWork(workId);
            }}
          >
            <PlusIcon className="size-3" />
          </button>
        ) : null}
      </h2>
      {open ? (
        group.sessions.length === 0 ? (
          <p className="px-2 pb-2 text-[11px] text-muted-foreground/70">暂无会话</p>
        ) : (
          <ul className="space-y-0.5">
            {group.sessions.map((session) => (
              <SidebarRow
                key={`${session.engine ?? "agent"}:${session.id}`}
                session={session}
                currentId={currentId}
                ungrouped={group.id === null}
                works={works}
                busy={busy}
                onSelect={onSelect}
                onRecover={onRecover}
                onArchive={onArchive}
                onAttach={onAttach}
                canRecoverCurrent={canRecoverCurrent}
                brandSrc={brandSrc}
                codexIconSrc={codexIconSrc}
                claudeIconSrc={claudeIconSrc}
              />
            ))}
          </ul>
        )
      ) : null}
    </section>
  );
}

// SidebarRow 一条会话：点开、恢复、归档，未归组可补挂。
function SidebarRow({
  session,
  currentId,
  ungrouped,
  works,
  busy,
  onSelect,
  onRecover,
  onArchive,
  onAttach,
  canRecoverCurrent,
  brandSrc,
  codexIconSrc,
  claudeIconSrc,
}: {
  session: SidebarSession;
  currentId?: string;
  ungrouped: boolean;
  works: { id: string; title: string }[];
  busy: boolean;
  onSelect: (id: string, engine?: SessionEngine) => void;
  onRecover?: (runId: string) => Promise<void>;
  onArchive?: (session: SidebarSession) => Promise<void>;
  onAttach?: (session: SidebarSession, workId: string) => Promise<void>;
  canRecoverCurrent: boolean;
  brandSrc?: string;
  codexIconSrc?: string;
  claudeIconSrc?: string;
}) {
  const engine = session.engine ?? "agent";
  const rowKey = `${engine}:${session.id}`;
  const active = rowKey === currentId;
  return (
    <li>
      <div
        className={cn(
          "group flex h-7 w-full items-center gap-1 rounded-md px-1.5 transition-colors",
          active ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
        )}
      >
        <button
          type="button"
          onClick={() => onSelect(session.id, engine)}
          className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
        >
          <span className={cn("inline-flex shrink-0", session.running && !session.awaiting && "session-live-mark")}>
            <SessionEngineMark
              engine={engine}
              brandSrc={brandSrc}
              codexIconSrc={codexIconSrc}
              claudeIconSrc={claudeIconSrc}
            />
          </span>
          <SidebarSessionTitle
            id={session.id}
            summary={session.summary}
            running={session.running}
            awaiting={session.awaiting}
          />
        </button>
        {engine === "agent" &&
        session.needs_recover &&
        session.active_run_id &&
        onRecover &&
        (rowKey !== currentId || canRecoverCurrent) ? (
          <Button
            size="sm"
            variant="secondary"
            className="h-6 shrink-0 px-2 text-xs"
            data-testid="run-recover"
            onClick={() => {
              void onRecover(session.active_run_id as string);
            }}
          >
            恢复
          </Button>
        ) : null}
        {ungrouped && onAttach && works.length > 0 ? (
          <select
            aria-label="放到分组"
            className="h-5 max-w-[3.25rem] shrink-0 rounded border border-border bg-background text-[10px]"
            defaultValue=""
            onChange={(event) => {
              const workId = event.target.value;
              if (workId) {
                void onAttach(session, workId);
              }
            }}
          >
            <option value="">放到</option>
            {works.map((work) => (
              <option key={work.id} value={work.id}>
                {work.title}
              </option>
            ))}
          </select>
        ) : null}
        {onArchive ? (
          <button
            type="button"
            aria-label={`归档 ${sessionTitle(session.id, session.summary)}`}
            disabled={busy}
            className={cn(
              "inline-flex size-5 shrink-0 items-center justify-center rounded-md text-muted-foreground/55 transition-colors",
              "hover:bg-muted hover:text-foreground",
              "opacity-80 group-hover:opacity-100 focus-visible:opacity-100",
            )}
            onClick={(event) => {
              event.stopPropagation();
              void onArchive(session);
            }}
          >
            <Archive className="size-3" />
          </button>
        ) : null}
        <span className="w-7 shrink-0 text-right text-[10px] tabular-nums text-muted-foreground/70">
          {compactAge(session.updated_at)}
        </span>
      </div>
    </li>
  );
}

// SidebarSessionTitle 正文可裁，末尾 (n) 不参与省略。进行中两段各自扫光，待审批改成琥珀呼吸。
function SidebarSessionTitle({
  id,
  summary,
  running,
  awaiting,
}: {
  id: string;
  summary?: string;
  running?: boolean;
  awaiting?: boolean;
}) {
  const { stem, suffix } = sessionTitleParts(id, summary);
  const motion = awaiting ? "live-status-waiting" : running ? "live-status-active" : undefined;
  return (
    <span className="flex min-w-0 text-xs font-medium">
      <span className={cn("min-w-0 truncate", motion)}>{stem}</span>
      {suffix ? <span className={cn("shrink-0", motion)}>{suffix}</span> : null}
    </span>
  );
}

// SessionEngineMark 只用品牌图区分引擎，不再贴文字徽章。
function SessionEngineMark({
  engine,
  brandSrc,
  codexIconSrc,
  claudeIconSrc,
}: {
  engine: SessionEngine;
  brandSrc?: string;
  codexIconSrc?: string;
  claudeIconSrc?: string;
}) {
  if (engine === "codex") {
    if (!codexIconSrc) {
      return null;
    }
    return (
      <img
        src={codexIconSrc}
        alt="Codex"
        className="size-4 shrink-0 rounded-[4px] shadow-[0_0_0_1px_rgba(255,255,255,0.28),0_0_8px_rgba(88,122,255,0.45)]"
      />
    );
  }
  if (engine === "claude") {
    if (!claudeIconSrc) {
      return null;
    }
    return (
      <img
        src={claudeIconSrc}
        alt="Claude"
        className="size-4 shrink-0 rounded-[4px] shadow-[0_0_0_1px_rgba(255,255,255,0.22),0_0_8px_rgba(217,119,87,0.45)]"
      />
    );
  }
  if (!brandSrc) {
    return null;
  }
  return (
    <span className="flex size-4 shrink-0 items-center justify-center rounded-[4px] bg-zinc-800 shadow-[0_0_0_1px_rgba(255,255,255,0.22),0_0_6px_rgba(244,244,245,0.16)]">
      <img src={brandSrc} alt="Local" className="size-3" />
    </span>
  );
}

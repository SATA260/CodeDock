"use client";

import type { BoardEngine, InboxItem, SessionView } from "@codedock/core/board";
import { Button, MessageResponse } from "@codedock/ui";
import { useEffect, useRef, useState } from "react";

import { useClaude } from "../claude/provider.tsx";
import type { SessionEngine } from "../chat/chat-page.tsx";
import { relativeTime, shortWorkspace } from "../chat/lib/format.ts";
import { useCodex } from "../codex/provider.tsx";
import { useAgent } from "../provider.tsx";
import { InboxActions } from "./inbox-actions.tsx";
import { InfoEditor } from "./info-editor.tsx";
import type { BoardColumn } from "./layout.ts";
import { useBoard } from "./provider.tsx";

// WorkColumn 一列上头是卡信息，下面挂会话并可直接批。
export function WorkColumn({
  column,
  onOpenSession,
  onDraftSession,
  onArchived,
  onRefresh,
}: {
  column: BoardColumn;
  onOpenSession: (id: string, engine: SessionEngine) => void;
  onDraftSession?: (workId: string, engine: SessionEngine, directory: string, title: string) => void;
  onArchived?: (id: string, engine: SessionEngine) => void;
  onRefresh: () => Promise<void> | void;
}) {
  const { client } = useBoard();
  const { pickDirectory } = useAgent();
  const [title, setTitle] = useState(column.title);
  const [info, setInfo] = useState(column.card?.info.body ?? "");
  const [editingInfo, setEditingInfo] = useState(false);
  const [engine, setEngine] = useState<BoardEngine>("agent");
  const [directory, setDirectory] = useState("");
  const [inbox, setInbox] = useState<InboxItem[]>([]);
  const [error, setError] = useState<string | null>(null);
  const workId = column.card?.work.id;
  const titleFocused = useRef(false);
  const skipTitleSave = useRef(false);

  useEffect(() => {
    if (!titleFocused.current) {
      setTitle(column.title);
    }
    setInfo(column.card?.info.body ?? "");
  }, [column.card?.info.body, column.title]);

  useEffect(() => {
    if (!workId) {
      setInbox([]);
      return;
    }
    void client.listInbox(workId).then(setInbox).catch(() => setInbox([]));
  }, [client, workId, column.card?.pending, column.sessions]);

  // saveTitle 离开输入框时改列头标题。用输入框里的字，避免回车时状态还没提交。空白改回去；重名时改回去并提示。
  const saveTitle = async (raw: string) => {
    titleFocused.current = false;
    if (skipTitleSave.current) {
      skipTitleSave.current = false;
      return;
    }
    const next = raw.trim();
    if (!workId || next === "" || next === column.title) {
      setTitle(column.title);
      return;
    }
    setError(null);
    try {
      await client.updateWork(workId, next);
      await onRefresh();
    } catch (err) {
      setTitle(column.title);
      setError(workTitleError(err));
    }
  };

  // revertTitle 放弃这次修改，不提交。
  const revertTitle = () => {
    skipTitleSave.current = true;
    titleFocused.current = false;
    setTitle(column.title);
  };

  // saveInfo 从编辑框覆盖写入这张卡的说明。
  const saveInfo = async (next: string) => {
    if (!workId) {
      return;
    }
    if (next !== (column.card?.info.body ?? "")) {
      await client.putInfo(workId, next);
      setInfo(next);
    }
    setEditingInfo(false);
    void onRefresh();
  };

  // pickCreateDirectory 为接下来要创建的会话选一个目录。
  const pickCreateDirectory = async () => {
    if (!pickDirectory) {
      return;
    }
    setError(null);
    try {
      const path = await pickDirectory({ start: directory || undefined });
      if (path) {
        setDirectory(path);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法选择目录");
    }
  };

  // prepareSession 弹出悬浮输入窗。发出消息才建会话，并挂到这张卡。
  const prepareSession = () => {
    if (!workId || !onDraftSession) {
      return;
    }
    onDraftSession(workId, engine, directory, title.trim() || column.title);
  };

  return (
    <section className="flex h-full w-full min-w-0 flex-col overflow-hidden rounded-md border border-border bg-background">
      <header className="shrink-0 space-y-2 border-b border-border px-2 py-2">
        {column.ungrouped ? (
          <h2 className="text-sm font-semibold">未分组</h2>
        ) : (
          <input
            value={title}
            aria-label="分组标题"
            onChange={(event) => setTitle(event.target.value)}
            onFocus={() => {
              titleFocused.current = true;
            }}
            onBlur={(event) => void saveTitle(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.currentTarget.blur();
              } else if (event.key === "Escape") {
                revertTitle();
                event.currentTarget.blur();
              }
            }}
            className="h-7 w-full cursor-text rounded border border-border bg-background px-1.5 text-sm font-semibold text-foreground outline-none focus:border-foreground/40"
          />
        )}
        {column.card ? (
          <>
            <div className="flex items-start gap-1">
              <div className="max-h-16 min-h-8 min-w-0 flex-1 overflow-hidden text-[11px] leading-4">
                {info.trim() ? (
                  <MessageResponse className="pointer-events-none text-[11px] [&_h1]:text-xs [&_h2]:text-xs [&_h3]:text-xs [&_pre]:p-1.5">
                    {info}
                  </MessageResponse>
                ) : (
                  <p className="text-muted-foreground">还没有说明</p>
                )}
              </div>
              <Button size="sm" variant="ghost" className="shrink-0 px-1.5" onClick={() => setEditingInfo(true)}>
                编辑
              </Button>
            </div>
            <InfoEditor open={editingInfo} value={info} onClose={() => setEditingInfo(false)} onSave={saveInfo} />
            <div className="flex flex-wrap items-center gap-1 text-[11px] text-muted-foreground">
              <span>{column.sessions.length} 会话</span>
              <span>进行中 {executingCount(column.sessions)}</span>
              <span>待审批 {pendingCount(column.sessions)}</span>
            </div>
            <div className="flex items-center gap-1 text-[11px]">
              <button
                type="button"
                className="h-6 min-w-0 flex-1 truncate rounded border border-border bg-background px-1.5 text-left font-mono text-muted-foreground hover:text-foreground"
                title={directory || "选择目录"}
                disabled={!pickDirectory}
                onClick={() => void pickCreateDirectory()}
              >
                {directory ? shortWorkspace(directory) : "选择目录"}
              </button>
              {directory ? (
                <button type="button" className="shrink-0 text-muted-foreground hover:text-foreground" onClick={() => setDirectory("")}>
                  清除
                </button>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center gap-1">
              <select
                value={engine}
                onChange={(event) => setEngine(event.target.value as BoardEngine)}
                className="h-6 rounded border border-border bg-background px-1 text-[11px]"
              >
                <option value="agent">Local</option>
                <option value="codex">Codex</option>
                <option value="claude">Claude</option>
              </select>
              <Button size="sm" variant="secondary" disabled={!onDraftSession} onClick={prepareSession}>
                创建会话
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  void client.deleteWork(workId ?? "").then(onRefresh);
                }}
              >
                删除
              </Button>
            </div>
          </>
        ) : (
          <p className="text-[11px] text-muted-foreground">可以先聊天，再放到某个分组。</p>
        )}
        {error ? <p className="text-[11px] text-destructive">{error}</p> : null}
      </header>
      <ul className="min-h-0 flex-1 space-y-1 overflow-y-auto px-2 py-2">
        {column.sessions.length === 0 ? (
          <li className="px-1 py-4 text-[11px] text-muted-foreground">还没有会话</li>
        ) : (
          column.sessions.map((session) => {
            const sessionInbox = inbox.filter((item) => item.session_id === session.session_id);
            const waiting = session.pending > 0;
            const executing = session.running && !waiting;
            return (
              <li key={`${session.engine}:${session.session_id}`} className="rounded-md border border-border/70 px-2 py-1.5">
                <div className="flex items-start gap-2">
                  <div className="min-w-0 flex-1">
                    <button
                      type="button"
                      className={`block w-full truncate text-left text-sm font-medium hover:text-foreground${executing ? " live-status-active" : ""}${waiting ? " live-status-waiting" : ""}`}
                      onClick={() => onOpenSession(session.session_id, session.engine)}
                    >
                      {session.summary || session.session_id}
                    </button>
                    <p className="mt-0.5 text-[11px] text-muted-foreground">
                      {engineLabel(session.engine)}
                      {executing ? <span className="live-status-active"> · 进行中</span> : ""}
                      {waiting ? <span className="live-status-waiting"> · 待审批</span> : ""}
                      {session.updated_at ? ` · ${relativeTime(session.updated_at)}` : ""}
                    </p>
                  </div>
                  <SessionArchive session={session} onRefresh={onRefresh} onArchived={onArchived} />
                </div>
                <SessionDirectory session={session} />
                <InboxActions
                  items={sessionInbox}
                  onDone={async () => {
                    if (workId) {
                      setInbox(await client.listInbox(workId));
                    }
                    await onRefresh();
                  }}
                />
                {column.ungrouped ? (
                  <AttachMenu
                    sessionId={session.session_id}
                    engine={session.engine}
                    onRefresh={onRefresh}
                  />
                ) : null}
              </li>
            );
          })
        )}
      </ul>
    </section>
  );
}

// SessionArchive 按引擎归档这路会话；进行中的 Local 先停掉再归档。
function SessionArchive({
  session,
  onRefresh,
  onArchived,
}: {
  session: SessionView;
  onRefresh: () => Promise<void> | void;
  onArchived?: (id: string, engine: SessionEngine) => void;
}) {
  const { client } = useAgent();
  const { client: codex } = useCodex();
  const { client: claude } = useClaude();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const label = session.summary || session.session_id;

  // archive 归档后刷新列，并通知外层关掉还开着的悬浮窗。
  const archive = async () => {
    setBusy(true);
    setError(null);
    try {
      if (session.engine === "codex") {
        await codex.archiveSession(session.session_id);
      } else if (session.engine === "claude") {
        await claude.archiveSession(session.session_id);
      } else {
        const current = await client.getSession(session.session_id);
        if (current.active_run_id) {
          try {
            await client.cancelRun(current.active_run_id);
          } catch {
            // 已结束则继续归档
          }
        }
        await client.archiveSession(session.session_id);
      }
      onArchived?.(session.session_id, session.engine);
      await onRefresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "归档失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="shrink-0 text-right">
      <button
        type="button"
        aria-label={`归档 ${label}`}
        disabled={busy}
        className="text-[11px] text-muted-foreground hover:text-foreground disabled:opacity-40"
        onClick={() => void archive()}
      >
        归档
      </button>
      {error ? <p className="max-w-16 text-[11px] text-destructive">{error}</p> : null}
    </div>
  );
}

// SessionDirectory 只展示创建会话时绑上的目录。之后不能改，也不能解绑。
function SessionDirectory({ session }: { session: SessionView }) {
  return (
    <div className="mt-1 text-[11px] text-muted-foreground">
      {session.checkout ? (
        <p className="truncate font-mono" title={session.checkout}>
          {shortWorkspace(session.checkout)}
          {session.branch ? ` · ${session.branch}` : ""}
          {session.dirty ? " · 有改动" : ""}
        </p>
      ) : (
        <p>还没有目录</p>
      )}
    </div>
  );
}

// AttachMenu 把未归组会话补挂到一张已有卡。
function AttachMenu({
  sessionId,
  engine,
  onRefresh,
}: {
  sessionId: string;
  engine: SessionEngine;
  onRefresh: () => Promise<void> | void;
}) {
  const { client } = useBoard();
  const [works, setWorks] = useState<{ id: string; title: string }[]>([]);
  useEffect(() => {
    void client.listWorks().then((items) => setWorks(items.map((item) => ({ id: item.id, title: item.title }))));
  }, [client]);
  if (works.length === 0) {
    return null;
  }
  return (
    <select
      className="mt-1 h-6 w-full rounded border border-border bg-background text-[11px]"
      defaultValue=""
      onChange={(event) => {
        const workId = event.target.value;
        if (!workId) {
          return;
        }
        void client.attachPlacement(workId, engine, sessionId).then(onRefresh);
      }}
    >
      <option value="">放到…</option>
      {works.map((work) => (
        <option key={work.id} value={work.id}>
          {work.title}
        </option>
      ))}
    </select>
  );
}

// executingCount 只数还在执行的会话。等审批的不记成进行中。
function executingCount(sessions: { running: boolean; pending: number }[]): number {
  return sessions.filter((session) => session.running && session.pending === 0).length;
}

// pendingCount 把列内待审批条数加总。
function pendingCount(sessions: { pending: number }[]): number {
  return sessions.reduce((sum, session) => sum + session.pending, 0);
}

// workTitleError 把重名失败说成界面上的短句。
export function workTitleError(err: unknown): string {
  const raw = err instanceof Error ? err.message : "无法保存";
  if (raw.includes("work title already exists")) {
    return "已有同名分组";
  }
  return raw;
}

// engineLabel 列内用短名区分引擎。
function engineLabel(engine: string): string {
  if (engine === "codex") {
    return "Codex";
  }
  if (engine === "claude") {
    return "Claude";
  }
  return "Local";
}

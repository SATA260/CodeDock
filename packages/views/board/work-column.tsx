"use client";

import type { BoardEngine, InboxItem, SessionView } from "@codedock/core/board";
import { Button, MessageResponse } from "@codedock/ui";
import { useEffect, useState } from "react";

import type { SessionEngine } from "../chat/chat-page.tsx";
import { relativeTime, shortWorkspace } from "../chat/lib/format.ts";
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
  onRefresh,
}: {
  column: BoardColumn;
  onOpenSession: (id: string, engine: SessionEngine) => void;
  onDraftSession?: (workId: string, engine: SessionEngine, directory: string) => void;
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

  useEffect(() => {
    setTitle(column.title);
    setInfo(column.card?.info.body ?? "");
  }, [column.card?.info.body, column.title]);

  useEffect(() => {
    if (!workId) {
      setInbox([]);
      return;
    }
    void client.listInbox(workId).then(setInbox).catch(() => setInbox([]));
  }, [client, workId, column.card?.pending, column.sessions]);

  // saveTitle 改列头标题。
  const saveTitle = async () => {
    if (!workId || title.trim() === "" || title === column.title) {
      return;
    }
    await client.updateWork(workId, title.trim());
    await onRefresh();
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

  // prepareSession 记下引擎和目录，回到输入框。发出消息才建会话，并挂到这张卡。
  const prepareSession = () => {
    if (!workId || !onDraftSession) {
      return;
    }
    onDraftSession(workId, engine, directory);
  };

  return (
    <section className="flex h-full min-w-[240px] flex-1 flex-col overflow-hidden rounded-md border border-border bg-background">
      <header className="shrink-0 space-y-2 border-b border-border px-2 py-2">
        {column.ungrouped ? (
          <h2 className="text-sm font-semibold">未分组</h2>
        ) : (
          <input
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            onBlur={() => void saveTitle()}
            className="w-full bg-transparent text-sm font-semibold outline-none"
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
              <span>进行中 {column.card.running}</span>
              <span>待审批 {column.card.pending}</span>
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
            return (
              <li key={`${session.engine}:${session.session_id}`} className="rounded-md border border-border/70 px-2 py-1.5">
                <button
                  type="button"
                  className={`block w-full truncate text-left text-sm font-medium hover:text-foreground${session.running ? " live-status-active" : ""}`}
                  onClick={() => onOpenSession(session.session_id, session.engine)}
                >
                  {session.summary || session.session_id}
                </button>
                <p className="mt-0.5 text-[11px] text-muted-foreground">
                  {engineLabel(session.engine)}
                  {session.running ? <span className="live-status-active"> · 进行中</span> : ""}
                  {session.pending ? ` · 待审批 ${session.pending}` : ""}
                  {session.updated_at ? ` · ${relativeTime(session.updated_at)}` : ""}
                </p>
                <SessionDirectory session={session} onRefresh={onRefresh} />
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

// SessionDirectory 在会话上绑定或解绑目录。
function SessionDirectory({
  session,
  onRefresh,
}: {
  session: SessionView;
  onRefresh: () => Promise<void> | void;
}) {
  const { client } = useBoard();
  const { pickDirectory } = useAgent();
  const [error, setError] = useState<string | null>(null);

  // bind 弹出系统目录选择框，把选中的目录记到这路会话上。
  const bind = async () => {
    if (!pickDirectory) {
      return;
    }
    const path = await pickDirectory();
    if (!path) {
      return;
    }
    setError(null);
    try {
      await client.bindDirectory(session.engine, session.session_id, path);
      await onRefresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法绑定目录");
    }
  };

  return (
    <div className="mt-1 space-y-0.5 text-[11px] text-muted-foreground">
      {session.checkout ? (
        <p className="truncate font-mono" title={session.checkout}>
          {shortWorkspace(session.checkout)}
          {session.branch ? ` · ${session.branch}` : ""}
          {session.dirty ? " · 有改动" : ""}
        </p>
      ) : (
        <p>还没有目录</p>
      )}
      <div className="flex gap-2">
        <button type="button" className="hover:text-foreground" disabled={!pickDirectory} onClick={() => void bind()}>
          绑定目录
        </button>
        {session.checkout ? (
          <button
            type="button"
            className="hover:text-foreground"
            onClick={() => {
              setError(null);
              void client.clearDirectory(session.engine, session.session_id).then(onRefresh).catch((err: unknown) => {
                setError(err instanceof Error ? err.message : "无法解绑");
              });
            }}
          >
            解绑
          </button>
        ) : null}
      </div>
      {error ? <p className="text-destructive">{error}</p> : null}
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

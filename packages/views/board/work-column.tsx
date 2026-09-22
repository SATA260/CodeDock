"use client";

import type { BoardEngine, InboxItem } from "@codedock/core/board";
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
  onRefresh,
}: {
  column: BoardColumn;
  onOpenSession: (id: string, engine: SessionEngine) => void;
  onRefresh: () => Promise<void> | void;
}) {
  const { client } = useBoard();
  const { pickDirectory } = useAgent();
  const [title, setTitle] = useState(column.title);
  const [info, setInfo] = useState(column.card?.info.body ?? "");
  const [editingInfo, setEditingInfo] = useState(false);
  const [engine, setEngine] = useState<BoardEngine>("agent");
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

  // startTalk 在这张卡上开问答会话并打开悬浮窗。
  const startTalk = async () => {
    if (!workId) {
      return;
    }
    setError(null);
    try {
      const started = await client.startSession(workId, { engine, kind: "talk" });
      await onRefresh();
      onOpenSession(started.session_id, started.engine);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法开会话");
    }
  };

  // startInDir 在已挂目录上开新会话。
  const startInDir = async (path: string) => {
    if (!workId) {
      return;
    }
    setError(null);
    try {
      const started = await client.startSession(workId, { engine, kind: "dir", checkout: path });
      await onRefresh();
      onOpenSession(started.session_id, started.engine);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法开目录会话");
    }
  };

  // attachDir 弹出系统目录选择框后挂到卡上。
  const attachDir = async () => {
    if (!workId || !pickDirectory) {
      return;
    }
    const path = await pickDirectory();
    if (!path) {
      return;
    }
    await client.attachCheckout(workId, path);
    await onRefresh();
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
            <div className="space-y-1 text-[11px] text-muted-foreground">
              {(column.card.dirs ?? []).map((dir) => (
                <div key={dir.path} className="flex items-start justify-between gap-1">
                  <button type="button" className="min-w-0 text-left hover:text-foreground" onClick={() => void startInDir(dir.path)}>
                    <span className="block truncate font-mono" title={dir.path}>
                      {shortWorkspace(dir.path)}
                    </span>
                    <span>
                      {dir.branch || "—"}
                      {dir.dirty ? " · 有改动" : ""}
                    </span>
                  </button>
                  <button
                    type="button"
                    className="shrink-0 text-muted-foreground hover:text-foreground"
                    onClick={() => {
                      void client.detachCheckout(workId ?? "", dir.path).then(onRefresh);
                    }}
                  >
                    解绑
                  </button>
                </div>
              ))}
            </div>
            <div className="flex flex-wrap items-center gap-1 text-[11px] text-muted-foreground">
              <span>{column.sessions.length} 会话</span>
              <span>进行中 {column.card.running}</span>
              <span>待审批 {column.card.pending}</span>
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
              <Button size="sm" variant="secondary" onClick={() => void startTalk()}>
                创建会话
              </Button>
              <Button size="sm" variant="ghost" disabled={!pickDirectory} onClick={() => void attachDir()}>
                绑定目录
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
                  className="block w-full truncate text-left text-sm font-medium hover:text-foreground"
                  onClick={() => onOpenSession(session.session_id, session.engine)}
                >
                  {session.summary || session.session_id}
                </button>
                <p className="mt-0.5 text-[11px] text-muted-foreground">
                  {engineLabel(session.engine)}
                  {session.running ? " · 进行中" : ""}
                  {session.pending ? ` · 待审批 ${session.pending}` : ""}
                  {session.updated_at ? ` · ${relativeTime(session.updated_at)}` : ""}
                </p>
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

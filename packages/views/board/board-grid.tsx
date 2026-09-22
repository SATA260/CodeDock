"use client";

import type { BoardView } from "@codedock/core/board";
import { useEffect, useRef, useState } from "react";

import type { SessionEngine } from "../chat/chat-page.tsx";
import { BOARD_COL_WIDTH, boardColumns, revealColumnCount, visibleColumnCount } from "./layout.ts";
import { useBoard } from "./provider.tsx";
import { WorkColumn } from "./work-column.tsx";

// BoardGrid 横向 Work 列：列宽固定，滑到右端再挂下一列。
export function BoardGrid({
  onOpenSession,
  onDraftSession,
  onArchived,
  revision = 0,
  notice = null,
}: {
  onOpenSession: (id: string, engine: SessionEngine) => void;
  /** 在看板上弹出输入窗；发出消息才建会话。 */
  onDraftSession?: (workId: string, engine: SessionEngine, directory: string, title: string) => void;
  onArchived?: (id: string, engine: SessionEngine) => void;
  /** 有新会话挂上后加一，用来重拉列。 */
  revision?: number;
  /** 顶栏操作失败时显示在列上方，例如新建分组。 */
  notice?: string | null;
}) {
  const { client } = useBoard();
  const [view, setView] = useState<BoardView>({ cards: [], ungrouped: [] });
  const [width, setWidth] = useState(720);
  const [shown, setShown] = useState(() => visibleColumnCount(720));
  const [loadError, setLoadError] = useState<string | null>(null);
  const frameRef = useRef<HTMLDivElement>(null);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // reload 只拉摘要，不加载对话正文。
  const reload = async () => {
    const next = await client.getBoard();
    setView(next);
  };

  useEffect(() => {
    void reload()
      .then(() => setLoadError(null))
      .catch((err: unknown) => {
        setLoadError(err instanceof Error ? err.message : "无法加载看板");
      });
  }, [client, revision]);

  const columns = boardColumns(view);
  const live = columns.some((column) =>
    column.sessions.some((session) => session.running || session.pending > 0),
  );

  // 进行中或待审批时重拉摘要，结束和问票出现后状态跟着变。
  useEffect(() => {
    if (!live) {
      return;
    }
    const timer = window.setInterval(() => {
      void client.getBoard().then(setView).catch(() => undefined);
    }, 3000);
    return () => window.clearInterval(timer);
  }, [client, live]);

  useEffect(() => {
    const node = frameRef.current;
    if (!node || typeof ResizeObserver === "undefined") {
      return;
    }
    const frame = new ResizeObserver((entries) => {
      const next = entries[0]?.contentRect.width ?? 0;
      if (next > 0) {
        setWidth(next);
      }
    });
    frame.observe(node);
    setWidth(node.clientWidth || 720);
    return () => frame.disconnect();
  }, []);

  const batch = visibleColumnCount(width);
  const visible = columns.slice(0, shown);
  const columnCount = useRef(columns.length);

  useEffect(() => {
    setShown((current) => Math.max(current, batch));
  }, [batch]);

  useEffect(() => {
    const previous = columnCount.current;
    columnCount.current = columns.length;
    if (previous > 1 && columns.length > previous) {
      setShown((current) => current + columns.length - previous);
    }
  }, [columns.length]);

  useEffect(() => {
    const root = scrollerRef.current;
    const sentinel = sentinelRef.current;
    if (!root || !sentinel || shown >= columns.length) {
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        setShown((current) => revealColumnCount(current, columns.length, 1));
      }
    }, { root, rootMargin: "0px 48px 0px 0px" });
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [columns.length, shown]);

  return (
    <div ref={frameRef} className="flex min-h-0 flex-1 flex-col gap-2 px-3 py-2">
      {notice || loadError ? <p className="shrink-0 text-xs text-destructive">{notice || loadError}</p> : null}
      <div ref={scrollerRef} className="flex min-h-0 min-w-0 flex-1 gap-2 overflow-x-auto">
        {visible.map((column) => (
          <div
            key={column.id}
            className="h-full shrink-0"
            style={{ width: BOARD_COL_WIDTH }}
          >
            <WorkColumn
              column={column}
              onOpenSession={onOpenSession}
              onDraftSession={onDraftSession}
              onArchived={onArchived}
              onRefresh={reload}
            />
          </div>
        ))}
        {shown < columns.length ? <div ref={sentinelRef} className="w-px shrink-0" aria-hidden /> : null}
      </div>
    </div>
  );
}

"use client";

import type { BoardView } from "@codedock/core/board";
import { Button } from "@codedock/ui";
import { useEffect, useRef, useState } from "react";

import type { SessionEngine } from "../chat/chat-page.tsx";
import { BOARD_COL_WIDTH, boardColumns, revealColumnCount, visibleColumnCount } from "./layout.ts";
import { useBoard } from "./provider.tsx";
import { WorkColumn, workTitleError } from "./work-column.tsx";

// BoardGrid 横向 Work 列：列宽固定，滑到右端再挂下一列。
export function BoardGrid({
  onOpenSession,
  onDraftSession,
  onArchived,
  revision = 0,
}: {
  onOpenSession: (id: string, engine: SessionEngine) => void;
  /** 在看板上弹出输入窗；发出消息才建会话。 */
  onDraftSession?: (workId: string, engine: SessionEngine, directory: string, title: string) => void;
  onArchived?: (id: string, engine: SessionEngine) => void;
  /** 有新会话挂上后加一，用来重拉列。 */
  revision?: number;
}) {
  const { client } = useBoard();
  const [view, setView] = useState<BoardView>({ cards: [], ungrouped: [] });
  const [width, setWidth] = useState(720);
  const [shown, setShown] = useState(() => visibleColumnCount(720));
  const [error, setError] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const frameRef = useRef<HTMLDivElement>(null);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // reload 只拉摘要，不加载对话正文。
  const reload = async () => {
    const next = await client.getBoard();
    setView(next);
  };

  useEffect(() => {
    void reload().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : "无法加载看板");
    });
  }, [client, revision]);

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

  const columns = boardColumns(view);
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
      <div className="flex shrink-0 items-center gap-2">
        <input
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          placeholder="标题"
          className="h-7 min-w-0 flex-1 rounded border border-border bg-background px-2 text-sm"
        />
        <Button
          size="sm"
          onClick={() => {
            setError(null);
            void client
              .createWork(title.trim())
              .then(() => {
                setTitle("");
                return reload();
              })
              .catch((err: unknown) => {
                setError(workTitleError(err));
              });
          }}
        >
          新建
        </Button>
      </div>
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
      <div ref={scrollerRef} className="flex min-h-0 min-w-0 flex-1 gap-2 overflow-x-auto">
        {visible.map((column) => (
          <div key={column.id} className="h-full shrink-0" style={{ width: BOARD_COL_WIDTH }}>
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

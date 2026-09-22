"use client";

import type { BoardView } from "@codedock/core/board";
import { Button } from "@codedock/ui";
import { useEffect, useRef, useState } from "react";

import type { SessionEngine } from "../chat/chat-page.tsx";
import { boardColumns, columnsPerRow, pageColumns, pageCount } from "./layout.ts";
import { useBoard } from "./provider.tsx";
import { WorkColumn } from "./work-column.tsx";

// BoardGrid 横向 Work 列：列高滚动、每行限列、底部分页。
export function BoardGrid({
  onOpenSession,
}: {
  onOpenSession: (id: string, engine: SessionEngine) => void;
}) {
  const { client } = useBoard();
  const [view, setView] = useState<BoardView>({ cards: [], ungrouped: [] });
  const [width, setWidth] = useState(720);
  const [page, setPage] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const frameRef = useRef<HTMLDivElement>(null);

  // reload 只拉摘要，不加载对话正文。
  const reload = async () => {
    const next = await client.getBoard();
    setView(next);
  };

  useEffect(() => {
    void reload().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : "无法加载看板");
    });
  }, [client]);

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

  const perPage = columnsPerRow(width);
  const columns = boardColumns(view);
  const pages = pageCount(columns.length, perPage);
  const current = Math.min(page, pages - 1);
  const visible = pageColumns(columns, current, perPage);

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
            void client.createWork(title.trim() || "未命名").then(() => {
              setTitle("");
              return reload();
            });
          }}
        >
          新建
        </Button>
      </div>
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
      <div className="flex min-h-0 flex-1 gap-2">
        {visible.map((column) => (
          <WorkColumn
            key={column.id}
            column={column}
            onOpenSession={onOpenSession}
            onRefresh={reload}
          />
        ))}
      </div>
      <div className="flex shrink-0 items-center justify-center gap-2 pb-1 text-xs text-muted-foreground">
        <Button size="sm" variant="ghost" disabled={current <= 0} onClick={() => setPage(current - 1)}>
          上一页
        </Button>
        <span>
          {current + 1} / {pages}
        </span>
        <Button
          size="sm"
          variant="ghost"
          disabled={current >= pages - 1}
          onClick={() => setPage(current + 1)}
        >
          下一页
        </Button>
      </div>
    </div>
  );
}

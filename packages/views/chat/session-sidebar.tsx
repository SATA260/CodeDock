"use client";

import type { Session } from "@codedock/core/chat";
import { Button, cn } from "@codedock/ui";
import { Archive, PlusIcon, Trash2Icon } from "lucide-react";
import { useEffect, useState } from "react";

import { relativeTime, sessionTitle, shortId } from "./lib/format.ts";

export type SidebarSession = Session & { engine?: "agent" | "codex" };

export function SessionSidebar({
  sessions,
  currentId,
  busy,
  error,
  hasMore = false,
  onLoadMore,
  onCreate,
  onSelect,
  onRecover,
  onDelete,
  onArchive,
  canRecoverCurrent = false,
  canArchive = false,
  brandSrc,
}: {
  sessions: SidebarSession[];
  currentId?: string;
  busy: boolean;
  error: string | null;
  hasMore?: boolean;
  onLoadMore?: () => void;
  onCreate: () => void;
  onSelect: (id: string, engine?: "agent" | "codex") => void;
  onRecover?: (runId: string) => Promise<void>;
  onDelete?: (session: SidebarSession) => Promise<void>;
  onArchive?: () => Promise<void>;
  canRecoverCurrent?: boolean;
  canArchive?: boolean;
  brandSrc?: string;
}) {
  const [pending, setPending] = useState<SidebarSession | null>(null);

  useEffect(() => {
    if (!pending) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        setPending(null);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [busy, pending]);

  return (
    <aside className="flex h-full w-60 shrink-0 flex-col border-r border-border bg-background">
      <div className="flex items-center justify-between gap-2 px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          {brandSrc ? (
            <img src={brandSrc} alt="" className="size-6 shrink-0" />
          ) : null}
          <div className="truncate text-sm font-semibold tracking-tight">CodeDock</div>
        </div>
        <Button size="sm" variant="secondary" disabled={busy} onClick={onCreate}>
          <PlusIcon className="size-3.5" />
          新对话
        </Button>
      </div>
      {error ? <p className="px-3 pb-2 text-xs text-destructive">{error}</p> : null}
      {canArchive && onArchive ? (
        <div className="px-3 pb-2">
          <Button size="sm" variant="outline" className="w-full" disabled={busy} onClick={() => void onArchive()}>
            <Archive className="size-3.5" />
            归档当前对话
          </Button>
        </div>
      ) : null}
      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {sessions.length === 0 ? (
          <p className="px-2 py-6 text-xs text-muted-foreground">还没有会话</p>
        ) : (
          <ul className="space-y-0.5">
            {sessions.map((session) => {
              const engine = session.engine ?? "agent";
              const rowKey = `${engine}:${session.id}`;
              const active = rowKey === currentId;
              return (
                <li key={rowKey}>
                  <div
                    className={cn(
                      "group flex w-full items-start gap-1 rounded-md px-2 py-1.5 leading-5 transition-colors",
                      active
                        ? "bg-muted text-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                    )}
                  >
                    <button
                      type="button"
                      onClick={() => onSelect(session.id, engine)}
                      className="min-w-0 flex-1 text-left"
                    >
                      <div className="flex items-center gap-1.5">
                        <span className="truncate text-sm font-medium">
                          {sessionTitle(session.id, session.summary)}
                        </span>
                        {engine === "codex" ? (
                          <span className="shrink-0 rounded bg-muted px-1 text-[10px] leading-4 text-muted-foreground">
                            Codex
                          </span>
                        ) : null}
                      </div>
                      <div className="mt-0.5 truncate text-xs text-muted-foreground/70">
                        {relativeTime(session.updated_at) || shortId(session.id)}
                      </div>
                    </button>
                    {engine !== "codex" &&
                    session.needs_recover &&
                    session.active_run_id &&
                    onRecover &&
                    (rowKey !== currentId || canRecoverCurrent) ? (
                      <Button
                        size="sm"
                        variant="secondary"
                        className="h-6 shrink-0 px-2 text-xs"
                        onClick={() => {
                          void onRecover(session.active_run_id as string);
                        }}
                      >
                        恢复
                      </Button>
                    ) : null}
                    {onDelete ? (
                      <button
                        type="button"
                        aria-label={`删除 ${sessionTitle(session.id, session.summary)}`}
                        disabled={busy}
                        className={cn(
                          "mt-0.5 inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground/55 transition-colors",
                          "hover:bg-destructive/15 hover:text-destructive",
                          "opacity-80 group-hover:opacity-100 focus-visible:opacity-100",
                        )}
                        onClick={(event) => {
                          event.stopPropagation();
                          setPending(session);
                        }}
                      >
                        <Trash2Icon className="size-3.5" />
                      </button>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
        {hasMore && onLoadMore ? (
          <Button className="mt-2 w-full" size="sm" variant="ghost" onClick={onLoadMore}>
            加载更多
          </Button>
        ) : null}
      </nav>
      {pending && onDelete ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
          onClick={() => {
            if (!busy) {
              setPending(null);
            }
          }}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="delete-session-title"
            className="w-full max-w-sm rounded-lg border border-border bg-background p-4 shadow-xl"
            onClick={(event) => event.stopPropagation()}
          >
            <h2 id="delete-session-title" className="text-sm font-medium text-foreground">
              删除对话
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              确定删除「{sessionTitle(pending.id, pending.summary)}」吗？删除后将从列表中移除。
              {pending.active_run_id ? " 当前任务会先中止。" : ""}
            </p>
            <div className="mt-4 flex justify-end gap-2">
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => setPending(null)}
              >
                取消
              </Button>
              <Button
                size="sm"
                variant="destructive"
                disabled={busy}
                onClick={() => {
                  void onDelete(pending)
                    .then(() => setPending(null))
                    .catch(() => undefined);
                }}
              >
                {busy ? "删除中…" : "删除"}
              </Button>
            </div>
          </div>
        </div>
      ) : null}
    </aside>
  );
}

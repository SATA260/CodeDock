"use client";

import type { Session } from "@codedock/core/chat";
import { Button, cn } from "@codedock/ui";
import { Archive, PlusIcon } from "lucide-react";

import type { SessionEngine } from "./chat-page.tsx";
import { relativeTime, sessionTitle, sessionTitleParts, shortId } from "./lib/format.ts";

export type SidebarSession = Session & { engine?: SessionEngine };

// SessionSidebar 混排 Local / Codex / Claude，引擎只靠图标区分。
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
  onArchive,
  canRecoverCurrent = false,
  brandSrc,
  codexIconSrc,
  claudeIconSrc,
}: {
  sessions: SidebarSession[];
  currentId?: string;
  busy: boolean;
  error: string | null;
  hasMore?: boolean;
  onLoadMore?: () => void;
  onCreate: () => void;
  onSelect: (id: string, engine?: SessionEngine) => void;
  onRecover?: (runId: string) => Promise<void>;
  onArchive?: (session: SidebarSession) => Promise<void>;
  canRecoverCurrent?: boolean;
  brandSrc?: string;
  codexIconSrc?: string;
  claudeIconSrc?: string;
}) {
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
                      "group flex w-full items-center gap-1 rounded-md px-2 py-1.5 leading-5 transition-colors",
                      active
                        ? "bg-muted text-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                    )}
                  >
                    <button
                      type="button"
                      onClick={() => onSelect(session.id, engine)}
                      className="flex min-w-0 flex-1 items-center gap-2.5 text-left"
                    >
                      <SessionEngineMark
                        engine={engine}
                        brandSrc={brandSrc}
                        codexIconSrc={codexIconSrc}
                        claudeIconSrc={claudeIconSrc}
                      />
                      <span className="min-w-0 flex-1">
                        <SidebarSessionTitle id={session.id} summary={session.summary} />
                        <span className="mt-0.5 block truncate text-xs text-muted-foreground/70">
                          {relativeTime(session.updated_at) || shortId(session.id)}
                        </span>
                      </span>
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
                        onClick={() => {
                          void onRecover(session.active_run_id as string);
                        }}
                      >
                        恢复
                      </Button>
                    ) : null}
                    {onArchive ? (
                      <button
                        type="button"
                        aria-label={`归档 ${sessionTitle(session.id, session.summary)}`}
                        disabled={busy}
                        className={cn(
                          "mt-0.5 inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground/55 transition-colors",
                          "hover:bg-muted hover:text-foreground",
                          "opacity-80 group-hover:opacity-100 focus-visible:opacity-100",
                        )}
                        onClick={(event) => {
                          event.stopPropagation();
                          void onArchive(session);
                        }}
                      >
                        <Archive className="size-3.5" />
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
    </aside>
  );
}

// SidebarSessionTitle 正文可裁，末尾 (n) 不参与省略。
function SidebarSessionTitle({ id, summary }: { id: string; summary?: string }) {
  const { stem, suffix } = sessionTitleParts(id, summary);
  return (
    <span className="flex min-w-0 text-sm font-medium">
      <span className="min-w-0 truncate">{stem}</span>
      {suffix ? <span className="shrink-0">{suffix}</span> : null}
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
        className="size-8 shrink-0 rounded-[8px] shadow-[0_0_0_1px_rgba(255,255,255,0.28),0_0_12px_rgba(88,122,255,0.55)]"
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
        className="size-8 shrink-0 rounded-[8px] shadow-[0_0_0_1px_rgba(255,255,255,0.22),0_0_12px_rgba(217,119,87,0.55)]"
      />
    );
  }
  if (!brandSrc) {
    return null;
  }
  return (
    <span className="flex size-8 shrink-0 items-center justify-center rounded-[8px] bg-zinc-800 shadow-[0_0_0_1px_rgba(255,255,255,0.22),0_0_10px_rgba(244,244,245,0.2)]">
      <img src={brandSrc} alt="Local" className="size-6" />
    </span>
  );
}

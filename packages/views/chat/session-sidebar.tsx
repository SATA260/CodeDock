"use client";

import type { Session } from "@codedock/core/chat";
import { Button, cn } from "@codedock/ui";
import { PlusIcon } from "lucide-react";

import { relativeTime, sessionTitle, shortId } from "./lib/format.ts";

export function SessionSidebar({
  sessions,
  currentId,
  busy,
  error,
  onCreate,
  onSelect,
  onRecover,
  canRecoverCurrent = false,
  brandSrc,
}: {
  sessions: Session[];
  currentId?: string;
  busy: boolean;
  error: string | null;
  onCreate: () => void;
  onSelect: (id: string) => void;
  onRecover?: (runId: string) => Promise<void>;
  canRecoverCurrent?: boolean;
  brandSrc?: string;
}) {
  return (
    <aside className="flex h-full w-60 shrink-0 flex-col border-r border-border bg-background">
      <div className="flex items-center justify-between gap-2 px-3 py-3">
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
              const active = session.id === currentId;
              return (
                <li key={session.id}>
                  <div
                    className={cn(
                      "flex w-full items-start gap-2 rounded-md px-2 py-2 transition-colors",
                      active
                        ? "bg-muted text-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                    )}
                  >
                    <button
                      type="button"
                      onClick={() => onSelect(session.id)}
                      className="min-w-0 flex-1 text-left"
                    >
                      <div className="truncate text-sm font-medium">
                        {sessionTitle(session.id, session.summary)}
                      </div>
                      <div className="mt-0.5 truncate text-xs text-muted-foreground/70">
                        {relativeTime(session.updated_at) || shortId(session.id)}
                      </div>
                    </button>
                    {session.needs_recover &&
                    session.active_run_id &&
                    onRecover &&
                    (session.id !== currentId || canRecoverCurrent) ? (
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
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </nav>
    </aside>
  );
}

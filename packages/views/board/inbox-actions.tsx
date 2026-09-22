"use client";

import type { BoardEngine, InboxItem } from "@codedock/core/board";
import { Button } from "@codedock/ui";
import { useState } from "react";

import { useAgent } from "../provider.tsx";
import { useBoard } from "./provider.tsx";

// InboxActions 在列内直接批一条问票，不把三引擎收成同一种结构。
export function InboxActions({
  items,
  onDone,
}: {
  items: InboxItem[];
  onDone?: () => Promise<void> | void;
}) {
  const { client } = useBoard();
  const { userId } = useAgent();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  if (items.length === 0) {
    return null;
  }
  return (
    <div className="mt-1 space-y-1">
      {items.map((item) => (
        <div key={`${item.engine}:${item.ticket_id}`} className="rounded border border-border/70 px-1.5 py-1">
          <p className="truncate text-[11px] text-muted-foreground">{item.summary || item.ticket_id}</p>
          <div className="mt-1 flex gap-1">
            <Button
              size="sm"
              variant="secondary"
              disabled={busy === item.ticket_id}
              onClick={() => void decide(item, true)}
            >
              通过
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={busy === item.ticket_id}
              onClick={() => void decide(item, false)}
            >
              拒绝
            </Button>
          </div>
        </div>
      ))}
      {error ? <p className="text-[11px] text-destructive">{error}</p> : null}
    </div>
  );

  // decide 按引擎把原问票转给已有裁决入口。
  async function decide(item: InboxItem, approved: boolean) {
    setBusy(item.ticket_id);
    setError(null);
    try {
      const engine = (item.engine === "native" ? "agent" : item.engine) as BoardEngine;
      if (engine === "agent") {
        await client.decideInbox({
          engine,
          ticket_id: item.ticket_id,
          status: approved ? "approved" : "denied",
          scope: "once",
          actor_id: userId,
        });
      } else {
        await client.decideInbox({
          engine,
          ticket_id: item.ticket_id,
          approved,
          scope: "once",
        });
      }
      await onDone?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "审批失败");
    } finally {
      setBusy(null);
    }
  }
}

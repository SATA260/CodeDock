"use client";

import type { ApprovalDecision, TimelineItem } from "@codedock/core/chat";
import { Button, formatJSON } from "@codedock/ui";
import { useState } from "react";

export function ApprovalDock({
  item,
  onDecide,
}: {
  item?: Extract<TimelineItem, { kind: "approval" }>;
  onDecide: (approvalId: string, decisions: ApprovalDecision[]) => Promise<void>;
}) {
  const [submitting, setSubmitting] = useState(false);
  if (!item) {
    return null;
  }

  const decideAll = async (status: "approved" | "denied") => {
    setSubmitting(true);
    try {
      await onDecide(
        item.approvalId,
        item.toolCalls.map((call) => ({ tool_call_id: call.id, status })),
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="mx-auto w-full max-w-3xl px-4 pb-2">
      <div className="rounded-xl border border-border bg-card px-3 py-3 shadow-sm">
        <div className="text-xs font-medium text-muted-foreground">需要你的批准才能继续</div>
        <ul className="mt-2 space-y-2">
          {item.toolCalls.map((call) => (
            <li key={call.id} className="rounded-lg bg-muted/50 px-2.5 py-2">
              <div className="font-mono text-sm text-foreground">{call.name}</div>
              {call.arguments != null ? (
                <pre className="mt-1 max-h-28 overflow-auto font-mono text-xs leading-5 text-muted-foreground">
                  {formatJSON(call.arguments)}
                </pre>
              ) : null}
            </li>
          ))}
        </ul>
        <div className="mt-3 flex justify-end gap-2">
          <Button size="sm" variant="outline" disabled={submitting} onClick={() => void decideAll("denied")}>
            拒绝
          </Button>
          <Button size="sm" disabled={submitting} onClick={() => void decideAll("approved")}>
            允许
          </Button>
        </div>
      </div>
    </div>
  );
}

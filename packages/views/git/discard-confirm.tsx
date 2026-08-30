"use client";

import { Button } from "@codedock/ui";
import { useEffect } from "react";

export type DiscardRequest = {
  name: string;
  paths: string[];
};

export function DiscardConfirm({
  request,
  busy,
  onCancel,
  onConfirm,
}: {
  request: DiscardRequest | null;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  useEffect(() => {
    if (!request) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        onCancel();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [busy, onCancel, request]);

  if (!request) {
    return null;
  }

  const count = request.paths.length;
  const summary =
    count > 1
      ? `将丢掉「${request.name}」下 ${count} 个文件的工作区改动。`
      : `将丢掉「${request.name}」的工作区改动。`;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4"
      role="presentation"
      onClick={() => {
        if (!busy) {
          onCancel();
        }
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="discard-confirm-title"
        className="w-full max-w-sm rounded-lg border border-border bg-background p-4 shadow-xl"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="discard-confirm-title" className="text-sm font-medium">
          撤回更改
        </h2>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {summary}
          未跟踪的文件会被删除，已暂存的内容不受影响。此操作无法撤销。
        </p>
        <div className="mt-4 flex justify-end gap-2">
          <Button size="sm" variant="ghost" disabled={busy} autoFocus onClick={onCancel}>
            取消
          </Button>
          <Button size="sm" variant="destructive" disabled={busy} onClick={onConfirm}>
            撤回
          </Button>
        </div>
      </div>
    </div>
  );
}

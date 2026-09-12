"use client";

import { useEffect, useState } from "react";

import type { PendingFollowup } from "./hooks/use-session-timeline.ts";

export function PendingDock({
  items,
  editingId,
  onBeginEdit,
  onCancelEdit,
  onSave,
  onDelete,
  onSendNow,
}: {
  items: PendingFollowup[];
  editingId?: string | null;
  onBeginEdit?: (id: string) => void;
  onCancelEdit?: () => void;
  onSave?: (id: string, text: string) => void;
  onDelete?: (id: string) => void;
  onSendNow?: () => void;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <div className="flex justify-center px-4 pb-1.5">
      <div
        className="w-full max-w-3xl rounded-xl border border-white/10 bg-zinc-950/40 px-2.5 py-1.5 shadow-[0_12px_40px_-16px_rgba(0,0,0,0.45)] backdrop-blur-md"
        role="dialog"
        aria-label="待发消息"
      >
        <div className="flex items-center justify-between gap-2 text-[10px] leading-3.5 text-muted-foreground">
          <span>排队 {items.length} 条，当前轮结束后发送</span>
          {onSendNow ? (
            <button
              type="button"
              disabled={Boolean(editingId)}
              className="shrink-0 text-[10px] leading-3.5 text-foreground underline-offset-2 hover:underline disabled:text-muted-foreground disabled:no-underline"
              onClick={onSendNow}
            >
              立即发送
            </button>
          ) : null}
        </div>
        <ul className="mt-0.5 max-h-24 divide-y divide-white/10 overflow-y-auto">
          {items.map((item) => (
            <PendingLine
              key={item.id}
              item={item}
              editing={editingId === item.id}
              onBeginEdit={onBeginEdit}
              onCancelEdit={onCancelEdit}
              onSave={onSave}
              onDelete={onDelete}
            />
          ))}
        </ul>
      </div>
    </div>
  );
}

function PendingLine({
  item,
  editing,
  onBeginEdit,
  onCancelEdit,
  onSave,
  onDelete,
}: {
  item: PendingFollowup;
  editing: boolean;
  onBeginEdit?: (id: string) => void;
  onCancelEdit?: () => void;
  onSave?: (id: string, text: string) => void;
  onDelete?: (id: string) => void;
}) {
  const [draft, setDraft] = useState(item.text);

  useEffect(() => {
    if (!editing) {
      setDraft(item.text);
    }
  }, [item.text, editing]);

  const save = () => {
    const next = draft.trim();
    if (!next || !onSave || next === item.text) {
      onCancelEdit?.();
      setDraft(item.text);
      return;
    }
    onSave(item.id, next);
  };

  if (editing) {
    return (
      <li className="py-0.5">
        <textarea
          value={draft}
          rows={2}
          className="w-full resize-none rounded-md border border-white/10 bg-zinc-950/40 px-1.5 py-0.5 text-[11px] leading-4 text-foreground outline-none focus:border-white/25"
          onChange={(event) => setDraft(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              save();
            }
            if (event.key === "Escape") {
              onCancelEdit?.();
              setDraft(item.text);
            }
          }}
        />
        <div className="mt-0.5 flex justify-end gap-2 text-[10px] leading-3.5">
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground"
            onClick={() => {
              onCancelEdit?.();
              setDraft(item.text);
            }}
          >
            取消
          </button>
          <button
            type="button"
            disabled={!draft.trim()}
            className="text-foreground disabled:text-muted-foreground"
            onClick={save}
          >
            保存
          </button>
        </div>
      </li>
    );
  }

  return (
    <li className="flex items-start gap-2 py-0.5">
      <p className="min-w-0 flex-1 whitespace-pre-wrap break-words text-[11px] leading-4 text-foreground">
        {item.text}
      </p>
      <div className="flex shrink-0 gap-1.5 pt-px text-[10px] leading-4 text-muted-foreground">
        {onBeginEdit ? (
          <button type="button" className="hover:text-foreground" onClick={() => onBeginEdit(item.id)}>
            改
          </button>
        ) : null}
        {onDelete ? (
          <button type="button" className="hover:text-foreground" onClick={() => onDelete(item.id)}>
            删
          </button>
        ) : null}
      </div>
    </li>
  );
}

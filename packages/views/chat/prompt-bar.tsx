"use client";

import type { ApprovalMode, WorkMode } from "@codedock/core/chat";
import {
  Button,
  PromptInput,
  PromptInputFooter,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputTools,
  isImeConfirm,
} from "@codedock/ui";
import { ChevronUp } from "lucide-react";
import { useEffect, useRef, useState } from "react";

const workModes: { value: WorkMode; label: string }[] = [
  { value: "agent", label: "agent" },
  { value: "ask", label: "ask" },
  { value: "plan", label: "plan" },
];

const approvalModes: { value: ApprovalMode; label: string }[] = [
  { value: "manual", label: "manual" },
  { value: "auto", label: "auto" },
  { value: "yolo", label: "yolo" },
];

export function PromptBar({
  running,
  sending,
  onSend,
  onCancel,
}: {
  running: boolean;
  sending: boolean;
  onSend: (text: string, mode: WorkMode, approval: ApprovalMode) => Promise<void>;
  onCancel: () => Promise<void>;
}) {
  const [text, setText] = useState("");
  const [mode, setMode] = useState<WorkMode>("agent");
  const [approval, setApproval] = useState<ApprovalMode>("manual");
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const keepFocus = () => {
    inputRef.current?.focus();
  };

  return (
    <div className="relative z-30 mx-auto w-full max-w-3xl px-4 pb-4">
      <PromptInput
        onSend={async (message) => {
          const next = message.text.trim();
          if (!next) {
            return;
          }
          setText("");
          keepFocus();
          try {
            await onSend(next, mode, approval);
          } finally {
            keepFocus();
          }
        }}
      >
        <PromptInputTextarea
          ref={inputRef}
          value={text}
          autoFocus
          placeholder="给 Agent 发消息…"
          onChange={(event) => setText(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key !== "Enter" || event.shiftKey || isImeConfirm(event)) {
              return;
            }
            event.preventDefault();
            event.currentTarget.form?.requestSubmit();
          }}
        />
        <PromptInputFooter>
          <PromptInputTools>
            <ChoiceMenu value={mode} options={workModes} onChange={setMode} />
            <ChoiceMenu value={approval} options={approvalModes} onChange={setApproval} />
            {running ? (
              <Button size="sm" variant="outline" onClick={() => void onCancel()}>
                取消
              </Button>
            ) : null}
          </PromptInputTools>
          <PromptInputSubmit
            status={sending ? "streaming" : "ready"}
            disabled={sending || !text.trim()}
          />
        </PromptInputFooter>
      </PromptInput>
    </div>
  );
}

function ChoiceMenu<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const current = options.find((item) => item.value === value)?.label ?? value;

  useEffect(() => {
    if (!open) {
      return;
    }
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  return (
    <div className="relative" ref={rootRef}>
      <Button
        size="sm"
        variant="outline"
        aria-expanded={open}
        aria-haspopup="listbox"
        onClick={() => setOpen((currentOpen) => !currentOpen)}
      >
        {current}
        <ChevronUp className="size-3.5" />
      </Button>
      {open ? (
        <div
          className="absolute bottom-full left-0 z-50 mb-1 min-w-28 overflow-hidden rounded-md border border-border bg-zinc-900 p-1 shadow-lg"
          role="listbox"
        >
          {options.map((item) => (
            <button
              key={item.value}
              type="button"
              role="option"
              aria-selected={item.value === value}
              className={
                item.value === value
                  ? "flex h-7 w-full items-center rounded-sm bg-secondary px-2 text-left text-xs text-foreground"
                  : "flex h-7 w-full items-center rounded-sm px-2 text-left text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
              }
              onClick={() => {
                onChange(item.value);
                setOpen(false);
              }}
            >
              {item.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

"use client";

import type { AgentMode } from "@codedock/core/chat";
import {
  Button,
  PromptInput,
  PromptInputFooter,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputTools,
} from "@codedock/ui";
import { ChevronUp } from "lucide-react";
import { useEffect, useRef, useState } from "react";

const modes: { value: AgentMode; label: string }[] = [
  { value: "ask_for_approval", label: "manual" },
  { value: "auto_approve", label: "auto" },
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
  onSend: (text: string, mode: AgentMode) => Promise<void>;
  onCancel: () => Promise<void>;
}) {
  const [text, setText] = useState("");
  const [mode, setMode] = useState<AgentMode>("ask_for_approval");

  return (
    <div className="relative z-30 mx-auto w-full max-w-3xl px-4 pb-4">
      <PromptInput
        onSend={async (message) => {
          const next = message.text.trim();
          if (!next) {
            return;
          }
          setText("");
          await onSend(next, mode);
        }}
      >
        <PromptInputTextarea
          value={text}
          disabled={sending}
          placeholder="给 Agent 发消息…"
          onChange={(event) => setText(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              event.currentTarget.form?.requestSubmit();
            }
          }}
        />
        <PromptInputFooter>
          <PromptInputTools>
            <ModeMenu value={mode} onChange={setMode} />
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

function ModeMenu({
  value,
  onChange,
}: {
  value: AgentMode;
  onChange: (mode: AgentMode) => void;
}) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const current = modes.find((item) => item.value === value)?.label ?? "manual";

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
          {modes.map((item) => (
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

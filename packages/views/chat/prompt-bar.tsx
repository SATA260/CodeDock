"use client";

import type { ApprovalMode, WorkMode } from "@codedock/core/chat";
import {
  Button,
  cn,
  PromptInput,
  PromptInputFooter,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputTools,
  isImeConfirm,
} from "@codedock/ui";
import { ChevronUp } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import {
  DEFAULT_APPROVAL_MODE,
  DEFAULT_WORK_MODE,
  readLastApprovalMode,
  readLastWorkMode,
  writeLastApprovalMode,
  writeLastWorkMode,
} from "./lib/composer.ts";

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

// PromptBar 本地对话输入栏；工作模式和审批模式记在本机，刷新后还原。
export function PromptBar({
  running,
  sending,
  onSend,
  onCancel,
  className,
}: {
  running: boolean;
  sending: boolean;
  onSend: (text: string, mode: WorkMode, approval: ApprovalMode) => Promise<void>;
  onCancel: () => Promise<void>;
  className?: string;
}) {
  const [text, setText] = useState("");
  const [mode, setMode] = useState<WorkMode>(DEFAULT_WORK_MODE);
  const [approval, setApproval] = useState<ApprovalMode>(DEFAULT_APPROVAL_MODE);
  const [hydrated, setHydrated] = useState(false);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // 等 hydration 后再读本机缓存，避免 SSR 文本对不上。
  useEffect(() => {
    setMode(readLastWorkMode());
    setApproval(readLastApprovalMode());
    setHydrated(true);
  }, []);

  // rememberWorkMode 记下工作模式，刷新后还用这个。
  const rememberWorkMode = (next: WorkMode) => {
    setMode(next);
    writeLastWorkMode(next);
  };
  // rememberApprovalMode 记下审批模式，刷新后还用这个。
  const rememberApprovalMode = (next: ApprovalMode) => {
    setApproval(next);
    writeLastApprovalMode(next);
  };
  const keepFocus = () => {
    inputRef.current?.focus();
  };

  return (
    <div
      className={cn("relative z-30 mx-auto w-full max-w-3xl px-4 pb-4", className)}
      data-testid="composer"
      data-hydrated={hydrated ? "true" : "false"}
    >
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
          data-testid="composer-input"
          placeholder="给 Local 发消息…"
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
            <ChoiceMenu testid="work-mode" value={mode} options={workModes} onChange={rememberWorkMode} />
            <ChoiceMenu testid="approval-mode" value={approval} options={approvalModes} onChange={rememberApprovalMode} />
            {running ? (
              <Button size="sm" variant="outline" data-testid="composer-cancel" onClick={() => void onCancel()}>
                取消
              </Button>
            ) : null}
          </PromptInputTools>
          <PromptInputSubmit
            status={sending ? "streaming" : "ready"}
            disabled={sending || !text.trim()}
            data-testid="composer-send"
          />
        </PromptInputFooter>
      </PromptInput>
    </div>
  );
}

// ChoiceMenu 从底部弹出选项，选中后关掉。
function ChoiceMenu<T extends string>({
  value,
  options,
  onChange,
  testid,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
  testid: string;
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
        data-testid={testid}
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
              data-testid={`${testid}-${item.value}`}
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

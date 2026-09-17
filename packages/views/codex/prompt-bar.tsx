"use client";

import type { CommandSpec, InputMode, ModeInfo, ModelInfo, Settings, TokenUsage } from "@codedock/core/codex";
import {
  Button,
  PromptInput,
  PromptInputFooter,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputTools,
  isImeConfirm,
} from "@codedock/ui";
import { Plus } from "lucide-react";
import { useMemo, useRef, useState } from "react";

import type { PickedLocalFile, PickFilesOptions } from "../provider.tsx";
import { ModePermissionMenu, ModelEffortMenu } from "./composer-menus.tsx";
import { formatTokens, remainingContext } from "./lib/format.ts";

export function CodexPromptBar({
  running,
  sending,
  sessionId,
  workspace,
  pickFiles,
  commands,
  models,
  modes,
  settings,
  onSend,
  onCancel,
  onCommand,
  onAttach,
  onAttachError,
  onApply,
  onCompact,
  onRefreshCatalog,
  usage,
}: {
  running: boolean;
  sending: boolean;
  sessionId?: string;
  workspace?: string;
  pickFiles?: (options?: PickFilesOptions) => Promise<PickedLocalFile[]>;
  commands: CommandSpec[];
  models: ModelInfo[];
  modes: ModeInfo[];
  settings: Settings;
  onSend: (text: string, mode: InputMode) => Promise<void>;
  onCancel: () => Promise<void>;
  onCommand: (name: string, args: string) => Promise<void>;
  onAttach: (files: PickedLocalFile[]) => Promise<void>;
  onAttachError?: (message: string) => void;
  onApply: (patch: Settings) => void;
  onCompact: () => Promise<void>;
  onRefreshCatalog?: () => void;
  usage?: TokenUsage;
}) {
  const [text, setText] = useState("");
  const [queue, setQueue] = useState(false);
  const [attached, setAttached] = useState<PickedLocalFile[]>([]);
  const [picking, setPicking] = useState(false);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const keepFocus = () => {
    inputRef.current?.focus();
  };
  const slash = useMemo(() => {
    const trimmed = text.trim();
    if (!trimmed.startsWith("/")) {
      return [] as CommandSpec[];
    }
    const token = trimmed.slice(1).split(/\s/)[0] ?? "";
    return commands.filter((item) => item.name.startsWith(token));
  }, [commands, text]);

  return (
    <div className="relative z-30 mx-auto w-full max-w-3xl px-4 pb-4">
      {slash.length > 0 ? (
        <div className="mb-1 rounded-md border border-border bg-zinc-900 p-1 text-xs">
          {slash.map((item) => (
            <button
              key={item.name}
              type="button"
              className="flex w-full items-center justify-between rounded-sm px-2 py-1 text-left hover:bg-muted"
              onClick={() => {
                const args = text.trim().slice(item.name.length + 1).trim();
                void onCommand(item.name, args);
                setText("");
              }}
            >
              <span>/{item.name}</span>
              <span className="text-muted-foreground">{item.hint || item.action}</span>
            </button>
          ))}
        </div>
      ) : null}
      {attached.length > 0 ? (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {attached.map((file) => (
            <span
              key={file.path}
              className="max-w-full truncate rounded-md border border-border bg-muted px-2 py-0.5 font-mono text-[11px] text-muted-foreground"
              title={file.path}
            >
              {file.name}
            </span>
          ))}
        </div>
      ) : null}
      <PromptInput
        onSend={async (message) => {
          const next = message.text.trim();
          if (!next) {
            return;
          }
          if (next.startsWith("/")) {
            const [name, ...rest] = next.slice(1).split(/\s+/);
            if (name) {
              setText("");
              keepFocus();
              try {
                await onCommand(name, rest.join(" "));
              } finally {
                keepFocus();
              }
              return;
            }
          }
          setText("");
          setAttached([]);
          keepFocus();
          try {
            await onSend(next, queue || running ? "queue" : "start");
          } finally {
            keepFocus();
          }
        }}
      >
        <div className="flex items-start">
          <AttachButton
            disabled={picking || !pickFiles}
            picking={picking}
            onPick={async () => {
              if (!pickFiles || picking) {
                return;
              }
              setPicking(true);
              try {
                const files = await pickFiles({
                  multiple: true,
                  start: workspace?.trim() || settings.cwd,
                });
                if (files.length === 0) {
                  return;
                }
                await onAttach(files);
                setAttached((current) => mergeAttached(current, files));
              } catch (err) {
                onAttachError?.(err instanceof Error ? err.message : "挂文件失败");
              } finally {
                setPicking(false);
                keepFocus();
              }
            }}
          />
          <PromptInputTextarea
            ref={inputRef}
            value={text}
            autoFocus
            className="min-w-0 flex-1 pl-1"
            placeholder="给 Codex 发消息，或输入 / 选斜杠命令…"
            onChange={(event) => setText(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key !== "Enter" || event.shiftKey || isImeConfirm(event)) {
                return;
              }
              event.preventDefault();
              event.currentTarget.form?.requestSubmit();
            }}
          />
        </div>
        <PromptInputFooter className="flex-wrap">
          <PromptInputTools>
            <ModelEffortMenu models={models} settings={settings} onApply={onApply} onRefresh={onRefreshCatalog} />
            <ModePermissionMenu modes={modes} settings={settings} onApply={onApply} />
            <CompactButton disabled={!sessionId} usage={usage} onCompact={onCompact} />
            <label className="flex items-center gap-1 text-xs text-muted-foreground">
              <input type="checkbox" checked={queue} onChange={(event) => setQueue(event.target.checked)} />
              排队
            </label>
            {running ? (
              <Button size="sm" variant="outline" onClick={() => void onCancel()}>
                打断
              </Button>
            ) : null}
          </PromptInputTools>
          <PromptInputSubmit status={sending ? "streaming" : "ready"} disabled={sending || !text.trim()} />
        </PromptInputFooter>
      </PromptInput>
    </div>
  );
}

function CompactButton({
  disabled,
  usage,
  onCompact,
}: {
  disabled: boolean;
  usage?: TokenUsage;
  onCompact: () => Promise<void>;
}) {
  const remaining = remainingContext(usage);
  const label = remaining ? `压缩 ${remaining.percent}%` : "压缩";
  const title = remaining
    ? `还剩 ${formatTokens(remaining.left)} / ${formatTokens(remaining.window)}`
    : "压缩上下文";
  return (
    <Button size="sm" variant="outline" disabled={disabled} title={title} onClick={() => void onCompact()}>
      {label}
    </Button>
  );
}

function AttachButton({
  disabled,
  picking,
  onPick,
}: {
  disabled: boolean;
  picking: boolean;
  onPick: () => Promise<void>;
}) {
  return (
    <div className="shrink-0 pl-1.5 pt-1.5">
      <Button
        size="sm"
        variant="ghost"
        aria-label="挂载文件"
        className="size-7 px-0"
        disabled={disabled}
        onClick={() => void onPick()}
      >
        <Plus className={picking ? "size-4 animate-pulse" : "size-4"} />
      </Button>
    </div>
  );
}

function mergeAttached(current: PickedLocalFile[], next: PickedLocalFile[]): PickedLocalFile[] {
  const seen = new Set(current.map((file) => file.path));
  const extra = next.filter((file) => !seen.has(file.path));
  return extra.length === 0 ? current : [...current, ...extra];
}

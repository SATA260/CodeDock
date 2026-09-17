"use client";

import { cn } from "@codedock/ui";
import { FolderOpen } from "lucide-react";
import type { ReactNode } from "react";

export function NewConversation({
  brandSrc,
  codexIconSrc,
  engine,
  onEngine,
  workspaceLabel,
  workspaceTitle,
  picking,
  canPick,
  onPick,
  onClear,
  canClear,
  children,
}: {
  brandSrc?: string;
  codexIconSrc?: string;
  engine: "agent" | "codex";
  onEngine: (engine: "agent" | "codex") => void;
  workspaceLabel: string;
  workspaceTitle: string;
  picking: boolean;
  canPick: boolean;
  onPick: () => void;
  onClear: () => void;
  canClear: boolean;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center overflow-y-auto px-6 py-10">
      <div className="flex w-full max-w-2xl -translate-y-[7vh] flex-col items-center">
        {brandSrc ? (
          <img src={brandSrc} alt="" className="size-24 shrink-0" />
        ) : null}
        <h1 className="mt-4 text-2xl font-semibold tracking-tight text-foreground">CodeDock</h1>
        <p className="mt-2 text-xs tracking-[0.18em] text-muted-foreground/70">以issue驱动开发，把对话停在codedock</p>
        <div
          className="mt-3 flex items-center rounded-lg border border-border p-1"
          role="group"
          aria-label="会话模式"
        >
          <EngineChoice
            active={engine === "agent"}
            icon={brandSrc}
            label="Local"
            onClick={() => onEngine("agent")}
          />
          <EngineChoice
            active={engine === "codex"}
            icon={codexIconSrc}
            label="Codex"
            onClick={() => onEngine("codex")}
          />
        </div>

        <div className="mt-10 w-full">
          <div className="mb-2 flex w-full items-center gap-2 rounded-lg border border-border bg-accent/50 px-3 py-2">
            <button
              data-workspace-pick=""
              type="button"
              disabled={!canPick || picking}
              title={workspaceTitle || "打开系统目录选择框"}
              className="flex min-w-0 flex-1 items-center gap-2 text-left text-sm leading-5 text-foreground disabled:opacity-50"
              onClick={onPick}
            >
              <FolderOpen className="size-4 shrink-0 text-muted-foreground" />
              <span
                className="min-w-0 truncate font-mono text-xs"
                title={workspaceTitle}
                data-workspace-path={workspaceTitle}
              >
                {canClear ? workspaceLabel : "选择目录（默认仓库）"}
              </span>
            </button>
            {canClear ? (
              <button
                type="button"
                className="shrink-0 text-xs text-muted-foreground hover:text-foreground"
                onClick={onClear}
              >
                默认
              </button>
            ) : null}
          </div>
          {children}
        </div>
      </div>
    </div>
  );
}

function EngineChoice({
  active,
  icon,
  label,
  onClick,
}: {
  active: boolean;
  icon?: string;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      className={cn(
        "inline-flex h-9 items-center gap-2 rounded-md px-3 text-sm",
        active
          ? "bg-muted font-medium text-foreground"
          : "text-muted-foreground hover:text-foreground",
      )}
      onClick={onClick}
    >
      {icon ? <img src={icon} alt="" className={label === "Codex" ? "size-5 rounded-[4px]" : "size-5"} /> : null}
      {label}
    </button>
  );
}

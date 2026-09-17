"use client";

import type { ClaudeMode, ClaudeModel, ClaudeSettings, ClaudeSettingsPatch } from "@codedock/core/claude";
import { Button } from "@codedock/ui";
import { ChevronUp } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";

// ModelEffortMenu 在输入栏下方弹出模型与推理强度，点选即写入覆盖。
export function ModelEffortMenu({
  models,
  settings,
  onApply,
  onRefresh,
}: {
  models: ClaudeModel[];
  settings: ClaudeSettings;
  onApply: (patch: ClaudeSettingsPatch) => void;
  onRefresh?: () => void;
}) {
  const options = modelsWithCurrent(models, settings);
  const defaultModel = options.find((model) => model.is_default) ?? options[0];
  const modelId = settings.model || defaultModel?.id || "";
  const selected = options.find((model) => model.id === modelId);
  const efforts = selected?.efforts?.length
    ? selected.efforts
    : selected?.default_effort
      ? [selected.default_effort]
      : [];
  const effortId =
    [settings.effort, selected?.default_effort].find(
      (value) => Boolean(value) && (efforts.length === 0 || efforts.includes(value as string)),
    ) || "";
  const label = effortId ? `${selected?.id || "模型"} · ${effortId}` : selected?.id || "模型";

  return (
    <UpPopover label={label} ariaLabel="模型与推理强度" onOpen={onRefresh}>
      <MenuSection title="模型">
        {options.length === 0 ? <EmptyRow>没有可选模型</EmptyRow> : null}
        {options.map((model) => (
          <MenuOption
            key={model.id}
            selected={model.id === modelId}
            onSelect={() =>
              onApply({
                model: model.id,
                effort: model.default_effort || effortId || undefined,
              })
            }
          >
            {model.id}
          </MenuOption>
        ))}
      </MenuSection>
      <MenuSection title="推理强度">
        {efforts.length === 0 ? <EmptyRow>先选模型</EmptyRow> : null}
        {efforts.map((effort) => (
          <MenuOption key={effort} selected={effort === effortId} onSelect={() => onApply({ effort })}>
            {effort}
          </MenuOption>
        ))}
      </MenuSection>
    </UpPopover>
  );
}

// PermissionMenu 列出 Claude 官方权限档，点选即生效。
export function PermissionMenu({
  modes,
  settings,
  onApply,
}: {
  modes: ClaudeMode[];
  settings: ClaudeSettings;
  onApply: (patch: ClaudeSettingsPatch) => void;
}) {
  const permissions = modes.filter((mode) => !mode.kind || mode.kind === "permission");
  const modeId = settings.permission_mode || permissions[0]?.id || "";
  const current = permissions.find((mode) => mode.id === modeId);

  return (
    <UpPopover label={modeLabel(current?.id || modeId)} ariaLabel="权限档">
      <MenuSection title="权限">
        {permissions.length === 0 ? <EmptyRow>没有可选权限</EmptyRow> : null}
        {permissions.map((mode) => (
          <MenuOption
            key={mode.id}
            selected={mode.id === modeId}
            onSelect={() => onApply({ permission_mode: mode.id })}
          >
            {modeLabel(mode.id)}
          </MenuOption>
        ))}
      </MenuSection>
    </UpPopover>
  );
}

// UpPopover 从输入栏向上弹出点选菜单。
export function UpPopover({
  label,
  ariaLabel,
  onOpen,
  children,
}: {
  label: ReactNode;
  ariaLabel: string;
  onOpen?: () => void;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

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
        aria-label={ariaLabel}
        aria-expanded={open}
        aria-haspopup="listbox"
        onClick={() =>
          setOpen((current) => {
            const next = !current;
            if (next) {
              onOpen?.();
            }
            return next;
          })
        }
      >
        <span className="max-w-40 truncate">{label}</span>
        <ChevronUp className="size-3.5 shrink-0" />
      </Button>
      {open ? (
        <div
          className="absolute bottom-full left-0 z-50 mb-1 min-w-44 overflow-hidden rounded-md border border-border bg-zinc-900 p-1 shadow-lg"
          role="listbox"
        >
          {children}
        </div>
      ) : null}
    </div>
  );
}

// MenuSection 给弹出菜单加一组标题。
function MenuSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="py-0.5">
      <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-muted-foreground">{title}</div>
      {children}
    </div>
  );
}

// MenuOption 点选一项后立刻回调，不再二次确认。
function MenuOption({
  selected,
  onSelect,
  children,
}: {
  selected: boolean;
  onSelect: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="option"
      aria-selected={selected}
      className={
        selected
          ? "flex h-7 w-full items-center rounded-sm bg-secondary px-2 text-left text-xs text-foreground"
          : "flex h-7 w-full items-center rounded-sm px-2 text-left text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
      }
      onClick={onSelect}
    >
      {children}
    </button>
  );
}

// EmptyRow 菜单空态。
function EmptyRow({ children }: { children: ReactNode }) {
  return <div className="px-2 py-1 text-xs text-muted-foreground">{children}</div>;
}

// modelsWithCurrent 若生效模型不在目录里，仍把它摆在第一项。
function modelsWithCurrent(models: ClaudeModel[], settings: ClaudeSettings): ClaudeModel[] {
  const visible = models.filter((model) => !model.hidden || model.id === settings.model);
  const id = settings.model?.trim();
  if (!id || visible.some((model) => model.id === id)) {
    return visible;
  }
  return [
    {
      id,
      efforts: settings.effort ? [settings.effort] : [],
      default_effort: settings.effort || "medium",
      hidden: false,
      is_default: true,
    },
    ...visible.map((model) => ({ ...model, is_default: false })),
  ];
}

// modeLabel 用官方英文标签，和 Claude Code 模式指示器同一套。
function modeLabel(id: string): string {
  switch (id) {
    case "default":
    case "manual":
      return "Ask before edits";
    case "acceptEdits":
      return "Edit automatically";
    case "plan":
      return "Plan mode";
    case "auto":
      return "Auto mode";
    case "dontAsk":
      return "Don't ask";
    case "bypassPermissions":
      return "Bypass permissions";
    default:
      return id || "Ask before edits";
  }
}

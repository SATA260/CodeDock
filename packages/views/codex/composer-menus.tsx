"use client";

import type { ModeInfo, ModelInfo, Settings } from "@codedock/core/codex";
import { Button } from "@codedock/ui";
import { ChevronUp } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";

export function ModelEffortMenu({
  models,
  settings,
  onApply,
}: {
  models: ModelInfo[];
  settings: Settings;
  onApply: (patch: Settings) => void;
}) {
  const defaultModel = models.find((model) => model.is_default) ?? models[0];
  const modelId = settings.model || defaultModel?.id || "";
  const selected = models.find((model) => model.id === modelId);
  const efforts = selected?.efforts?.length
    ? selected.efforts
    : selected?.default_effort
      ? [selected.default_effort]
      : [];
  const effortId =
    [settings.effort, selected?.default_effort].find(
      (value) => Boolean(value) && (efforts.length === 0 || efforts.includes(value as string)),
    ) || "";
  const modelLabel = selected?.display_name || selected?.id || "模型";
  const label = effortId ? `${modelLabel} · ${effortId}` : modelLabel;

  return (
    <UpPopover label={label} ariaLabel="模型与推理强度">
      <MenuSection title="模型">
        {models.length === 0 ? <EmptyRow>没有可选模型</EmptyRow> : null}
        {models.map((model) => (
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
            {model.display_name || model.id}
          </MenuOption>
        ))}
      </MenuSection>
      <MenuSection title="推理强度">
        {efforts.length === 0 ? <EmptyRow>先选模型</EmptyRow> : null}
        {efforts.map((effort) => (
          <MenuOption
            key={effort}
            selected={effort === effortId}
            onSelect={() => onApply({ effort })}
          >
            {effort}
          </MenuOption>
        ))}
      </MenuSection>
    </UpPopover>
  );
}

export function ModePermissionMenu({
  modes,
  settings,
  onApply,
}: {
  modes: ModeInfo[];
  settings: Settings;
  onApply: (patch: Settings) => void;
}) {
  const plans = modes.filter((mode) => mode.kind === "collaboration");
  const permissions = modes.filter((mode) => mode.kind === "permission");
  const planId = settings.collaboration_mode || "default";
  const plan = plans.find((mode) => mode.id === planId);
  const permissionId = permissionValue(settings.sandbox, permissions);
  const permission = permissions.find((mode) => mode.id === permissionId);
  const planLabel = plan ? modeLabel(plan) : "模式";
  const permissionLabel = permission ? modeLabel(permission) : "权限";
  const label = permissionId ? `${planLabel} · ${permissionLabel}` : planLabel;

  return (
    <UpPopover label={label} ariaLabel="模式与权限">
      <MenuSection title="模式">
        {plans.length === 0 ? <EmptyRow>没有可选模式</EmptyRow> : null}
        {plans.map((mode) => (
          <MenuOption
            key={mode.id}
            selected={mode.id === planId}
            onSelect={() => onApply({ collaboration_mode: mode.id })}
          >
            {modeLabel(mode)}
          </MenuOption>
        ))}
      </MenuSection>
      <MenuSection title="权限">
        {permissions.length === 0 ? <EmptyRow>没有可选权限</EmptyRow> : null}
        {permissions.map((mode) => (
          <MenuOption
            key={mode.id}
            selected={mode.id === permissionId}
            disabled={!mode.allowed}
            onSelect={() =>
              onApply({
                sandbox: sandboxOf(mode) || mode.id,
                approval_policy: mode.approval,
              })
            }
          >
            {modeLabel(mode)}
          </MenuOption>
        ))}
      </MenuSection>
    </UpPopover>
  );
}

export function UpPopover({
  label,
  ariaLabel,
  align = "left",
  children,
}: {
  label: ReactNode;
  ariaLabel: string;
  align?: "left" | "right";
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
        onClick={() => setOpen((current) => !current)}
      >
        <span className="max-w-40 truncate">{label}</span>
        <ChevronUp className="size-3.5 shrink-0" />
      </Button>
      {open ? (
        <div
          className={
            align === "right"
              ? "absolute bottom-full right-0 z-50 mb-1 min-w-44 overflow-hidden rounded-md border border-border bg-zinc-900 p-1 shadow-lg"
              : "absolute bottom-full left-0 z-50 mb-1 min-w-44 overflow-hidden rounded-md border border-border bg-zinc-900 p-1 shadow-lg"
          }
          role="listbox"
        >
          {children}
        </div>
      ) : null}
    </div>
  );
}

function MenuSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="py-0.5">
      <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-muted-foreground">{title}</div>
      {children}
    </div>
  );
}

function MenuOption({
  selected,
  disabled,
  onSelect,
  children,
}: {
  selected: boolean;
  disabled?: boolean;
  onSelect: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="option"
      aria-selected={selected}
      disabled={disabled}
      className={
        selected
          ? "flex h-7 w-full items-center rounded-sm bg-secondary px-2 text-left text-xs text-foreground"
          : "flex h-7 w-full items-center rounded-sm px-2 text-left text-xs text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
      }
      onClick={onSelect}
    >
      {children}
    </button>
  );
}

function EmptyRow({ children }: { children: ReactNode }) {
  return <div className="px-2 py-1 text-xs text-muted-foreground">{children}</div>;
}

function modeLabel(mode: ModeInfo): string {
  return mode.label || mode.id.replace(/^:/, "") || mode.id;
}

function sandboxOf(mode?: ModeInfo): string {
  if (!mode) {
    return "";
  }
  if (mode.sandbox) {
    return mode.sandbox;
  }
  return mode.id.replace(/^:/, "");
}

function permissionValue(sandbox: string | undefined, permissions: ModeInfo[]): string {
  if (!sandbox) {
    return "";
  }
  const match = permissions.find((mode) => mode.id === sandbox || sandboxOf(mode) === sandbox);
  return match?.id ?? sandbox;
}

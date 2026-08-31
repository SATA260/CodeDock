"use client";

import type { PromptConfig } from "@codedock/core/git";
import { cn } from "@codedock/ui";
import { ChevronDownIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";

export function PromptPicker({
  prompt,
  disabled,
  onSelect,
  onSaveCustom,
}: {
  prompt: PromptConfig | null;
  disabled: boolean;
  onSelect: (selected: string) => Promise<void>;
  onSaveCustom: (custom: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [custom, setCustom] = useState(prompt?.custom ?? "");
  const root = useRef<HTMLDivElement>(null);
  const selected = prompt?.selected ?? "conventional";
  const label = prompt?.presets.find((item) => item.id === selected)?.name ?? "提示词";

  const persistCustom = () => {
    if (selected === "custom" && custom !== (prompt?.custom ?? "")) {
      void onSaveCustom(custom);
    }
  };

  const close = () => {
    persistCustom();
    setOpen(false);
  };

  useEffect(() => {
    setCustom(prompt?.custom ?? "");
  }, [prompt?.custom]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onPointer = (event: PointerEvent) => {
      if (root.current && !root.current.contains(event.target as Node)) {
        close();
      }
    };
    document.addEventListener("pointerdown", onPointer);
    return () => document.removeEventListener("pointerdown", onPointer);
  }, [open, custom, selected, prompt?.custom]);

  return (
    <div className="relative" ref={root}>
      <button
        type="button"
        disabled={disabled || !prompt}
        aria-expanded={open}
        className="inline-flex max-w-28 items-center gap-1 rounded-md border border-border px-1.5 py-0.5 text-xs hover:bg-accent/60 disabled:opacity-50"
        onClick={() => {
          if (open) {
            close();
            return;
          }
          setOpen(true);
        }}
      >
        <span className="truncate">{label}</span>
        <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
      </button>
      {open && prompt ? (
        <div className="absolute left-0 top-full z-20 mt-1 w-72 overflow-hidden border border-border bg-background shadow-lg">
          <div className="px-3 py-1.5 text-[11px] text-muted-foreground">生成说明用的提示词</div>
          <ul>
            {prompt.presets.map((preset) => {
              const active = preset.id === selected;
              return (
                <li key={preset.id}>
                  <button
                    type="button"
                    className={cn(
                      "flex w-full flex-col items-start gap-0.5 px-3 py-1.5 text-left hover:bg-accent/60",
                      active && "bg-muted",
                    )}
                    onClick={() => {
                      if (preset.id !== selected) {
                        persistCustom();
                        void onSelect(preset.id);
                      }
                      if (preset.id !== "custom") {
                        setOpen(false);
                      }
                    }}
                  >
                    <span className="text-sm">{preset.name}</span>
                    {preset.id !== "custom" && preset.system_prompt ? (
                      <span className="line-clamp-2 text-[11px] text-muted-foreground">{preset.system_prompt}</span>
                    ) : null}
                  </button>
                </li>
              );
            })}
          </ul>
          {selected === "custom" ? (
            <div className="border-t border-border p-2">
              <textarea
                className="min-h-16 w-full resize-none bg-transparent text-xs outline-none placeholder:text-muted-foreground"
                placeholder="自定义提示词，失焦后保存"
                value={custom}
                onChange={(event) => setCustom(event.target.value)}
                onBlur={() => {
                  if (custom !== (prompt.custom ?? "")) {
                    void onSaveCustom(custom);
                  }
                }}
              />
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

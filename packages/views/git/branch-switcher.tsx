"use client";

import type { Branch, BranchView } from "@codedock/core/git";
import { cn } from "@codedock/ui";
import { ChevronDownIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { shortHead } from "./lib/status.ts";

export function BranchSwitcher({
  view,
  busy,
  onSelectLocal,
  onSelectRemote,
}: {
  view: BranchView;
  busy: boolean;
  onSelectLocal: (name: string) => Promise<void>;
  onSelectRemote: (name: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const current = view.current || "游离 HEAD";

  useEffect(() => {
    if (!open) {
      return;
    }
    const onPointer = (event: PointerEvent) => {
      if (root.current && !root.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointer);
    return () => document.removeEventListener("pointerdown", onPointer);
  }, [open]);

  const pick = async (kind: "local" | "remote", name: string) => {
    if (busy) {
      return;
    }
    setOpen(false);
    if (kind === "local") {
      if (name === view.current) {
        return;
      }
      await onSelectLocal(name);
      return;
    }
    await onSelectRemote(name);
  };

  return (
    <div className="relative" ref={root}>
      <button
        type="button"
        disabled={busy}
        aria-expanded={open}
        className="inline-flex max-w-56 items-center gap-1 rounded-md border border-border px-1.5 py-0.5 font-mono text-[12px] hover:bg-accent/60 disabled:opacity-50"
        onClick={() => setOpen((prev) => !prev)}
      >
        <span className="truncate">{current}</span>
        <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
      </button>
      {open ? (
        <div className="absolute left-0 top-full z-20 mt-1 w-80 overflow-hidden border border-border bg-background shadow-lg">
          <BranchGroup
            title="本地分支"
            empty="还没有本地分支"
            items={view.locals}
            current={view.current}
            onPick={(name) => void pick("local", name)}
          />
          <BranchGroup
            title="远端分支"
            empty="没有 fetch 缓存"
            items={view.remotes}
            current={view.current}
            onPick={(name) => void pick("remote", name)}
          />
        </div>
      ) : null}
    </div>
  );
}

function BranchGroup({
  title,
  empty,
  items,
  current,
  onPick,
}: {
  title: string;
  empty: string;
  items: Branch[];
  current: string;
  onPick: (name: string) => void;
}) {
  return (
    <div className="border-b border-border last:border-b-0">
      <div className="px-3 py-1.5 text-[11px] text-muted-foreground">{title}</div>
      {items.length === 0 ? (
        <p className="px-3 pb-2 text-xs text-muted-foreground">{empty}</p>
      ) : (
        <ul className="max-h-56 overflow-y-auto">
          {items.map((branch) => {
            const active = !branch.is_remote && (branch.is_current || branch.name === current);
            return (
              <li key={branch.name}>
                <button
                  type="button"
                  className={cn(
                    "flex w-full flex-col items-start gap-0.5 px-3 py-1.5 text-left hover:bg-accent/60",
                    active && "bg-muted",
                  )}
                  onClick={() => onPick(branch.name)}
                >
                  <span className="flex w-full items-baseline gap-2">
                    <span className="truncate font-mono text-[13px]">{branch.name}</span>
                    {active ? (
                      <span className="shrink-0 text-[11px] text-muted-foreground">当前</span>
                    ) : null}
                  </span>
                  <span className="truncate text-[11px] text-muted-foreground">
                    {shortHead(branch.head)}
                    {branch.title ? ` · ${branch.title}` : ""}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

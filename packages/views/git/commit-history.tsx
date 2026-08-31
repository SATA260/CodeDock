"use client";

import type { Commit } from "@codedock/core/git";
import { cn } from "@codedock/ui";
import { ChevronDownIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { shortHead } from "./lib/status.ts";

export function CommitHistory({ commits, busy }: { commits: Commit[]; busy: boolean }) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const latest = commits[0];

  useEffect(() => {
    if (!open) {
      return;
    }
    const onPointer = (event: PointerEvent) => {
      if (root.current && !root.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="relative min-w-0" ref={root}>
      <button
        type="button"
        disabled={busy || commits.length === 0}
        aria-expanded={open}
        aria-haspopup="listbox"
        className="inline-flex max-w-72 items-center gap-1 rounded-md border border-border px-1.5 py-0.5 text-[12px] hover:bg-accent/60 disabled:opacity-50"
        onClick={() => setOpen((prev) => !prev)}
      >
        <span className="shrink-0 font-mono text-muted-foreground">
          {latest ? shortHead(latest.id) : "历史"}
        </span>
        {latest ? <span className="min-w-0 truncate">{latest.title}</span> : null}
        <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
      </button>
      {open ? (
        <div
          role="listbox"
          aria-label="提交历史"
          className="absolute left-0 top-full z-20 mt-1 w-96 overflow-hidden border border-border bg-background shadow-lg"
        >
          <div className="px-3 py-1.5 text-[11px] text-muted-foreground">提交历史</div>
          {commits.length === 0 ? (
            <p className="px-3 pb-2 text-xs text-muted-foreground">还没有提交</p>
          ) : (
            <ul className="max-h-72 overflow-y-auto">
              {commits.map((commit, index) => (
                <li
                  key={commit.id}
                  className={cn("flex flex-col gap-0.5 px-3 py-1.5", index === 0 && "bg-muted")}
                >
                  <span className="flex w-full items-baseline gap-2">
                    <span className="truncate text-[13px]">{commit.title}</span>
                    {index === 0 ? (
                      <span className="shrink-0 text-[11px] text-muted-foreground">HEAD</span>
                    ) : null}
                  </span>
                  <span className="truncate font-mono text-[11px] text-muted-foreground">
                    {shortHead(commit.id)}
                    {commit.author ? ` · ${commit.author}` : ""}
                    {commit.date ? ` · ${commit.date.slice(0, 10)}` : ""}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : null}
    </div>
  );
}

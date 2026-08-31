"use client";

import { Button, cn } from "@codedock/ui";
import { ChevronDownIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";

export function PublishActions({
  canCommit,
  canPush,
  busy,
  onPush,
}: {
  canCommit: boolean;
  canPush: boolean;
  busy: boolean;
  onPush: () => void;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);

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
    <div className="relative flex shrink-0" ref={root}>
      <Button type="submit" size="sm" disabled={!canCommit} className="rounded-r-none">
        Commit
      </Button>
      <Button
        type="button"
        size="sm"
        disabled={busy}
        aria-expanded={open}
        aria-haspopup="menu"
        aria-label="更多"
        className="rounded-l-none border-l border-l-primary-foreground/25 px-1.5"
        onClick={() => setOpen((prev) => !prev)}
      >
        <ChevronDownIcon className="size-3.5" />
      </Button>
      {open ? (
        <div
          role="menu"
          className="absolute right-0 top-full z-20 mt-1 min-w-28 overflow-hidden border border-border bg-background py-1 shadow-lg"
        >
          <MenuItem
            disabled={!canCommit}
            onClick={() => {
              setOpen(false);
            }}
            submit
          >
            Commit
          </MenuItem>
          <MenuItem
            disabled={!canPush}
            onClick={() => {
              setOpen(false);
              onPush();
            }}
          >
            Push
          </MenuItem>
        </div>
      ) : null}
    </div>
  );
}

function MenuItem({
  disabled,
  submit,
  onClick,
  children,
}: {
  disabled?: boolean;
  submit?: boolean;
  onClick: () => void;
  children: string;
}) {
  return (
    <button
      type={submit ? "submit" : "button"}
      role="menuitem"
      disabled={disabled}
      className={cn(
        "flex w-full px-3 py-1.5 text-left text-sm hover:bg-accent/60 disabled:pointer-events-none disabled:opacity-40",
      )}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

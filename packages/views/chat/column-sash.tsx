"use client";

import { useRef } from "react";

import { cn } from "@codedock/ui";

// ColumnSash 竖向拖条，拖动时连续回调位移，松开时记住宽度；双击收起该侧。
export function ColumnSash({
  onMove,
  onCollapse,
  label,
}: {
  onMove: (deltaX: number, persist: boolean) => void;
  onCollapse?: () => void;
  label: string;
}) {
  const lastX = useRef(0);
  const dragging = useRef(false);
  const moveRef = useRef(onMove);
  moveRef.current = onMove;

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      tabIndex={0}
      className={cn(
        "group relative z-20 w-1.5 shrink-0 cursor-col-resize bg-border",
        "hover:bg-foreground/25 focus-visible:bg-foreground/25",
        "active:bg-foreground/40",
      )}
      onPointerDown={(event) => {
        if (event.button !== 0) {
          return;
        }
        dragging.current = true;
        lastX.current = event.clientX;
        document.documentElement.classList.add("select-none", "cursor-col-resize");
        const onMovePointer = (next: PointerEvent) => {
          if (!dragging.current) {
            return;
          }
          const delta = next.clientX - lastX.current;
          lastX.current = next.clientX;
          if (delta !== 0) {
            moveRef.current(delta, false);
          }
        };
        const onUp = () => {
          if (!dragging.current) {
            return;
          }
          dragging.current = false;
          document.documentElement.classList.remove("select-none", "cursor-col-resize");
          window.removeEventListener("pointermove", onMovePointer);
          window.removeEventListener("pointerup", onUp);
          window.removeEventListener("pointercancel", onUp);
          moveRef.current(0, true);
        };
        window.addEventListener("pointermove", onMovePointer);
        window.addEventListener("pointerup", onUp);
        window.addEventListener("pointercancel", onUp);
        event.preventDefault();
      }}
      onDoubleClick={() => {
        onCollapse?.();
      }}
      onKeyDown={(event) => {
        if (event.key === "ArrowLeft") {
          event.preventDefault();
          onMove(-16, true);
        }
        if (event.key === "ArrowRight") {
          event.preventDefault();
          onMove(16, true);
        }
      }}
    >
      <span className="sr-only">{label}</span>
      <span className="pointer-events-none absolute inset-y-0 -left-1 -right-1" />
    </div>
  );
}

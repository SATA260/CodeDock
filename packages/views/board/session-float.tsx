"use client";

import type { BoardEngine } from "@codedock/core/board";
import type { TimelineItem } from "@codedock/core/chat";
import { Button } from "@codedock/ui";
import { Settings, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";

import { ApprovalDock } from "../chat/approval-dock.tsx";
import type { SessionEngine } from "../chat/chat-page.tsx";
import { ConversationTimeline } from "../chat/conversation-timeline.tsx";
import { useSessionTimeline } from "../chat/hooks/use-session-timeline.ts";
import { PendingDock } from "../chat/pending-dock.tsx";
import { PromptBar } from "../chat/prompt-bar.tsx";
import { ClaudePane } from "../claude/claude-pane.tsx";
import { CodexPane } from "../codex/codex-pane.tsx";
import { useAgent } from "../provider.tsx";
import { SessionLinkEditor } from "./session-links.tsx";

export type FloatSession = {
  id: string;
  engine: SessionEngine;
};

const MIN_W = 320;
const MIN_H = 240;

type FloatBox = { x: number; y: number; w: number; h: number };

type ResizeEdge = { n?: boolean; s?: boolean; e?: boolean; w?: boolean };

type Gesture = {
  kind: "move" | "resize";
  edge?: ResizeEdge;
  x: number;
  y: number;
  box: FloatBox;
};

// SessionFloat 看板不退时用可拖悬浮窗看完整对话；无遮罩，关掉前始终浮在壳上。
export function SessionFloat({
  session,
  onClose,
  onOpenPlan,
  onOpenFile,
  onOpenGit,
}: {
  session: FloatSession;
  onClose: () => void;
  onOpenPlan: Parameters<typeof ConversationTimeline>[0]["onOpenPlan"];
  onOpenFile: Parameters<typeof ConversationTimeline>[0]["onOpenFile"];
  onOpenGit: () => void;
}) {
  const { pickFiles } = useAgent();
  const timeline = useSessionTimeline(session.engine === "agent" ? session.id : undefined);
  const pendingApprovals = useMemo(
    () =>
      timeline.state.items.filter(
        (item): item is Extract<TimelineItem, { kind: "approval" }> =>
          item.kind === "approval" && item.status === "pending",
      ),
    [timeline.state.items],
  );
  const [box, setBox] = useState<FloatBox>({ x: 72, y: 72, w: 560, h: 640 });
  const [linksOpen, setLinksOpen] = useState(false);
  const gesture = useRef<Gesture | null>(null);

  useEffect(() => {
    setLinksOpen(false);
  }, [session.id, session.engine]);

  useEffect(() => {
    const onMove = (event: PointerEvent) => {
      const current = gesture.current;
      if (!current) {
        return;
      }
      const dx = event.clientX - current.x;
      const dy = event.clientY - current.y;
      if (current.kind === "move") {
        setBox(clampBox({ ...current.box, x: current.box.x + dx, y: current.box.y + dy }));
        return;
      }
      setBox(resizeBox(current.box, current.edge ?? {}, dx, dy));
    };
    const onUp = () => {
      gesture.current = null;
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
  }, []);

  // beginMove 从标题栏拖动整窗，点到按钮和输入时不拖。
  const beginMove = (event: ReactPointerEvent) => {
    if ((event.target as HTMLElement).closest("button, input, a, textarea, select")) {
      return;
    }
    gesture.current = { kind: "move", x: event.clientX, y: event.clientY, box };
  };

  // beginResize 从边或角改宽高，西/北侧同时挪位置。
  const beginResize = (edge: ResizeEdge) => (event: ReactPointerEvent) => {
    event.preventDefault();
    event.stopPropagation();
    gesture.current = { kind: "resize", edge, x: event.clientX, y: event.clientY, box };
  };

  return (
    <div
      className="fixed z-40 flex flex-col overflow-hidden rounded-lg border border-border bg-background shadow-2xl"
      style={{ left: box.x, top: box.y, width: box.w, height: box.h }}
    >
      <ResizeHandle label="调整上边" className="absolute inset-x-2 top-0 z-50 h-1.5 cursor-ns-resize" onPointerDown={beginResize({ n: true })} />
      <ResizeHandle label="调整下边" className="absolute inset-x-2 bottom-0 z-50 h-1.5 cursor-ns-resize" onPointerDown={beginResize({ s: true })} />
      <ResizeHandle label="调整左边" className="absolute inset-y-2 left-0 z-50 w-1.5 cursor-ew-resize" onPointerDown={beginResize({ w: true })} />
      <ResizeHandle label="调整右边" className="absolute inset-y-2 right-0 z-50 w-1.5 cursor-ew-resize" onPointerDown={beginResize({ e: true })} />
      <ResizeHandle label="调整左上角" className="absolute left-0 top-0 z-50 size-3 cursor-nwse-resize" onPointerDown={beginResize({ n: true, w: true })} />
      <ResizeHandle label="调整右上角" className="absolute right-0 top-0 z-50 size-3 cursor-nesw-resize" onPointerDown={beginResize({ n: true, e: true })} />
      <ResizeHandle label="调整左下角" className="absolute bottom-0 left-0 z-50 size-3 cursor-nesw-resize" onPointerDown={beginResize({ s: true, w: true })} />
      <ResizeHandle label="调整右下角" className="absolute bottom-0 right-0 z-50 size-3 cursor-nwse-resize" onPointerDown={beginResize({ s: true, e: true })} />
      <header
        className="flex shrink-0 cursor-grab items-center gap-2 border-b border-border px-2 py-1.5 active:cursor-grabbing"
        onPointerDown={beginMove}
      >
        <p className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
          {session.engine === "codex" ? "Codex" : session.engine === "claude" ? "Claude" : "Local"} · {session.id}
        </p>
        <Button
          size="sm"
          variant="ghost"
          className="px-1.5"
          aria-label="设置链接"
          aria-pressed={linksOpen}
          onClick={() => setLinksOpen((open) => !open)}
        >
          <Settings className="size-3.5" />
        </Button>
        <Button size="sm" variant="ghost" className="px-1.5" aria-label="关闭对话" onClick={onClose}>
          <X className="size-3.5" />
        </Button>
      </header>
      {linksOpen ? <SessionLinkEditor engine={session.engine as BoardEngine} sessionId={session.id} /> : null}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {session.engine === "codex" ? (
          <CodexPane
            sessionId={session.id}
            pickFiles={pickFiles}
            onOpenSession={() => undefined}
            onNewConversation={() => undefined}
            onListChange={async () => undefined}
          />
        ) : session.engine === "claude" ? (
          <ClaudePane
            sessionId={session.id}
            pickFiles={pickFiles}
            onOpenSession={() => undefined}
            onNewConversation={() => undefined}
            onListChange={async () => undefined}
          />
        ) : (
          <>
            {timeline.error ? (
              <div className="border-b border-destructive/30 bg-destructive/10 px-3 py-1.5 text-xs text-red-300">
                {timeline.error}
              </div>
            ) : null}
            <ConversationTimeline
              state={timeline.state}
              loading={timeline.loading}
              scrollKey={session.id}
              onOpenPlan={onOpenPlan}
              onOpenFile={onOpenFile}
              onOpenGit={onOpenGit}
            />
            <div className="relative z-30 shrink-0">
              <PendingDock
                items={timeline.pending}
                editingId={timeline.editingId}
                onBeginEdit={timeline.beginEditPending}
                onCancelEdit={timeline.cancelEditPending}
                onSave={timeline.savePending}
                onDelete={timeline.deletePending}
                onSendNow={timeline.sendNow}
              />
              <ApprovalDock items={pendingApprovals} onDecide={timeline.decide} />
              <PromptBar
                running={timeline.running}
                sending={timeline.sending}
                onSend={timeline.send}
                onCancel={timeline.cancel}
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}

// ResizeHandle 是悬浮窗边上的拖拽热区。
function ResizeHandle({
  label,
  className,
  onPointerDown,
}: {
  label: string;
  className: string;
  onPointerDown: (event: ReactPointerEvent<HTMLDivElement>) => void;
}) {
  return (
    <div
      role="separator"
      aria-label={label}
      className={className}
      onPointerDown={onPointerDown}
    />
  );
}

// resizeBox 按拖动方向改宽高；西、北侧同时移动原点。
function resizeBox(start: FloatBox, edge: ResizeEdge, dx: number, dy: number): FloatBox {
  let { x, y, w, h } = start;
  if (edge.e) {
    w = start.w + dx;
  }
  if (edge.s) {
    h = start.h + dy;
  }
  if (edge.w) {
    w = start.w - dx;
    x = start.x + dx;
  }
  if (edge.n) {
    h = start.h - dy;
    y = start.y + dy;
  }
  if (w < MIN_W) {
    if (edge.w) {
      x -= MIN_W - w;
    }
    w = MIN_W;
  }
  if (h < MIN_H) {
    if (edge.n) {
      y -= MIN_H - h;
    }
    h = MIN_H;
  }
  return clampBox({ x, y, w, h });
}

// clampBox 把窗限制在视口内，并守住最小宽高。
function clampBox(box: FloatBox): FloatBox {
  const maxW = Math.max(MIN_W, window.innerWidth - 16);
  const maxH = Math.max(MIN_H, window.innerHeight - 16);
  const w = Math.min(Math.max(MIN_W, box.w), maxW);
  const h = Math.min(Math.max(MIN_H, box.h), maxH);
  const x = Math.min(Math.max(8, box.x), Math.max(8, window.innerWidth - w - 8));
  const y = Math.min(Math.max(8, box.y), Math.max(8, window.innerHeight - h - 8));
  return { x, y, w, h };
}

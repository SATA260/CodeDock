"use client";

import { fileKey, type FileChangePreview, type PlanPreview } from "@codedock/core/chat";
import { Button, cn } from "@codedock/ui";
import { FileCode2, FileText, GitBranch, Plus, X } from "lucide-react";
import { useState } from "react";

import { GitPage } from "../git/git-page.tsx";
import { FileDiffPane } from "./file-diff.tsx";
import { PlanDocBody } from "./plan-preview.tsx";
import {
  DOCK_KINDS,
  type DockKind,
  type DockWindow,
  type FileWindow,
  type PlanWindow,
} from "./workbench.ts";

// SideDock 是对话右侧的多窗口栏：Plan、单个文件正文、Git，可新建和关闭。
export function SideDock({
  windows,
  activeId,
  plans,
  files,
  onSelect,
  onClose,
  onCreate,
  width,
}: {
  windows: DockWindow[];
  activeId: string | null;
  plans: PlanPreview[];
  files: FileChangePreview[];
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onCreate: (kind: DockKind, seed?: { plan?: PlanPreview; file?: FileChangePreview }) => void;
  width?: number;
}) {
  const active = windows.find((item) => item.id === activeId) ?? windows[windows.length - 1] ?? null;

  return (
    <aside
      data-testid="dock"
      className="flex h-full shrink-0 flex-col bg-background"
      style={width ? { width, minWidth: width, maxWidth: width } : { width: "38%" }}
    >
      <DockBar
        windows={windows}
        activeId={active?.id ?? null}
        plans={plans}
        files={files}
        onSelect={onSelect}
        onClose={onClose}
        onCreate={onCreate}
      />
      <div className="min-h-0 flex-1 overflow-hidden">
        {active ? <DockBody window={active} files={files} /> : <DockEmpty />}
      </div>
    </aside>
  );
}

// DockBar 一行窗口标签，右侧加号新建指定窗口。
function DockBar({
  windows,
  activeId,
  plans,
  files,
  onSelect,
  onClose,
  onCreate,
}: {
  windows: DockWindow[];
  activeId: string | null;
  plans: PlanPreview[];
  files: FileChangePreview[];
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onCreate: (kind: DockKind, seed?: { plan?: PlanPreview; file?: FileChangePreview }) => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="flex h-10 shrink-0 items-center gap-0.5 border-b border-border px-1">
      <div className="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden">
        {windows.map((window) => (
          <DockTab
            key={window.id}
            window={window}
            active={window.id === activeId}
            onSelect={() => onSelect(window.id)}
            onClose={() => onClose(window.id)}
          />
        ))}
      </div>
      <div className="relative flex shrink-0 items-center">
        <Button
          size="sm"
          variant="ghost"
          aria-expanded={menuOpen}
          aria-haspopup="menu"
          title="新建窗口"
          onClick={() => setMenuOpen((open) => !open)}
        >
          <Plus className="size-3.5" />
        </Button>
        {menuOpen ? (
          <>
            <button
              type="button"
              className="fixed inset-0 z-30 cursor-default"
              aria-label="关闭新建窗口菜单"
              onClick={() => setMenuOpen(false)}
            />
          <div
            role="menu"
            className="absolute right-0 top-full z-40 mt-1 min-w-40 rounded-md border border-border bg-card py-1 shadow-lg"
          >
            {DOCK_KINDS.map((item) => (
              <button
                key={item.kind}
                type="button"
                role="menuitem"
                className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-foreground hover:bg-accent"
                onClick={() => {
                  onCreate(
                    item.kind,
                    item.kind === "plan"
                      ? { plan: plans[0] }
                      : item.kind === "file"
                        ? { file: files[0] }
                        : undefined,
                  );
                  setMenuOpen(false);
                }}
              >
                <KindIcon kind={item.kind} />
                {item.label}
              </button>
            ))}
          </div>
          </>
        ) : null}
      </div>
    </div>
  );
}

// DockTab 单个窗口标签，点选中，叉关闭。
function DockTab({
  window,
  active,
  onSelect,
  onClose,
}: {
  window: DockWindow;
  active: boolean;
  onSelect: () => void;
  onClose: () => void;
}) {
  return (
    <div
      className={cn(
        "flex max-w-[11rem] shrink-0 items-center rounded-md",
        active ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-accent",
      )}
    >
      <button
        type="button"
        className="flex min-w-0 items-center gap-1.5 px-2 py-1 text-xs"
        onClick={onSelect}
      >
        <KindIcon kind={window.kind} />
        <span className="truncate">{window.title}</span>
      </button>
      <button
        type="button"
        className="mr-1 rounded p-0.5 hover:bg-background hover:text-foreground"
        aria-label={`关闭 ${window.title}`}
        onClick={(event) => {
          event.stopPropagation();
          onClose();
        }}
      >
        <X className="size-3" />
      </button>
    </div>
  );
}

// DockBody 按窗口种类渲染 Plan、文件或 Git。
function DockBody({
  window,
  files,
}: {
  window: DockWindow;
  files: FileChangePreview[];
}) {
  if (window.kind === "git") {
    return <GitPage variant="dock" />;
  }
  if (window.kind === "plan") {
    return <PlanPane window={window} />;
  }
  return <FilePane window={window} files={files} />;
}

// PlanPane 在右侧展开整篇计划。
function PlanPane({ window }: { window: PlanWindow }) {
  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="dock-window-plan">
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3 text-xs text-muted-foreground">
        <FileText className="size-3.5" />
        <span className="truncate font-mono text-foreground">{window.name || "未命名计划"}</span>
        {window.toolState ? <span className="ml-auto">{window.toolState}</span> : null}
      </div>
      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        {window.error ? <p className="mb-2 text-xs text-destructive">{window.error}</p> : null}
        <PlanDocBody
          content={window.content}
          emptyHint={window.name ? "这篇计划还没有正文" : "从对话里点开 Plan，或等模型写出计划"}
          animating={window.toolState === "pending" || window.toolState === "running"}
        />
      </div>
    </div>
  );
}

// FilePane 只展示当前点开的那一个文件的正文。
function FilePane({
  window,
  files,
}: {
  window: FileWindow;
  files: FileChangePreview[];
}) {
  const selected = files.find((file) => fileKey(file) === window.selectedKey) ?? null;
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden" data-testid="dock-window-file">
      <FileDiffPane file={selected} />
    </div>
  );
}

// DockEmpty 还没有窗口时的提示。
function DockEmpty() {
  return (
    <div className="flex h-full flex-col justify-center gap-2 px-6 text-sm text-muted-foreground">
      <p>右侧用来看 Plan、会话改过的文件和 Git。</p>
      <p className="text-xs">点对话里的 Plan 或文件，或用右上角 + 新建窗口。</p>
    </div>
  );
}

// KindIcon 窗口种类小图标。
function KindIcon({ kind }: { kind: DockKind }) {
  const className = "size-3.5 shrink-0";
  if (kind === "git") {
    return <GitBranch className={className} />;
  }
  if (kind === "plan") {
    return <FileText className={className} />;
  }
  return <FileCode2 className={className} />;
}


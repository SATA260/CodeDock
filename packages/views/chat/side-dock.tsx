"use client";

import type { FileChangePreview, PlanPreview } from "@codedock/core/chat";
import { Button, MessageResponse, cn } from "@codedock/ui";
import { FileCode2, FileText, GitBranch, Plus, X } from "lucide-react";
import { useState } from "react";

import { GitPage } from "../git/git-page.tsx";
import { PlanDocBody } from "./plan-preview.tsx";
import {
  DOCK_KINDS,
  type DockKind,
  type DockWindow,
  type FileWindow,
  type PlanWindow,
} from "./workbench.ts";

// SideDock 是对话右侧的多窗口栏：Plan、文件详情、Git，可新建和关闭。
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
        {active ? <DockBody window={active} /> : <DockEmpty />}
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
function DockBody({ window }: { window: DockWindow }) {
  if (window.kind === "git") {
    return <GitPage variant="dock" />;
  }
  if (window.kind === "plan") {
    return <PlanPane window={window} />;
  }
  return <FilePane window={window} />;
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

// FilePane 显示一次 write/edit 的文件详情。
function FilePane({ window }: { window: FileWindow }) {
  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="dock-window-file">
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3 text-xs text-muted-foreground">
        <FileCode2 className="size-3.5" />
        <span className="truncate font-mono text-foreground">{window.path || "未打开文件"}</span>
        <span className="ml-auto">{window.action === "edit" ? "编辑" : "写入"}</span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {window.content.trim() ? (
          window.action === "edit" && window.content.includes("@@ edit") ? (
            <pre className="whitespace-pre-wrap break-words px-4 py-3 font-mono text-xs leading-5 text-foreground">
              {window.content}
            </pre>
          ) : (
            <div className="px-4 py-3">
              <MessageResponse className="text-sm">{fence(window.path, window.content)}</MessageResponse>
            </div>
          )
        ) : (
          <p className="px-4 py-3 text-xs text-muted-foreground">
            {window.path ? "这次改动没有带上正文" : "从对话里点开文件，或用 + 打开 Git 看工作区"}
          </p>
        )}
      </div>
    </div>
  );
}

// DockEmpty 还没有窗口时的提示。
function DockEmpty() {
  return (
    <div className="flex h-full flex-col justify-center gap-2 px-6 text-sm text-muted-foreground">
      <p>右侧用来看 Plan、文件详情和 Git。</p>
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

// fence 把文件正文包进 markdown 代码块，按扩展名着色。
function fence(path: string, content: string): string {
  const ext = path.split(".").pop()?.toLowerCase() ?? "";
  const lang =
    ext === "py" ? "python" : ext === "ts" || ext === "tsx" ? "ts" : ext === "js" || ext === "jsx" ? "js" : ext;
  return `\`\`\`${lang}\n${content}\n\`\`\``;
}

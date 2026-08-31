"use client";

import type { FileStatus } from "@codedock/core/git";
import { cn } from "@codedock/ui";
import { ChevronDownIcon, ChevronRightIcon, File, Folder, FolderOpen, Minus, Plus, Undo2 } from "lucide-react";
import { useMemo, useState } from "react";

import { isPreviewablePath } from "./lib/preview.ts";
import { buildFileTree, collectFilePaths, type FileTreeNode } from "./lib/tree.ts";

const ROW_PAD = 10;
const DEPTH_STEP = 20;

export type TreeAction = {
  kind: "stage" | "unstage";
  onRun: (paths: string[]) => void;
  onDiscard?: (info: { name: string; paths: string[] }) => void;
};

export function FileTree({
  files,
  labelOf,
  empty,
  activePath,
  busy,
  action,
  onPreview,
}: {
  files: FileStatus[];
  labelOf: (file: FileStatus) => string;
  empty: string;
  activePath?: string | null;
  busy?: boolean;
  action?: TreeAction;
  onPreview?: (path: string) => void;
}) {
  const tree = useMemo(() => buildFileTree(files), [files]);
  if (files.length === 0) {
    return <p className="px-3 py-3 text-xs text-muted-foreground">{empty}</p>;
  }
  return (
    <ul>
      {tree.map((node) => (
        <TreeNode
          key={node.path}
          node={node}
          depth={0}
          activePath={activePath}
          busy={busy}
          labelOf={labelOf}
          action={action}
          onPreview={onPreview}
        />
      ))}
    </ul>
  );
}

function TreeNode({
  node,
  depth,
  activePath,
  busy,
  labelOf,
  action,
  onPreview,
}: {
  node: FileTreeNode;
  depth: number;
  activePath?: string | null;
  busy?: boolean;
  labelOf: (file: FileStatus) => string;
  action?: TreeAction;
  onPreview?: (path: string) => void;
}) {
  const [open, setOpen] = useState(true);
  const paths = collectFilePaths(node);
  const file = node.file;
  const expandable = !file || node.children.length > 0;
  const previewable = Boolean(file && isPreviewablePath(file.path));
  const active = Boolean(file && activePath === file.path);
  const padding = ROW_PAD + depth * DEPTH_STEP;

  return (
    <li>
      <div
        role={previewable || expandable ? "button" : undefined}
        tabIndex={previewable || expandable ? 0 : undefined}
        aria-expanded={expandable ? open : undefined}
        className={cn(
          "group relative flex items-center gap-1.5 py-0.5 pr-2 text-sm hover:bg-accent/60",
          active && "bg-accent before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-zinc-400",
          (previewable || expandable) && "cursor-pointer",
        )}
        style={{ paddingLeft: padding }}
        onKeyDown={(event) => {
          if (event.key !== "Enter" && event.key !== " ") {
            return;
          }
          event.preventDefault();
          if (expandable) {
            setOpen((prev) => !prev);
            return;
          }
          if (previewable && file) {
            onPreview?.(file.path);
          }
        }}
        onClick={() => {
          if (expandable) {
            setOpen((prev) => !prev);
            return;
          }
          if (previewable && file) {
            onPreview?.(file.path);
          }
        }}
      >
        {expandable ? (
          <span className="flex size-4 shrink-0 items-center justify-center text-muted-foreground" aria-hidden>
            {open ? <ChevronDownIcon className="size-3.5" /> : <ChevronRightIcon className="size-3.5" />}
          </span>
        ) : (
          <span className="size-4 shrink-0" aria-hidden />
        )}
        {previewable ? (
          <File className="size-3.5 shrink-0 text-muted-foreground" />
        ) : open && expandable ? (
          <FolderOpen className="size-3.5 shrink-0 text-amber-500/80" />
        ) : (
          <Folder className="size-3.5 shrink-0 text-amber-500/80" />
        )}
        <span className="min-w-0 flex-1">
          <span className="block truncate font-mono text-[13px]">{node.name}</span>
          {file?.orig_path ? (
            <span className="block truncate text-[11px] text-muted-foreground">来自 {file.orig_path}</span>
          ) : null}
        </span>
        {file ? (
          <span className="shrink-0 text-[11px] text-muted-foreground">{labelOf(file)}</span>
        ) : (
          <span className="shrink-0 text-[11px] text-muted-foreground">{paths.length}</span>
        )}
        {action ? (
          <button
            type="button"
            aria-label={action.kind === "stage" ? `暂存 ${node.name}` : `取消暂存 ${node.name}`}
            disabled={busy || paths.length === 0}
            className="flex size-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
            onClick={(event) => {
              event.stopPropagation();
              action.onRun(paths);
            }}
          >
            {action.kind === "stage" ? <Plus className="size-3.5" /> : <Minus className="size-3.5" />}
          </button>
        ) : null}
        {action?.onDiscard ? (
          <button
            type="button"
            aria-label={`撤回 ${node.name}`}
            disabled={busy || paths.length === 0}
            className="flex size-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
            onClick={(event) => {
              event.stopPropagation();
              action.onDiscard?.({ name: node.name, paths });
            }}
          >
            <Undo2 className="size-3.5" />
          </button>
        ) : null}
      </div>
      {expandable && open ? (
        <ul>
          {node.children.map((child) => (
            <TreeNode
              key={child.path}
              node={child}
              depth={depth + 1}
              activePath={activePath}
              busy={busy}
              labelOf={labelOf}
              action={action}
              onPreview={onPreview}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

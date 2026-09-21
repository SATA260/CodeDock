"use client";

import {
  fileChangeFromTool,
  fileKey,
  planPreviewFromTool,
  type FileChangePreview,
  type PlanPreview,
  type RunFileGroup,
  type TimelineItem,
  type ToolItemState,
} from "@codedock/core/chat";
import { useCallback, useState } from "react";

import { FILES_WINDOW_ID, nextFilesWindows, type FileWindow } from "./lib/files-window.ts";

export type { FileWindow };
export { FILES_WINDOW_ID };

export type DockKind = "plan" | "file" | "git";

export type PlanWindow = {
  id: string;
  kind: "plan";
  title: string;
  name: string;
  content: string;
  toolState?: ToolItemState;
  error?: string;
};

export type GitWindow = {
  id: string;
  kind: "git";
  title: string;
};

export type DockWindow = PlanWindow | FileWindow | GitWindow;

export const DOCK_KINDS: { kind: DockKind; label: string }[] = [
  { kind: "plan", label: "Plan" },
  { kind: "file", label: "文件" },
  { kind: "git", label: "Git" },
];

export const GIT_WINDOW_ID = "git";

const EMPTY_FILE: FileChangePreview = { path: "", content: "", action: "write", patch: "", runId: "" };

export type TitledRunFileGroup = RunFileGroup & { title: string };

// planWindowId 用计划名做窗口键，同名复用。
export function planWindowId(name: string): string {
  return `plan:${name || "untitled"}`;
}

// useWorkbench 管理右侧多窗口：打开、刷新已打开的、关闭、切到指定窗口。
export function useWorkbench() {
  const [windows, setWindows] = useState<DockWindow[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);

  // upsert 写入或更新一个窗口，并切到它。
  const upsert = useCallback((window: DockWindow) => {
    setWindows((current) => {
      const index = current.findIndex((item) => item.id === window.id);
      if (index < 0) {
        return [...current, window];
      }
      const next = current.slice();
      next[index] = { ...current[index], ...window } as DockWindow;
      return next;
    });
    setActiveId(window.id);
  }, []);

  // openPlan 打开或刷新指定计划窗口。
  const openPlan = useCallback(
    (preview: PlanPreview, extra?: { toolState?: ToolItemState; error?: string }) => {
      upsert({
        id: planWindowId(preview.name),
        kind: "plan",
        title: preview.name || "Plan",
        name: preview.name,
        content: preview.content,
        toolState: extra?.toolState,
        error: extra?.error,
      });
    },
    [upsert],
  );

  // openFile 打开或切到文件窗口，并选中这条路径。
  const openFile = useCallback(
    (change: FileChangePreview) => {
      upsert({
        id: FILES_WINDOW_ID,
        kind: "file",
        title: fileBasename(change.path) || "文件",
        selectedKey: fileKey(change),
      });
    },
    [upsert],
  );

  // selectFile 只改列表选中项，不切窗口。
  const selectFile = useCallback((file: FileChangePreview) => {
    setWindows((current) =>
      current.map((item) => (item.kind === "file" ? { ...item, selectedKey: fileKey(file) } : item)),
    );
  }, []);

  // syncFiles 把时间线上的文件同步进已打开的文件窗口；create 时在还没有窗口时补开。
  const syncFiles = useCallback((files: FileChangePreview[], create: boolean) => {
    setWindows((current) => nextFilesWindows(current, files, create));
    if (create) {
      setActiveId((active) => active ?? FILES_WINDOW_ID);
    }
  }, []);

  // openGit 打开 Git 窗口。
  const openGit = useCallback(() => {
    upsert({ id: GIT_WINDOW_ID, kind: "git", title: "Git" });
  }, [upsert]);

  // createKind 按种类新建窗口，可用现成的计划或文件做种子。
  const createKind = useCallback(
    (kind: DockKind, seed?: { plan?: PlanPreview; file?: FileChangePreview }) => {
      if (kind === "git") {
        openGit();
        return;
      }
      if (kind === "plan") {
        openPlan(seed?.plan ?? { kind: "doc", name: "", content: "", source: "read" });
        return;
      }
      openFile(seed?.file ?? EMPTY_FILE);
    },
    [openFile, openGit, openPlan],
  );

  // closeWindow 关掉指定窗口，并切到剩下的最后一个。
  const closeWindow = useCallback((id: string) => {
    setWindows((current) => {
      const next = current.filter((item) => item.id !== id);
      setActiveId((active) => {
        if (active !== id) {
          return active;
        }
        return next[next.length - 1]?.id ?? null;
      });
      return next;
    });
  }, []);

  // refreshOpen 只更新已经打开的窗口，内容没变就不动，避免把页面刷死。
  const refreshOpen = useCallback((window: DockWindow) => {
    setWindows((current) => {
      const index = current.findIndex((item) => item.id === window.id);
      if (index < 0) {
        return current;
      }
      const merged = { ...current[index], ...window } as DockWindow;
      if (sameDockWindow(current[index], merged)) {
        return current;
      }
      const next = current.slice();
      next[index] = merged;
      return next;
    });
  }, []);

  // reset 清空全部窗口，换会话时用。
  const reset = useCallback(() => {
    setWindows([]);
    setActiveId(null);
  }, []);

  return {
    windows,
    activeId,
    setActiveId,
    openPlan,
    openFile,
    selectFile,
    syncFiles,
    openGit,
    createKind,
    closeWindow,
    refreshOpen,
    reset,
  };
}

// sameDockWindow 判断刷新前后窗口内容是否一样。
function sameDockWindow(left: DockWindow, right: DockWindow): boolean {
  if (left.id !== right.id || left.kind !== right.kind || left.title !== right.title) {
    return false;
  }
  if (left.kind === "plan" && right.kind === "plan") {
    return (
      left.name === right.name &&
      left.content === right.content &&
      left.toolState === right.toolState &&
      left.error === right.error
    );
  }
  if (left.kind === "file" && right.kind === "file") {
    return left.selectedKey === right.selectedKey;
  }
  return true;
}

// collectDockArtifacts 从时间线抽出最新的计划和文件改动，给窗口菜单和刷新用。
export function collectDockArtifacts(items: TimelineItem[]): {
  plans: Array<PlanPreview & { toolState?: ToolItemState; error?: string }>;
  files: FileChangePreview[];
  fileGroups: TitledRunFileGroup[];
} {
  const plans = new Map<string, PlanPreview & { toolState?: ToolItemState; error?: string }>();
  const files = new Map<string, FileChangePreview>();
  for (const item of items) {
    if (item.kind !== "tool") {
      continue;
    }
    const preview = planPreviewFromTool(item);
    if (preview) {
      plans.set(preview.name, { ...preview, toolState: item.state, error: item.error });
      continue;
    }
    const change = fileChangeFromTool(item);
    if (change) {
      files.set(change.path, change);
    }
  }
  const list = [...files.values()];
  return {
    plans: [...plans.values()],
    files: list,
    fileGroups: list.length ? [{ runId: "session", title: "改动", files: list }] : [],
  };
}

// fileBasename 取路径最后一段给窗口标签用。
function fileBasename(path: string): string {
  const parts = path.replace(/\\/g, "/").split("/").filter(Boolean);
  return parts[parts.length - 1] || "";
}

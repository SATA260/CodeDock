"use client";

import {
  fileChangeFromTool,
  planPreviewFromTool,
  type FileChangePreview,
  type PlanPreview,
  type TimelineItem,
  type ToolItemState,
} from "@codedock/core/chat";
import { useCallback, useState } from "react";

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

export type FileWindow = {
  id: string;
  kind: "file";
  title: string;
  path: string;
  content: string;
  action: "write" | "edit";
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

// planWindowId 用计划名做窗口键，同名复用。
export function planWindowId(name: string): string {
  return `plan:${name || "untitled"}`;
}

// fileWindowId 用路径做窗口键，同文件复用。
export function fileWindowId(path: string): string {
  return `file:${path || "untitled"}`;
}

export const GIT_WINDOW_ID = "git";

// useWorkbench 管理右侧多窗口：打开、刷新已打开的、关闭、切到指定窗口。
export function useWorkbench() {
  const [windows, setWindows] = useState<DockWindow[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);

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

  const openFile = useCallback(
    (change: FileChangePreview) => {
      upsert({
        id: fileWindowId(change.path),
        kind: "file",
        title: basename(change.path),
        path: change.path,
        content: change.content,
        action: change.action,
      });
    },
    [upsert],
  );

  const openGit = useCallback(() => {
    upsert({ id: GIT_WINDOW_ID, kind: "git", title: "Git" });
  }, [upsert]);

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
      openFile(seed?.file ?? { path: "", content: "", action: "write" });
    },
    [openFile, openGit, openPlan],
  );

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

  const refreshOpen = useCallback((window: DockWindow) => {
    setWindows((current) => {
      const index = current.findIndex((item) => item.id === window.id);
      if (index < 0) {
        return current;
      }
      const next = current.slice();
      next[index] = { ...current[index], ...window } as DockWindow;
      return next;
    });
  }, []);

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
    openGit,
    createKind,
    closeWindow,
    refreshOpen,
    reset,
  };
}

// collectDockArtifacts 从时间线抽出最新的计划和文件改动，给窗口菜单和刷新用。
export function collectDockArtifacts(items: TimelineItem[]): {
  plans: Array<PlanPreview & { toolState?: ToolItemState; error?: string }>;
  files: FileChangePreview[];
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
  return { plans: [...plans.values()], files: [...files.values()] };
}

// basename 取路径最后一段，给标签用。
function basename(path: string): string {
  const parts = path.replace(/\\/g, "/").split("/").filter(Boolean);
  return parts[parts.length - 1] || path || "文件";
}

import { fileKey, firstLine, type FileChangePreview, type TimelineItem } from "@codedock/core/chat";

export const FILES_WINDOW_ID = "files";

export type FileWindow = {
  id: string;
  kind: "file";
  title: string;
  selectedKey: string; // runId:path，用来对上某一轮的那份 diff
};

type DockLike = { id: string; kind: string; selectedKey?: string };

// nextFilesWindows 同步文件窗口；create 时只补开列表，不预选文件。
export function nextFilesWindows<T extends DockLike>(
  current: T[],
  files: FileChangePreview[],
  create: boolean,
): T[] {
  const existing = current.find((item) => item.id === FILES_WINDOW_ID);
  if (!existing) {
    if (!create || files.length === 0) {
      return current;
    }
    return [
      ...current,
      {
        id: FILES_WINDOW_ID,
        kind: "file",
        title: "文件",
        selectedKey: "",
      } as unknown as T,
    ];
  }
  if (existing.kind !== "file") {
    return current;
  }
  const selected = files.some((file) => fileKey(file) === existing.selectedKey)
    ? (existing.selectedKey ?? "")
    : "";
  if (selected === existing.selectedKey) {
    return current;
  }
  return current.map((item) =>
    item.id === FILES_WINDOW_ID && item.kind === "file" ? { ...item, selectedKey: selected } : item,
  );
}

// runFileTitle 用该轮用户原话当框标题，没有就退回「改动」。
export function runFileTitle(items: TimelineItem[], runId: string): string {
  const user = items.find((item) => item.kind === "user" && item.runId === runId);
  if (user?.kind === "user" && user.text.trim()) {
    return firstLine(user.text, 36);
  }
  return "改动";
}

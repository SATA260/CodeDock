import { planPreviewFromTool } from "./plan.ts";

export type FileChangePreview = {
  path: string;
  content: string;
  action: "write" | "edit";
};

const FILE_TOOLS = new Set(["write", "edit"]);

// isFileChangeTool 判断工具是否改普通文件（计划文档不算）。
export function isFileChangeTool(name: string): boolean {
  return FILE_TOOLS.has(name);
}

// fileChangeFromTool 从 write/edit 抽出路径和正文；.cursor 计划走 plan 预览。
export function fileChangeFromTool(input: {
  name: string;
  arguments?: unknown;
  output?: unknown;
}): FileChangePreview | null {
  if (!FILE_TOOLS.has(input.name)) {
    return null;
  }
  if (planPreviewFromTool(input)) {
    return null;
  }
  const args = asRecord(input.arguments);
  const path = stringField(args, "path");
  if (!path) {
    return null;
  }
  if (input.name === "write") {
    return { path, content: stringField(args, "content"), action: "write" };
  }
  return {
    path,
    content: editContent(args),
    action: "edit",
  };
}

// compactToolDump 去掉计划正文和文件正文，只留路径等元数据给时间线。
export function compactToolDump(input: {
  name: string;
  arguments?: unknown;
  output?: unknown;
}): { input: unknown; output: unknown } {
  if (!planPreviewFromTool(input) && !fileChangeFromTool(input)) {
    return { input: input.arguments, output: input.output };
  }
  return {
    input: omitHeavy(input.arguments),
    output: omitHeavy(input.output),
  };
}

// editContent 拼出 edit 的替换说明，没有完整新文件时也能在右侧看。
function editContent(args: Record<string, unknown> | null): string {
  const whole = stringField(args, "newText") || stringField(args, "new_text");
  if (whole) {
    return whole;
  }
  const edits = args?.edits;
  if (!Array.isArray(edits) || edits.length === 0) {
    return "";
  }
  return edits
    .map((item, index) => {
      const row = asRecord(item);
      const next = stringField(row, "newText") || stringField(row, "new_text");
      const prev = stringField(row, "oldText") || stringField(row, "old_text");
      if (!next && !prev) {
        return "";
      }
      return `@@ edit ${index + 1} @@\n${prev ? `- ${prev}\n` : ""}${next ? `+ ${next}` : ""}`.trim();
    })
    .filter(Boolean)
    .join("\n\n");
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  if (typeof value !== "string") {
    return null;
  }
  const trimmed = value.trim();
  if (!trimmed.startsWith("{")) {
    return null;
  }
  try {
    const parsed: unknown = JSON.parse(trimmed);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return null;
  }
  return null;
}

function stringField(record: Record<string, unknown> | null, key: string): string {
  if (!record) {
    return "";
  }
  const value = record[key];
  return typeof value === "string" ? value : "";
}

function omitHeavy(value: unknown): unknown {
  const record = asRecord(value);
  if (!record) {
    return value;
  }
  const next = { ...record };
  delete next.content;
  delete next.newText;
  delete next.new_text;
  delete next.oldText;
  delete next.old_text;
  delete next.edits;
  return next;
}

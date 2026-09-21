import { planPreviewFromTool } from "./plan.ts";

export type FileChangePreview = {
  path: string;
  content: string;
  action: "write" | "edit";
  patch: string; // 服务端带过来的 unified diff；没有就空着，前端先不算
  runId: string; // 这次改动所属的 Run，用来按轮分框
};

export type RunFileGroup = {
  runId: string;
  files: FileChangePreview[];
};

const FILE_TOOLS = new Set(["write", "edit"]);

// isFileChangeTool 判断工具是否改普通文件（计划文档不算）。
export function isFileChangeTool(name: string): boolean {
  return FILE_TOOLS.has(name);
}

// fileChangeFromTool 从 write/edit 抽出路径和新正文；计划文档不算。前端先不现算 patch。
export function fileChangeFromTool(input: {
  name: string;
  arguments?: unknown;
  output?: unknown;
  runId?: string;
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
    return {
      path,
      content: stringField(args, "content"),
      action: "write",
      patch: detailsPatch(input.output),
      runId: input.runId ?? "",
    };
  }
  const sides = editSides(args);
  return {
    path,
    content: sides.next,
    action: "edit",
    patch: detailsPatch(input.output),
    runId: input.runId ?? "",
  };
}

// fileKey 用 run + 路径区分同一文件在不同轮次的改动。
export function fileKey(file: Pick<FileChangePreview, "runId" | "path">): string {
  return file.runId ? `${file.runId}:${file.path}` : file.path;
}

// collectRunFileGroups 按 run 归并 write/edit，同路径只留该轮最后一次。
export function collectRunFileGroups(
  items: Array<{
    kind: string;
    name?: string;
    runId?: string;
    arguments?: unknown;
    output?: unknown;
  }>,
): RunFileGroup[] {
  const groups = new Map<string, Map<string, FileChangePreview>>();
  const order: string[] = [];
  for (const item of items) {
    if (item.kind !== "tool" || !item.name) {
      continue;
    }
    const change = fileChangeFromTool({
      name: item.name,
      arguments: item.arguments,
      output: item.output,
      runId: item.runId ?? "",
    });
    if (!change) {
      continue;
    }
    const runId = change.runId || "run";
    let files = groups.get(runId);
    if (!files) {
      files = new Map();
      groups.set(runId, files);
      order.push(runId);
    }
    files.set(change.path, change);
  }
  return order.map((runId) => ({ runId, files: [...(groups.get(runId)?.values() ?? [])] }));
}

// compactToolDump 去掉计划正文、文件正文和 patch，只留路径等元数据给时间线。
export function compactToolDump(input: {
  name: string;
  arguments?: unknown;
  output?: unknown;
}): { input: unknown; output: unknown } {
  if (!planPreviewFromTool(input) && !isFileChangeTool(input.name)) {
    return { input: input.arguments, output: input.output };
  }
  return {
    input: omitHeavy(input.arguments),
    output: omitHeavy(input.output),
  };
}

// unifiedPatch 用公共前后缀生成一份 unified diff，给 DiffView 当 hunk。
export function unifiedPatch(path: string, oldText: string, newText: string): string {
  if (oldText === newText) {
    return "";
  }
  const oldLines = splitDiffLines(oldText);
  const newLines = splitDiffLines(newText);
  const prefix = commonAffix(oldLines, newLines);
  const suffix = commonSuffix(oldLines, newLines, prefix);
  const lines: string[] = [];
  for (let index = 0; index < prefix; index += 1) {
    lines.push(` ${oldLines[index]}`);
  }
  for (let index = prefix; index < oldLines.length - suffix; index += 1) {
    lines.push(`-${oldLines[index]}`);
  }
  for (let index = prefix; index < newLines.length - suffix; index += 1) {
    lines.push(`+${newLines[index]}`);
  }
  for (let index = oldLines.length - suffix; index < oldLines.length; index += 1) {
    lines.push(` ${oldLines[index]}`);
  }
  const oldStart = oldLines.length === 0 ? 0 : 1;
  const newStart = newLines.length === 0 ? 0 : 1;
  const oldLabel = oldLines.length === 0 ? "/dev/null" : `a/${path}`;
  const newLabel = newLines.length === 0 ? "/dev/null" : `b/${path}`;
  return [
    `--- ${oldLabel}`,
    `+++ ${newLabel}`,
    `@@ -${oldStart},${oldLines.length} +${newStart},${newLines.length} @@`,
    ...lines,
    "",
  ].join("\n");
}

// diffViewHunks 去掉工具输出里的分隔线，留下 DiffView 能吃的 unified hunk。
export function diffViewHunks(patch: string): string[] {
  const body = patch.replace(/^=+\r?\n/gm, "").trim();
  return body ? [body] : [];
}

// editSides 取出 edit 的旧/新正文；多段替换按段拼在一起。
function editSides(args: Record<string, unknown> | null): { prev: string; next: string } {
  const wholeNext = stringField(args, "newText") || stringField(args, "new_text");
  const wholePrev = stringField(args, "oldText") || stringField(args, "old_text");
  if (wholeNext || wholePrev) {
    return { prev: wholePrev, next: wholeNext };
  }
  const edits = args?.edits;
  if (!Array.isArray(edits) || edits.length === 0) {
    return { prev: "", next: "" };
  }
  const prev: string[] = [];
  const next: string[] = [];
  for (const item of edits) {
    const row = asRecord(item);
    prev.push(stringField(row, "oldText") || stringField(row, "old_text"));
    next.push(stringField(row, "newText") || stringField(row, "new_text"));
  }
  return { prev: prev.join("\n\n"), next: next.join("\n\n") };
}

// detailsPatch 从工具结果 details.patch 取出服务端算好的 unified diff。
function detailsPatch(output: unknown): string {
  const record = asRecord(output);
  return stringField(asRecord(record?.details), "patch");
}

// splitDiffLines 按行切开，丢掉末尾空段，和 unified hunk 对齐。
function splitDiffLines(text: string): string[] {
  if (text === "") {
    return [];
  }
  const lines = text.split("\n");
  if (lines[lines.length - 1] === "") {
    lines.pop();
  }
  return lines;
}

// commonAffix 返回新旧行列从头开始相同的行数。
function commonAffix(oldLines: string[], newLines: string[]): number {
  let length = 0;
  while (length < oldLines.length && length < newLines.length && oldLines[length] === newLines[length]) {
    length += 1;
  }
  return length;
}

// commonSuffix 在去掉公共前缀后，从尾部再数相同行。
function commonSuffix(oldLines: string[], newLines: string[], prefix: number): number {
  let length = 0;
  while (
    length < oldLines.length - prefix &&
    length < newLines.length - prefix &&
    oldLines[oldLines.length - 1 - length] === newLines[newLines.length - 1 - length]
  ) {
    length += 1;
  }
  return length;
}

// asRecord 把对象或 JSON 对象字符串收成字典。
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

// stringField 读字典里的字符串字段，缺了或类型不对就空串。
function stringField(record: Record<string, unknown> | null, key: string): string {
  if (!record) {
    return "";
  }
  const value = record[key];
  return typeof value === "string" ? value : "";
}

// omitHeavy 去掉正文、替换片段和 diff，避免时间线被文件内容撑满。
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
  const details = asRecord(next.details);
  if (details) {
    const slim = { ...details };
    delete slim.patch;
    delete slim.diff;
    next.details = slim;
  }
  return next;
}

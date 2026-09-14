export type PlanDocPreview = {
  kind: "doc";
  name: string;
  content: string;
  source: "write" | "read";
};

export type PlanPreview = PlanDocPreview;

const PLAN_DOC_TOOLS = new Set(["plan_read", "plan_write"]);

export function isPlanTool(name: string): boolean {
  return name === "plan_list" || PLAN_DOC_TOOLS.has(name);
}

export function normalizePlanName(name: string): string {
  const base = name.trim().split(/[\\/]/).pop()?.trim() ?? "";
  if (!base) {
    return "plan.md";
  }
  return /\.md$/i.test(base) ? base : `${base}.md`;
}

export function planPreviewFromTool(input: {
  name: string;
  arguments?: unknown;
  output?: unknown;
}): PlanPreview | null {
  if (input.name === "write") {
    const args = asRecord(input.arguments);
    const name = planNameFromWritePath(stringField(args, "path"));
    if (!name) {
      return null;
    }
    return {
      kind: "doc",
      name,
      content: stringField(args, "content"),
      source: "write",
    };
  }
  if (!PLAN_DOC_TOOLS.has(input.name)) {
    return null;
  }
  const output = asRecord(input.output);
  const args = asRecord(input.arguments);
  return {
    kind: "doc",
    name: normalizePlanName(stringField(output, "name") || stringField(args, "name")),
    content: stringField(output, "content") || stringField(args, "content"),
    source: input.name === "plan_write" ? "write" : "read",
  };
}

export function latestPlanDocIds(
  tools: Array<{ id: string; name: string; arguments?: unknown; output?: unknown }>,
): Set<string> {
  let current: string | undefined;
  for (const tool of tools) {
    if (planPreviewFromTool(tool)) {
      current = tool.id;
    }
  }
  return current ? new Set([current]) : new Set();
}

export function planToolDump(input: {
  name: string;
  arguments?: unknown;
  output?: unknown;
}): { input: unknown; output: unknown } {
  if (!planPreviewFromTool(input)) {
    return { input: input.arguments, output: input.output };
  }
  return {
    input: omitContent(input.arguments),
    output: omitContent(input.output),
  };
}

function planNameFromWritePath(path: string): string | null {
  const parts = path.trim().replace(/\\/g, "/").split("/").filter(Boolean);
  if (parts.length < 2) {
    return null;
  }
  const file = parts[parts.length - 1] ?? "";
  const dir = parts[parts.length - 2];
  if (dir !== ".cursor" || !/\.md$/i.test(file)) {
    return null;
  }
  return file;
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

function omitContent(value: unknown): unknown {
  const record = asRecord(value);
  if (!record || !("content" in record)) {
    return value;
  }
  const { content: _content, ...rest } = record;
  return rest;
}

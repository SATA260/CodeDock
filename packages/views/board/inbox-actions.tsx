"use client";

import type { BoardEngine, InboxItem } from "@codedock/core/board";
import { Button } from "@codedock/ui";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useMemo, useState } from "react";

import { useAgent } from "../provider.tsx";
import { useBoard } from "./provider.tsx";

type Choice = "approved" | "denied";

type ToolPage = {
  id: string;
  name: string;
  detail: string;
};

// InboxActions 在列内按条审批。同一张问票里的工具一次只看一条摘要，选齐才提交。
export function InboxActions({
  items,
  onDone,
}: {
  items: InboxItem[];
  onDone?: () => Promise<void> | void;
}) {
  if (items.length === 0) {
    return null;
  }
  return (
    <div className="mt-1 space-y-1">
      {items.map((item) => (
        <InboxTicket key={`${item.engine}:${item.ticket_id}`} item={item} onDone={onDone} />
      ))}
    </div>
  );
}

// InboxTicket 一次展示一条工具摘要，通过或拒绝后再看下一条。
function InboxTicket({
  item,
  onDone,
}: {
  item: InboxItem;
  onDone?: () => Promise<void> | void;
}) {
  const { client } = useBoard();
  const { userId } = useAgent();
  const pages = useMemo(() => toolPages(item), [item]);
  const batched = useMemo(() => hasToolCalls(item), [item]);
  const [index, setIndex] = useState(0);
  const [choices, setChoices] = useState<Record<string, Choice>>({});
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const page = pages[Math.min(index, Math.max(pages.length - 1, 0))];
  if (!page) {
    return null;
  }
  const choice = choices[page.id];

  // choose 记下当前这一条。同票里还有没选的就翻过去；都选完才按引擎提交。
  const choose = async (status: Choice) => {
    const nextChoices = { ...choices, [page.id]: status };
    setChoices(nextChoices);
    setError(null);
    const missing = pages.findIndex((entry) => !nextChoices[entry.id]);
    if (missing >= 0) {
      const later = pages.findIndex((entry, cursor) => cursor > index && !nextChoices[entry.id]);
      setIndex(later >= 0 ? later : missing);
      return;
    }
    setSubmitting(true);
    try {
      const engine = (item.engine === "native" ? "agent" : item.engine) as BoardEngine;
      const picked = pages.map((entry) => ({
        id: entry.id,
        status: nextChoices[entry.id] ?? status,
      }));
      if (engine === "agent" && batched) {
        await client.decideInbox({
          engine,
          ticket_id: item.ticket_id,
          decisions: picked.map((entry) => ({ tool_call_id: entry.id, status: entry.status })),
          scope: "once",
          actor_id: userId,
        });
      } else if (engine === "agent") {
        await client.decideInbox({
          engine,
          ticket_id: item.ticket_id,
          status: picked[0]?.status ?? status,
          scope: "once",
          actor_id: userId,
        });
      } else {
        await client.decideInbox({
          engine,
          ticket_id: item.ticket_id,
          approved: (picked[0]?.status ?? status) === "approved",
          scope: "once",
        });
      }
      await onDone?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "审批失败");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="rounded border border-border/70 px-1.5 py-1">
      {pages.length > 1 ? (
        <div className="mb-1 flex items-center justify-between gap-1 text-[10px] text-muted-foreground">
          <span>一条一条审</span>
          <div className="flex items-center gap-0.5">
            <Button
              size="sm"
              variant="ghost"
              className="size-6 px-0"
              disabled={index <= 0 || submitting}
              aria-label="上一条"
              onClick={() => setIndex((current) => Math.max(0, current - 1))}
            >
              <ChevronLeft className="size-3.5" />
            </Button>
            <span className="min-w-8 text-center tabular-nums">
              {index + 1} / {pages.length}
            </span>
            <Button
              size="sm"
              variant="ghost"
              className="size-6 px-0"
              disabled={index >= pages.length - 1 || submitting}
              aria-label="下一条"
              onClick={() => setIndex((current) => Math.min(pages.length - 1, current + 1))}
            >
              <ChevronRight className="size-3.5" />
            </Button>
          </div>
        </div>
      ) : null}
      <div className="flex items-center justify-between gap-2">
        <p className="min-w-0 truncate font-mono text-[11px] text-foreground">{page.name}</p>
        <span className="shrink-0 text-[10px] text-muted-foreground">{choiceLabel(choice)}</span>
      </div>
      {page.detail ? (
        <pre className="mt-0.5 max-h-24 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] leading-4 text-muted-foreground">
          {page.detail}
        </pre>
      ) : null}
      <div className="mt-1 flex gap-1">
        <Button size="sm" variant="secondary" disabled={submitting} onClick={() => void choose("approved")}>
          {choice === "approved" ? "已通过" : "通过"}
        </Button>
        <Button size="sm" variant="ghost" disabled={submitting} onClick={() => void choose("denied")}>
          {choice === "denied" ? "已拒绝" : "拒绝"}
        </Button>
      </div>
      {error ? <p className="mt-1 text-[11px] text-destructive">{error}</p> : null}
    </div>
  );
}

// choiceLabel 用短字标出这一条还没选、已通过或已拒绝。
function choiceLabel(choice?: Choice): string {
  if (choice === "approved") {
    return "已通过";
  }
  if (choice === "denied") {
    return "已拒绝";
  }
  return "待审";
}

// hasToolCalls 判断这张票是不是一批要分开裁的工具。
function hasToolCalls(item: InboxItem): boolean {
  const payload = asRecord(item.payload);
  return Array.isArray(payload?.tool_calls) && payload.tool_calls.some((call) => text(asRecord(call)?.id));
}

// toolPages 把一张问票拆成一条工具一页。没有工具列表时整票算一页。
function toolPages(item: InboxItem): ToolPage[] {
  const payload = asRecord(item.payload);
  const calls = Array.isArray(payload?.tool_calls) ? payload.tool_calls : [];
  const pages: ToolPage[] = [];
  for (const call of calls) {
    const row = asRecord(call);
    const id = text(row?.id);
    if (!id) {
      continue;
    }
    pages.push({
      id,
      name: text(row?.name) || "工具",
      detail: argumentDetail(row?.arguments),
    });
  }
  if (pages.length > 0) {
    return pages;
  }
  const command = text(payload?.command);
  const prompt = text(payload?.prompt);
  const paths = Array.isArray(payload?.paths) ? payload.paths.map(text).filter(Boolean).join("\n") : "";
  const detail = [command, prompt, paths].filter(Boolean).join("\n");
  const name = text(payload?.kind) || item.summary || "审批";
  return [{ id: item.ticket_id, name: name === "tools" ? item.summary || "审批" : name, detail }];
}

// argumentDetail 抽出命令、路径或搜索词。没有这些字段时收成短 JSON。
function argumentDetail(value: unknown): string {
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) {
      return "";
    }
    try {
      return argumentDetail(JSON.parse(trimmed) as unknown);
    } catch {
      return trimmed;
    }
  }
  const row = asRecord(value);
  if (!row) {
    return "";
  }
  const command = text(row.command);
  if (command) {
    return command;
  }
  const path = text(row.path);
  const pattern = text(row.pattern);
  if (path && pattern) {
    return `${pattern}  ${path}`;
  }
  if (path) {
    return path;
  }
  if (pattern) {
    return pattern;
  }
  const compact = JSON.stringify(row);
  return compact === "{}" ? "" : compact;
}

// asRecord 只接受普通对象。
function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

// text 取出非空字符串。
function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

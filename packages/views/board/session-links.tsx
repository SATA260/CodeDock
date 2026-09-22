"use client";

import type { BoardEngine, SessionLinks } from "@codedock/core/board";
import { Button } from "@codedock/ui";
import { X } from "lucide-react";
import { useEffect, useState } from "react";

import { useBoard } from "./provider.tsx";

// SessionLinkEditor 按行编辑会话上的 GitHub 链接，不让用户选择 Issue 还是 PR。
export function SessionLinkEditor({
  engine,
  sessionId,
}: {
  engine: BoardEngine;
  sessionId: string;
}) {
  const { client } = useBoard();
  const [rows, setRows] = useState<string[]>([""]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void client
      .getLinks(engine, sessionId)
      .then((links) => {
        if (!cancelled) {
          setRows(rowsFromLinks(links));
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "无法读取链接");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [client, engine, sessionId]);

  // save 把非空行整批换上去。
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const saved = await client.replaceLinks(
        engine,
        sessionId,
        rows.map((row) => row.trim()).filter(Boolean),
      );
      setRows(rowsFromLinks(saved));
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="shrink-0 border-b border-border px-2 py-2">
      <div className="flex flex-col gap-1">
        {rows.map((row, index) => (
          <div key={index} className="flex items-center gap-1">
            <input
              value={row}
              aria-label={`链接 ${index + 1}`}
              placeholder="https://github.com/owner/repo/issues/15"
              className="h-7 min-w-0 flex-1 rounded border border-border bg-background px-2 text-xs"
              onChange={(event) => {
                const next = rows.slice();
                next[index] = event.target.value;
                setRows(next);
              }}
            />
            <Button
              size="sm"
              variant="ghost"
              className="px-1.5"
              aria-label="删除这条链接"
              onClick={() => {
                const next = rows.filter((_, item) => item !== index);
                setRows(next.length > 0 ? next : [""]);
              }}
            >
              <X className="size-3.5" />
            </Button>
          </div>
        ))}
      </div>
      <div className="mt-1.5 flex items-center gap-1">
        <Button size="sm" variant="ghost" onClick={() => setRows([...rows, ""])}>
          添加一条
        </Button>
        <Button size="sm" disabled={busy} onClick={() => void save()}>
          {busy ? "保存中" : "保存"}
        </Button>
      </div>
      {error ? <p className="mt-1 text-xs text-destructive">{error}</p> : null}
    </div>
  );
}

// rowsFromLinks 把已挂快照收成一行一个链接；没有时留一条空行。
function rowsFromLinks(links: SessionLinks): string[] {
  const rows: string[] = [];
  if (links.issue) {
    rows.push(linkText(links.issue.url, links.issue.repo, links.issue.number, "issues"));
  }
  for (const pull of links.pulls) {
    rows.push(linkText(pull.url, pull.repo, pull.number, "pull"));
  }
  return rows.length > 0 ? rows : [""];
}

// linkText 优先用快照里的地址，没有就按仓库和编号拼一条 GitHub 链接。
function linkText(url: string, repo: string, number: number, path: "issues" | "pull"): string {
  if (url) {
    return url;
  }
  if (repo && number > 0) {
    return `https://github.com/${repo}/${path}/${number}`;
  }
  return "";
}

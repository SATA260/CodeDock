"use client";

import { Button } from "@codedock/ui";
import { Folder, FolderOpen } from "lucide-react";
import { useEffect, useState } from "react";

import type { DirectoryListing } from "../provider.tsx";

export function WorkspacePicker({
  initialPath,
  listDirectories,
  onSelect,
  onClose,
}: {
  initialPath?: string;
  listDirectories: (path?: string) => Promise<DirectoryListing>;
  onSelect: (path: string) => void;
  onClose: () => void;
}) {
  const [listing, setListing] = useState<DirectoryListing | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = async (path?: string) => {
    setLoading(true);
    setError(null);
    try {
      setListing(await listDirectories(path));
    } catch (err) {
      if (path) {
        try {
          setListing(await listDirectories());
          setError(null);
          return;
        } catch {
          // keep the original error
        }
      }
      setError(err instanceof Error ? err.message : "无法打开目录");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load(initialPath);
  }, [initialPath]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      role="presentation"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-labelledby="workspace-picker-title"
        data-workspace-dialog=""
        className="flex max-h-[min(32rem,80vh)] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-border bg-zinc-900 shadow-xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="border-b border-border px-4 py-3">
          <h2 id="workspace-picker-title" className="text-sm font-medium text-foreground">
            选择工作目录
          </h2>
          <p className="mt-1 truncate font-mono text-xs text-muted-foreground" title={listing?.path}>
            {listing?.path ?? "…"}
          </p>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-2 py-2">
          {listing?.parent ? (
            <button
              type="button"
              className="flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
              onClick={() => void load(listing.parent)}
            >
              <Folder className="size-3.5 shrink-0" />
              ..
            </button>
          ) : null}
          {loading && !listing ? (
            <p className="px-2 py-6 text-xs text-muted-foreground">正在读取目录…</p>
          ) : null}
          {error ? <p className="px-2 py-2 text-xs text-destructive">{error}</p> : null}
          {listing?.entries.map((entry) => (
            <button
              key={entry.path}
              type="button"
              className="flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-sm text-foreground hover:bg-muted"
              onClick={() => void load(entry.path)}
            >
              <FolderOpen className="size-3.5 shrink-0 text-muted-foreground" />
              <span className="truncate">{entry.name}</span>
            </button>
          ))}
          {listing && listing.entries.length === 0 && !loading ? (
            <p className="px-2 py-6 text-xs text-muted-foreground">这个目录下没有子文件夹</p>
          ) : null}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-3">
          <Button size="sm" variant="ghost" onClick={onClose}>
            取消
          </Button>
          <Button
            size="sm"
            disabled={!listing?.path}
            onClick={() => {
              if (listing?.path) {
                onSelect(listing.path);
              }
            }}
          >
            选择此目录
          </Button>
        </div>
      </div>
    </div>
  );
}

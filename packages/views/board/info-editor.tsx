"use client";

import { Button } from "@codedock/ui";
import { useEffect, useRef, useState, type ComponentType } from "react";
import { createPortal } from "react-dom";

type MarkdownEditor = ComponentType<{
  value?: string;
  height?: number | string;
  preview?: "live" | "edit" | "preview";
  visibleDragbar?: boolean;
  autoFocus?: boolean;
  commands?: unknown[];
  extraCommands?: unknown[];
  onChange?: (value?: string) => void;
}>;

// InfoEditor 弹出 @uiw/react-md-editor，用来改这张卡的说明。
export function InfoEditor({
  open,
  value,
  onClose,
  onSave,
}: {
  open: boolean;
  value: string;
  onClose: () => void;
  onSave: (next: string) => Promise<void>;
}) {
  const [draft, setDraft] = useState(value);
  const valueRef = useRef(value);
  valueRef.current = value;
  const [Editor, setEditor] = useState<MarkdownEditor | null>(null);
  const [commands, setCommands] = useState<{ main: unknown[]; extra: unknown[] } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      setBusy(false);
      return;
    }
    setDraft(valueRef.current);
    setError(null);
    let cancelled = false;
    void Promise.all([import("@uiw/react-md-editor"), import("@uiw/react-md-editor/commands-cn")]).then(
      ([editorMod, cn]) => {
        if (cancelled) {
          return;
        }
        setEditor(() => editorMod.default);
        setCommands({ main: cn.getCommands(), extra: cn.getExtraCommands() });
      },
      (err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "编辑器加载失败");
        }
      },
    );
    return () => {
      cancelled = true;
    };
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [busy, onClose, open]);

  if (!open || typeof document === "undefined") {
    return null;
  }

  // save 写回说明并关掉编辑框。
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await onSave(draft);
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setBusy(false);
    }
  };

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4"
      role="presentation"
      onClick={() => {
        if (!busy) {
          onClose();
        }
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="work-info-editor-title"
        className="flex h-[min(72vh,720px)] w-full max-w-4xl flex-col overflow-hidden rounded-lg border border-border bg-background shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="flex shrink-0 items-center justify-between border-b border-border px-3 py-2">
          <h2 id="work-info-editor-title" className="text-sm font-medium">
            编辑说明
          </h2>
        </header>
        <div data-color-mode="dark" className="min-h-0 flex-1">
          {Editor && commands ? (
            <Editor
              value={draft}
              height="100%"
              preview="live"
              visibleDragbar={false}
              autoFocus
              commands={commands.main}
              extraCommands={commands.extra}
              onChange={(next) => setDraft(next ?? "")}
            />
          ) : (
            <p className="px-3 py-2 text-sm text-muted-foreground">{error ?? "正在打开编辑器"}</p>
          )}
        </div>
        {error && Editor ? <p className="shrink-0 px-3 py-1 text-xs text-destructive">{error}</p> : null}
        <footer className="flex shrink-0 items-center justify-end gap-2 border-t border-border px-3 py-2">
          <Button size="sm" variant="ghost" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button size="sm" disabled={busy || !Editor} onClick={() => void save()}>
            {busy ? "保存中" : "保存"}
          </Button>
        </footer>
      </div>
    </div>,
    document.body,
  );
}

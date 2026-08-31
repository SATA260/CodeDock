"use client";

import type { DiffFile as GitDiffFile } from "@codedock/core/git";
import { DiffModeEnum, DiffView } from "@git-diff-view/react";
import "@git-diff-view/react/styles/diff-view.css";

import { scopeLabel, type PreviewTarget } from "./lib/preview.ts";

export function DiffPanel({
  target,
  file,
  ready,
}: {
  target: PreviewTarget | null;
  file: GitDiffFile | null;
  ready: boolean;
}) {
  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
        {target ? (
          <>
            <span className="shrink-0 text-[11px] text-muted-foreground">{scopeLabel(target.scope)}</span>
            <span className="min-w-0 truncate font-mono text-[12px]">{target.path}</span>
            {file?.orig_path ? (
              <span className="min-w-0 truncate text-[11px] text-muted-foreground">来自 {file.orig_path}</span>
            ) : null}
          </>
        ) : (
          <span className="text-[12px] text-muted-foreground">差异</span>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">{content(target, file, ready)}</div>
    </section>
  );
}

function content(target: PreviewTarget | null, file: GitDiffFile | null, ready: boolean) {
  if (!target) {
    return <Empty>点击左侧文件查看差异</Empty>;
  }
  if (!ready) {
    return <Empty>正在读取差异…</Empty>;
  }
  if (!file) {
    return <Empty>该文件当前没有差异</Empty>;
  }
  if (file.binary) {
    return <Empty>二进制文件，无法展示文本差异</Empty>;
  }
  const patch = file.patch.trim();
  if (!patch) {
    return <Empty>这个文件没有可展示的文本差异</Empty>;
  }
  return (
    <DiffView
      key={`${target.scope}:${file.path}`}
      className="min-h-full"
      data={{
        oldFile: { fileName: file.orig_path || file.path },
        newFile: { fileName: file.path },
        hunks: [file.patch],
      }}
      diffViewMode={DiffModeEnum.Unified}
      diffViewTheme="dark"
      diffViewHighlight
      diffViewWrap
      diffViewFontSize={12}
    />
  );
}

function Empty({ children }: { children: string }) {
  return <p className="px-4 py-8 text-sm text-muted-foreground">{children}</p>;
}

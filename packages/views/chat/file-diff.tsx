"use client";

import type { FileChangePreview } from "@codedock/core/chat";

// FileDiffPane 展示当前选中文件的正文；先不渲染 git diff，避免把页面卡死。
export function FileDiffPane({ file }: { file: FileChangePreview | null }) {
  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="dock-file-diff">
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
        {file?.path ? (
          <>
            <span className="min-w-0 truncate font-mono text-[12px]">{file.path}</span>
            <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">
              {file.action === "edit" ? "编辑" : "写入"}
            </span>
          </>
        ) : (
          <span className="text-[12px] text-muted-foreground">文件</span>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">{fileBody(file)}</div>
    </section>
  );
}

// fileBody 只铺正文；没选中或没有内容时给空态。
function fileBody(file: FileChangePreview | null) {
  if (!file?.path) {
    return <Empty>从对话里点开一个文件查看内容</Empty>;
  }
  if (!file.content.trim()) {
    return <Empty>这次改动没有带上正文</Empty>;
  }
  return (
    <pre className="whitespace-pre-wrap break-words px-4 py-3 font-mono text-xs leading-5 text-foreground">
      {file.content}
    </pre>
  );
}

// Empty 右侧还没有可展示正文时的提示。
function Empty({ children }: { children: string }) {
  return <p className="px-4 py-8 text-sm text-muted-foreground">{children}</p>;
}

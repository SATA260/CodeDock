"use client";

import type { FileChangePreview } from "@codedock/core/chat";

// FileDiffPane 展示单个文件正文：行号 + 不折行，由外层单独滚动。
export function FileDiffPane({ file }: { file: FileChangePreview | null }) {
  return (
    <section className="flex h-full min-h-0 min-w-0 flex-1 flex-col" data-testid="dock-file-diff">
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
      <div className="min-h-0 flex-1 overflow-auto overscroll-contain" data-testid="dock-file-source">
        {fileBody(file)}
      </div>
    </section>
  );
}

// fileBody 按行铺正文，高度跟内容走，避免被代码块组件锁成窗口高。
function fileBody(file: FileChangePreview | null) {
  if (!file?.path) {
    return <Empty>从对话里点开一个文件查看内容</Empty>;
  }
  if (!file.content.trim()) {
    return <Empty>这次改动没有带上正文</Empty>;
  }
  const lines = file.content.split("\n");
  return (
    <div className="w-max min-w-full py-2 font-mono text-[13px] leading-5">
      {lines.map((line, index) => (
        <div key={index} className="flex">
          <span className="sticky left-0 w-10 shrink-0 select-none bg-background pr-3 text-right text-[12px] text-muted-foreground/50">
            {index + 1}
          </span>
          <span className="whitespace-pre pr-4">{line.length > 0 ? line : " "}</span>
        </div>
      ))}
    </div>
  );
}

// Empty 右侧还没有可展示正文时的提示。
function Empty({ children }: { children: string }) {
  return <p className="px-4 py-8 text-sm text-muted-foreground">{children}</p>;
}

"use client";

import { Button } from "@codedock/ui";
import { useMemo, useState, type ReactNode } from "react";

import { BranchSwitcher } from "./branch-switcher.tsx";
import { CommitHistory } from "./commit-history.tsx";
import { DiffPanel } from "./diff-panel.tsx";
import { useGitSite } from "./hooks/use-git-site.ts";
import type { PreviewTarget } from "./lib/preview.ts";
import { shortHead, trackLabel } from "./lib/status.ts";
import { WorkspacePanel } from "./workspace-panel.tsx";

export type GitPageProps = {
  onBack?: () => void;
  headerActions?: ReactNode;
};

export function GitPage({ onBack, headerActions }: GitPageProps) {
  const site = useGitSite();
  const [preview, setPreview] = useState<PreviewTarget | null>(null);
  const previewFile = useMemo(() => {
    if (!preview) {
      return null;
    }
    const list = preview.scope === "staged" ? site.diffs.staged : site.diffs.worktree;
    return list.find((file) => file.path === preview.path) ?? null;
  }, [preview, site.diffs]);

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background text-foreground">
      <header className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
        <div className="shrink-0 text-sm font-semibold tracking-tight">仓库</div>
        {site.state.is_repo ? (
          <>
            <BranchSwitcher
              view={site.branches}
              busy={site.busy}
              onSelectLocal={site.switchBranch}
              onSelectRemote={site.checkoutRemote}
            />
            <CommitHistory commits={site.commits} busy={site.busy} />
            <div className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">
              {site.state.path || "—"}
              {site.state.head ? ` · ${shortHead(site.state.head)}` : ""}
              {site.state.upstream
                ? ` · ${trackLabel(site.state.ahead, site.state.behind, site.state.upstream, site.state.upstream_gone)}`
                : ""}
              {site.state.integrating ? ` · 正在 ${site.state.integrating}` : ""}
            </div>
          </>
        ) : null}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          {headerActions}
          {onBack ? (
            <Button size="sm" variant="ghost" onClick={onBack}>
              回对话
            </Button>
          ) : null}
        </div>
      </header>
      {site.error ? (
        <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-red-300">
          {site.error}
        </div>
      ) : null}
      <div className="flex min-h-0 flex-1">
        {site.loading ? (
          <p className="px-3 py-2 text-xs text-muted-foreground">正在读取仓库…</p>
        ) : !site.state.is_repo ? (
          <p className="px-3 py-2 text-sm text-muted-foreground">
            当前文件夹还不是 Git 仓库。把 <span className="font-mono">GIT_REPO</span> 指到仓根后再打开。
          </p>
        ) : (
          <>
            <WorkspacePanel
              state={site.state}
              busy={site.busy}
              generating={site.generating}
              prompt={site.prompt}
              preview={preview}
              onPreview={setPreview}
              onReload={site.reload}
              onStage={site.stage}
              onUnstage={site.unstage}
              onDiscard={site.discard}
              onCommit={site.commit}
              onGenerate={site.generate}
              onSavePrompt={site.savePrompt}
              onPush={site.push}
            />
            <DiffPanel target={preview} file={previewFile} ready={!site.loading} />
          </>
        )}
      </div>
    </div>
  );
}

"use client";

import type { MessageDraft, PromptConfig, SiteState } from "@codedock/core/git";
import { Button } from "@codedock/ui";
import { RefreshCw } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { DiscardConfirm, type DiscardRequest } from "./discard-confirm.tsx";
import { FileTree } from "./file-tree.tsx";
import type { PreviewTarget } from "./lib/preview.ts";
import { splitWorkspaceFiles, stagedLabel, worktreeLabel } from "./lib/status.ts";
import { PromptPicker } from "./prompt-picker.tsx";
import { PublishActions } from "./publish-actions.tsx";

function draftText(draft: MessageDraft): string {
  const title = draft.title.trim();
  const body = draft.body.trim();
  return body ? `${title}\n\n${body}` : title;
}

export function WorkspacePanel({
  state,
  busy,
  generating,
  prompt,
  preview,
  onPreview,
  onReload,
  onStage,
  onUnstage,
  onDiscard,
  onCommit,
  onGenerate,
  onSavePrompt,
  onPush,
  compact = false,
}: {
  compact?: boolean;
  state: SiteState;
  busy: boolean;
  generating: boolean;
  prompt: PromptConfig | null;
  preview: PreviewTarget | null;
  onPreview: (target: PreviewTarget) => void;
  onReload: () => Promise<void>;
  onStage: (paths: string[]) => Promise<void>;
  onUnstage: (paths: string[]) => Promise<void>;
  onDiscard: (paths: string[]) => Promise<void>;
  onCommit: (message: string) => Promise<void>;
  onGenerate: () => Promise<MessageDraft>;
  onSavePrompt: (selected: string, custom: string) => Promise<void>;
  onPush: () => Promise<void>;
}) {
  const [message, setMessage] = useState("");
  const [discardRequest, setDiscardRequest] = useState<DiscardRequest | null>(null);
  const files = state.files ?? [];
  const { staged, worktree } = useMemo(() => splitWorkspaceFiles(files), [files]);

  const stage = (paths: string[]) => {
    void onStage(paths)
      .then(() => {
        if (preview && preview.scope === "worktree" && paths.includes(preview.path)) {
          onPreview({ path: preview.path, scope: "staged" });
        }
      })
      .catch(() => undefined);
  };

  const unstage = (paths: string[]) => {
    void onUnstage(paths)
      .then(() => {
        if (preview && preview.scope === "staged" && paths.includes(preview.path)) {
          onPreview({ path: preview.path, scope: "worktree" });
        }
      })
      .catch(() => undefined);
  };

  const confirmDiscard = () => {
    if (!discardRequest) {
      return;
    }
    void onDiscard(discardRequest.paths)
      .then(() => setDiscardRequest(null))
      .catch(() => undefined);
  };

  return (
    <section
      className={
        compact
          ? "flex max-h-[32%] min-h-[6.5rem] w-full shrink-0 flex-col border-b border-border"
          : "flex h-full min-h-0 w-[340px] shrink-0 flex-col border-r border-border"
      }
    >
      <form
        className={
          compact
            ? "flex shrink-0 flex-col gap-1 border-b border-border px-2 py-1.5"
            : "flex shrink-0 flex-col gap-2 border-b border-border px-3 py-2"
        }
        onSubmit={(event) => {
          event.preventDefault();
          const text = message.trim();
          if (!text) {
            return;
          }
          void onCommit(text)
            .then(() => {
              setMessage("");
            })
            .catch(() => undefined);
        }}
      >
        <textarea
          className={
            compact
              ? "min-h-9 w-full resize-none bg-transparent text-[11px] leading-snug outline-none placeholder:text-muted-foreground"
              : "min-h-[168px] w-full resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          }
          placeholder="已确认的说明，提交前可以再改"
          value={message}
          onChange={(event) => setMessage(event.target.value)}
        />
        <div className={compact ? "flex items-center gap-1" : "flex items-center gap-2"}>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className={compact ? "h-6 px-2 text-[11px]" : undefined}
            disabled={busy || generating || staged.length === 0}
            onClick={() => {
              void onGenerate()
                .then((draft) => {
                  setMessage(draftText(draft));
                })
                .catch(() => undefined);
            }}
          >
            {generating ? "生成中" : "生成"}
          </Button>
          <PromptPicker
            prompt={prompt}
            disabled={busy || generating}
            onSelect={(selected) => onSavePrompt(selected, prompt?.custom ?? "")}
            onSaveCustom={(custom) => onSavePrompt(prompt?.selected ?? "custom", custom)}
          />
          <div className="ml-auto">
            <PublishActions
              compact={compact}
              busy={busy}
              canCommit={!busy && message.trim() !== "" && staged.length > 0}
              canPush={
                !busy && (state.remotes ?? []).length > 0 && Boolean(state.upstream) && !state.upstream_gone
              }
              onPush={() => {
                void onPush().catch(() => undefined);
              }}
            />
          </div>
        </div>
      </form>
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <FileSection
          compact={compact}
          title="已暂存"
          count={staged.length}
          action={
            <Button
              size="sm"
              variant="ghost"
              className={compact ? "h-6 px-1.5 text-[11px]" : undefined}
              disabled={busy}
              onClick={() => void onReload()}
            >
              <RefreshCw className={busy ? "size-3 animate-spin" : "size-3"} />
              刷新
            </Button>
          }
        >
          <FileTree
            compact={compact}
            files={staged}
            busy={busy}
            activePath={preview?.scope === "staged" ? preview.path : null}
            labelOf={stagedLabel}
            empty="没有已暂存的文件"
            action={{ kind: "unstage", onRun: unstage }}
            onPreview={(path) => onPreview({ path, scope: "staged" })}
          />
        </FileSection>
        <FileSection compact={compact} title="当前目录" count={worktree.length}>
          <FileTree
            compact={compact}
            files={worktree}
            busy={busy}
            activePath={preview?.scope === "worktree" ? preview.path : null}
            labelOf={worktreeLabel}
            empty="工作区是干净的"
            action={{ kind: "stage", onRun: stage, onDiscard: setDiscardRequest }}
            onPreview={(path) => onPreview({ path, scope: "worktree" })}
          />
        </FileSection>
      </div>
      <DiscardConfirm
        request={discardRequest}
        busy={busy}
        onCancel={() => setDiscardRequest(null)}
        onConfirm={confirmDiscard}
      />
    </section>
  );
}

// FileSection 文件列表的分组标题，侧栏里用更小字号。
function FileSection({
  title,
  count,
  action,
  compact = false,
  children,
}: {
  title: string;
  count: number;
  action?: ReactNode;
  compact?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="border-b border-border last:border-b-0">
      <div className={compact ? "flex items-center gap-1.5 px-2 py-0.5" : "flex items-center gap-2 px-3 py-1"}>
        <div className={compact ? "text-[11px] font-medium" : "text-sm font-medium"}>
          {title}
          <span className="ml-1.5 text-[10px] font-normal text-muted-foreground">{count}</span>
        </div>
        {action}
      </div>
      {children}
    </div>
  );
}

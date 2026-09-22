"use client";

import type { ClaudeSettings, ClaudeSettingsPatch } from "@codedock/core/claude";
import { Button } from "@codedock/ui";
import { useState } from "react";

import type { PickedLocalFile, PickFilesOptions } from "../provider.tsx";
import { useClaudeCatalog } from "./hooks/use-claude-catalog.ts";
import { useClaudeSession } from "./hooks/use-session.ts";
import { useClaude } from "./provider.tsx";
import { ClaudePromptBar } from "./prompt-bar.tsx";
import { ClaudeTimeline } from "./timeline.tsx";

const imageExt = /\.(png|jpe?g|gif|webp|heic|bmp|svg)$/i;

// ClaudePane 组合实录、审批提示与底栏；composeOnly 时只渲染新建页输入。
export function ClaudePane({
  sessionId,
  workspace,
  pickFiles,
  onOpenSession,
  onCreated,
  onPrepare,
  onNewConversation,
  onListChange,
  composeOnly = false,
}: {
  sessionId?: string;
  workspace?: string;
  pickFiles?: (options?: PickFilesOptions) => Promise<PickedLocalFile[]>;
  onOpenSession: (id: string) => void;
  /** 第一条消息已经发出、会话刚建好时调用，用来挂到 Work。 */
  onCreated?: (id: string) => Promise<void>;
  /** 会话刚建好、第一条消息发出前调用，用来先挂上 Issue/PR。 */
  onPrepare?: (id: string) => Promise<void>;
  onNewConversation: () => void;
  onListChange?: () => Promise<void>;
  composeOnly?: boolean;
}) {
  const catalog = useClaudeCatalog();
  const session = useClaudeSession(sessionId);
  const { client } = useClaude();
  const [composerError, setComposerError] = useState<string | null>(null);
  const [pageNotice, setPageNotice] = useState<string | null>(null);
  const [pendingSettings, setPendingSettings] = useState<ClaudeSettingsPatch>({});
  const [starting, setStarting] = useState(false);
  const [forking, setForking] = useState(false);

  const settings: ClaudeSettings = sessionId
    ? session.settings
    : {
        model: pendingSettings.model ?? "",
        effort: pendingSettings.effort ?? "",
        permission_mode: pendingSettings.permission_mode ?? "",
        cwd: workspace?.trim() || pendingSettings.cwd || "",
        overridden: pendingSettings.overridden ?? [],
      };
  const notice = session.notice ?? pageNotice;
  const fail = (err: unknown, fallback: string) => {
    setComposerError(err instanceof Error ? err.message : fallback);
  };

  const refreshList = async () => {
    await onListChange?.();
  };

  const openCreated = async (id: string) => {
    await refreshList();
    onOpenSession(id);
  };

  const createWithSettings = async () => {
    const created = await client.createSession();
    const cwd = workspace?.trim() || pendingSettings.cwd;
    const patch: ClaudeSettingsPatch = { ...pendingSettings };
    if (cwd) {
      patch.cwd = cwd;
    }
    if (patch.model || patch.effort || patch.permission_mode || patch.cwd) {
      await client.applySettings(created.id, patch);
    }
    await onPrepare?.(created.id);
    return created.id;
  };

  const applySettings = (patch: ClaudeSettingsPatch) => {
    if (!sessionId) {
      setPendingSettings((current) => ({
        ...current,
        ...patch,
        cwd: workspace?.trim() || patch.cwd || current.cwd,
      }));
      return;
    }
    void session.applySettings(patch);
  };

  const onSend = async (text: string, mode: "start" | "queue") => {
    setComposerError(null);
    if (sessionId) {
      await session.send(text, mode);
      await refreshList();
      return;
    }
    setStarting(true);
    try {
      const id = await createWithSettings();
      await client.startTurn(id, { content: text, mode });
      await onCreated?.(id);
      await openCreated(id);
    } catch (err) {
      fail(err, "无法开对话");
    } finally {
      setStarting(false);
    }
  };

  const onFork = async () => {
    if (!sessionId || forking) {
      return;
    }
    setForking(true);
    try {
      const forked = await session.fork();
      await refreshList();
      if (forked) {
        onOpenSession(forked.id);
      }
    } finally {
      setForking(false);
    }
  };

  const composer = (
    <ClaudePromptBar
      className={composeOnly ? "mx-0 max-w-none px-0 pb-0" : undefined}
      running={session.running}
      sending={session.sending || starting}
      sessionId={sessionId}
      workspace={workspace}
      pickFiles={pickFiles}
      commands={catalog.commands}
      models={catalog.models}
      modes={catalog.modes}
      settings={settings}
      onSend={onSend}
      onCancel={session.interrupt}
      onApply={applySettings}
      onCompact={async () => {
        await session.runCommand("compact");
      }}
      onRefreshCatalog={() => void catalog.refresh()}
      usage={session.usage}
      onAttachError={(message) => fail(new Error(message), message)}
      onAttach={async (files) => {
        setComposerError(null);
        const cwd = workspace?.trim();
        try {
          const id = sessionId ?? (await createWithSettings());
          const names: string[] = [];
          for (const file of files) {
            if (imageExt.test(file.path)) {
              if (id === sessionId) {
                await session.attachImage(file.path);
              } else {
                await client.attachImage(id, file.path);
              }
            } else {
              const path = mentionPath(file.path, cwd);
              if (id === sessionId) {
                await session.attachMention(path);
              } else {
                await client.mention(id, path);
              }
            }
            names.push(file.name);
          }
          if (id !== sessionId) {
            setPageNotice(`已挂 ${names.join("、")}，随下一条发送`);
            await openCreated(id);
          }
        } catch (err) {
          fail(err, "挂文件失败");
        }
      }}
      onCommand={async (name, args) => {
        setComposerError(null);
        try {
          if (!sessionId) {
            const id = await createWithSettings();
            const hint = await client.invoke(id, name, args);
            if (hint) {
              setPageNotice(hint);
            }
            await openCreated(id);
            return;
          }
          if (name === "archive") {
            await session.archive();
            await refreshList();
            onNewConversation();
            return;
          }
          if (name === "fork" || name === "branch") {
            await onFork();
            return;
          }
          if (name === "stop") {
            await session.interrupt();
            return;
          }
          await session.runCommand(name, args);
          await refreshList();
        } catch (err) {
          fail(err, "命令失败");
        }
      }}
    />
  );

  const banner =
    catalog.error || session.error || composerError || notice ? (
      <div
        className={
          catalog.error || session.error || composerError
            ? "flex shrink-0 items-center justify-between gap-2 border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-red-300"
            : "flex shrink-0 items-center justify-between gap-2 border-b border-border px-4 py-2 text-xs text-muted-foreground"
        }
      >
        <span>{catalog.error ?? session.error ?? composerError ?? notice}</span>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            session.setNotice(null);
            session.setError(null);
            setComposerError(null);
            setPageNotice(null);
          }}
        >
          关闭
        </Button>
      </div>
    ) : null;

  if (composeOnly) {
    return (
      <div className="w-full">
        {banner}
        {composer}
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      {banner}
      <ClaudeTimeline
        items={session.items}
        loading={session.loading}
        scrollKey={sessionId}
        canFork={Boolean(sessionId) && !forking && session.items.length > 0}
        onFork={onFork}
      />
      <div className="relative z-30 shrink-0">{composer}</div>
    </div>
  );
}

// mentionPath 尽量收成相对仓库根的路径，方便 Claude 当文件提及。
function mentionPath(abs: string, cwd?: string): string {
  const root = cwd?.trim();
  if (!root) {
    return abs;
  }
  const prefix = root.endsWith("/") || root.endsWith("\\") ? root : `${root}/`;
  if (abs.startsWith(prefix)) {
    return abs.slice(prefix.length);
  }
  return abs;
}

"use client";

import type { Settings } from "@codedock/core/codex";
import { Button } from "@codedock/ui";
import { useState } from "react";

import type { PickedLocalFile, PickFilesOptions } from "../provider.tsx";

import { CodexAskDock } from "./ask-dock.tsx";
import { useCodexCatalog } from "./hooks/use-codex-catalog.ts";
import { useCodexSession } from "./hooks/use-session.ts";
import { useCodex } from "./provider.tsx";
import { CodexPromptBar } from "./prompt-bar.tsx";
import { CodexTimeline } from "./timeline.tsx";

// CodexPane 组合 Codex 实录和底栏；composeOnly 时只渲染新建页输入，发出消息才建会话。
export function CodexPane({
  sessionId,
  workspace,
  pickFiles,
  onOpenSession,
  onCreated,
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
  onNewConversation: () => void;
  onListChange?: () => Promise<void>;
  composeOnly?: boolean;
}) {
  const catalog = useCodexCatalog();
  const session = useCodexSession(sessionId);
  const { client } = useCodex();
  const [composerError, setComposerError] = useState<string | null>(null);
  const [pageNotice, setPageNotice] = useState<string | null>(null);
  const [pendingSettings, setPendingSettings] = useState<Settings>({});
  const [starting, setStarting] = useState(false);
  const [forking, setForking] = useState(false);

  const settings = sessionId
    ? session.state.settings
    : { ...pendingSettings, cwd: workspace?.trim() || pendingSettings.cwd };
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
    const created = await client.createSession({
      ...pendingSettings,
      cwd: workspace?.trim() || pendingSettings.cwd,
    });
    return created.id;
  };

  const applySettings = (patch: Settings) => {
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
    <CodexPromptBar
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
      onCompact={() => session.compact()}
      onRefreshCatalog={() => void catalog.refresh()}
      usage={session.state.usage}
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
            const result = await client.invokeCommand(id, name, args);
            if (result.hint) {
              setPageNotice(result.hint);
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
          if (name === "fork") {
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
      <CodexTimeline
        state={session.state}
        loading={session.loading}
        scrollKey={sessionId}
        canFork={Boolean(sessionId) && !forking}
        onFork={onFork}
      />
      <div className="relative z-30 shrink-0">
        <CodexAskDock asks={session.state.asks} onDecide={session.decide} onExpire={session.expire} />
        {composer}
      </div>
    </div>
  );
}

const imageExt = /\.(png|jpe?g|gif|webp|heic|bmp|svg)$/i;

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

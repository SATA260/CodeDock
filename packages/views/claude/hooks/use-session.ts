"use client";

import type {
  ClaudeDecision,
  ClaudeProgress,
  ClaudeSession,
  ClaudeSettings,
  ClaudeSettingsPatch,
  ClaudeTimelineItem,
  ClaudeTokenUsage,
} from "@codedock/core/claude";
import { useCallback, useEffect, useRef, useState } from "react";

import { useClaude } from "../provider.tsx";

const emptySettings = (): ClaudeSettings => ({
  model: "",
  effort: "",
  permission_mode: "",
  cwd: "",
  overridden: [],
});

// useClaudeSession 读一条对话的实录与生效配置；进行中只轮询 hydrate，滚动跟随交给 Conversation 的 ResizeObserver。
export function useClaudeSession(sessionId: string | undefined) {
  const { client } = useClaude();
  const [session, setSession] = useState<ClaudeSession | null>(null);
  const [items, setItems] = useState<ClaudeTimelineItem[]>([]);
  const [usage, setUsage] = useState<ClaudeTokenUsage | undefined>(undefined);
  const [settings, setSettings] = useState<ClaudeSettings>(emptySettings);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [loading, setLoading] = useState(Boolean(sessionId));
  const sessionRef = useRef<ClaudeSession | null>(null);
  sessionRef.current = session;

  const hydrate = useCallback(
    async (id: string, signal?: AbortSignal) => {
      const [nextSession, transcript, nextSettings] = await Promise.all([
        client.getSession(id),
        client.hydrate(id),
        client.getSettings(id).catch(() => emptySettings()),
      ]);
      if (signal?.aborted) {
        return;
      }
      setSession(nextSession);
      setItems(withIds(transcript.items, Boolean(nextSession.active_turn_id)));
      setUsage(transcript.usage);
      setSettings(nextSettings);
      setLoading(false);
      setError(null);
    },
    [client],
  );

  useEffect(() => {
    if (!sessionId) {
      setSession(null);
      setItems([]);
      setUsage(undefined);
      setSettings(emptySettings());
      setLoading(false);
      setError(null);
      setNotice(null);
      return;
    }
    setLoading(true);
    setSession(null);
    setItems([]);
    setUsage(undefined);
    setSettings(emptySettings());
    setError(null);
    setNotice(null);
    const ac = new AbortController();
    let cancelled = false;
    void hydrate(sessionId, ac.signal).catch((err: unknown) => {
      if (cancelled || ac.signal.aborted) {
        return;
      }
      setLoading(false);
      setError(err instanceof Error ? err.message : "无法读取 Claude 对话");
    });
    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [hydrate, sessionId]);

  useEffect(() => {
    if (!sessionId || !session?.active_turn_id) {
      return;
    }
    const timer = window.setInterval(() => {
      void hydrate(sessionId).catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "无法刷新实录");
      });
    }, 800);
    return () => window.clearInterval(timer);
  }, [hydrate, session?.active_turn_id, sessionId]);

  const send = useCallback(
    async (content: string, mode: "start" | "queue") => {
      if (!sessionId || !content.trim()) {
        return;
      }
      setSending(true);
      setItems((current) => [...current, optimisticUser(content, current.length)]);
      try {
        const turnId = await client.startTurn(sessionId, {
          content,
          input: { text: content, mentions: [], images: [] },
          mode,
        });
        setSession((current) =>
          current && current.id === sessionId && mode !== "queue"
            ? { ...current, active_turn_id: turnId }
            : current,
        );
        await hydrate(sessionId);
        setError(null);
      } catch (err) {
        setItems((current) => current.filter((item) => !item.id.startsWith("optimistic:")));
        setError(err instanceof Error ? err.message : "发送失败");
      } finally {
        setSending(false);
      }
    },
    [client, hydrate, sessionId],
  );

  const interrupt = useCallback(async () => {
    const turnId = sessionRef.current?.active_turn_id;
    if (!turnId) {
      return;
    }
    try {
      await client.cancelTurn(turnId);
      if (sessionId) {
        await hydrate(sessionId);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "打断失败");
    }
  }, [client, hydrate, sessionId]);

  const applySettings = useCallback(
    async (patch: ClaudeSettingsPatch) => {
      if (!sessionId) {
        return;
      }
      try {
        const next = await client.applySettings(sessionId, patch);
        setSettings(next);
      } catch (err) {
        setError(err instanceof Error ? err.message : "保存设置失败");
      }
    },
    [client, sessionId],
  );

  const runCommand = useCallback(
    async (name: string, args = "") => {
      if (!sessionId) {
        return "";
      }
      try {
        const hint = await client.invoke(sessionId, name, args);
        if (hint) {
          setNotice(hint);
        }
        await hydrate(sessionId);
        return hint;
      } catch (err) {
        setError(err instanceof Error ? err.message : "命令失败");
        return "";
      }
    },
    [client, hydrate, sessionId],
  );

  const attachMention = useCallback(
    async (path: string) => {
      if (!sessionId) {
        return;
      }
      try {
        await client.mention(sessionId, path);
        setNotice(`已挂文件 ${path}，随下一条发送`);
      } catch (err) {
        setError(err instanceof Error ? err.message : "挂文件失败");
      }
    },
    [client, sessionId],
  );

  const attachImage = useCallback(
    async (path: string) => {
      if (!sessionId) {
        return;
      }
      try {
        await client.attachImage(sessionId, path);
        setNotice(`已挂图片 ${path}，随下一条发送`);
      } catch (err) {
        setError(err instanceof Error ? err.message : "挂图片失败");
      }
    },
    [client, sessionId],
  );

  const fork = useCallback(async () => {
    if (!sessionId) {
      return undefined;
    }
    try {
      return await client.forkSession(sessionId);
    } catch (err) {
      setError(err instanceof Error ? err.message : "fork 失败");
      return undefined;
    }
  }, [client, sessionId]);

  const archive = useCallback(async () => {
    if (!sessionId) {
      return;
    }
    try {
      await client.archiveSession(sessionId);
    } catch (err) {
      setError(err instanceof Error ? err.message : "归档失败");
    }
  }, [client, sessionId]);

  const decide = useCallback(
    async (approvalId: string, answer: ClaudeDecision) => {
      try {
        await client.decide(approvalId, answer);
        if (sessionId) {
          await hydrate(sessionId);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "作答失败");
      }
    },
    [client, hydrate, sessionId],
  );

  const continueTurn = useCallback(async () => {
    const turnId = sessionRef.current?.active_turn_id;
    if (!turnId) {
      return;
    }
    try {
      await client.continueTurn(turnId);
      if (sessionId) {
        await hydrate(sessionId);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "继续失败");
    }
  }, [client, hydrate, sessionId]);

  return {
    session,
    items,
    usage,
    settings,
    error,
    setError,
    notice,
    setNotice,
    sending,
    loading,
    running: Boolean(session?.active_turn_id),
    send,
    interrupt,
    applySettings,
    runCommand,
    attachMention,
    attachImage,
    fork,
    archive,
    decide,
    continueTurn,
  };
}

// withIds 给实录行补稳定 id；进行中把最后一条非用户行标成 streaming，供时间线跟随。
function withIds(items: ClaudeProgress[], running: boolean): ClaudeTimelineItem[] {
  return items.map((item, index) => ({
    ...item,
    text: item.text ?? "",
    command: item.command ?? "",
    paths: item.paths ?? [],
    diff: item.diff ?? "",
    id: `${index}:${item.kind}`,
    streaming: running && index === items.length - 1 && item.kind !== "user",
  }));
}

// optimisticUser 在 hydrate 回来前先把用户这句话贴上。
function optimisticUser(text: string, index: number): ClaudeTimelineItem {
  return {
    id: `optimistic:${index}`,
    kind: "user",
    text,
    command: "",
    paths: [],
    diff: "",
  };
}

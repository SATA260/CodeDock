"use client";

import {
  applyCodexEvent,
  applyOptimisticUser,
  dropOptimisticUser,
  emptyCodexState,
  hydrateCodex,
  isLiveTurn,
  watchCodexEvents,
  type ApprovalAsk,
  type AskAnswer,
  type CodexViewState,
  type InputMode,
  type SessionDetail,
  type Settings,
} from "@codedock/core/codex";
import { useCallback, useEffect, useRef, useState } from "react";

import { useCodex } from "../provider.tsx";

export function useCodexSession(sessionId: string | undefined) {
  const { client } = useCodex();
  const [state, setState] = useState<CodexViewState>(emptyCodexState);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [loading, setLoading] = useState(Boolean(sessionId));
  const stateRef = useRef(state);
  stateRef.current = state;

  const hydrate = useCallback(
    async (id: string, signal?: AbortSignal) => {
      const [detail, settings, asks] = await Promise.all([
        client.getSession(id, signal).catch((err: unknown): SessionDetail => {
          if (!isEmptyThread(err)) {
            throw err;
          }
          return { session: { id, thread_id: id, archived: false }, progress: [], asks: [] };
        }),
        client.getSettings(id).catch(() => ({}) as Settings),
        client.listAsks(id).catch(() => [] as ApprovalAsk[]),
      ]);
      if (signal?.aborted) {
        return;
      }
      const next = hydrateCodex({ ...detail, asks, settings, usage: detail.usage ?? stateRef.current.usage });
      next.lastSeq = Math.max(stateRef.current.lastSeq, next.lastSeq);
      stateRef.current = next;
      setState(next);
      setLoading(false);
      setError(null);
    },
    [client],
  );

  useEffect(() => {
    if (!sessionId) {
      setState(emptyCodexState());
      setLoading(false);
      setError(null);
      setNotice(null);
      return;
    }
    setLoading(true);
    const empty = emptyCodexState();
    stateRef.current = empty;
    setState(empty);
    setError(null);
    setNotice(null);
    const ac = new AbortController();
    let cancelled = false;
    void (async () => {
      try {
        await hydrate(sessionId, ac.signal);
        await watchCodexEvents({
          baseUrl: client.baseUrl,
          sessionId,
          getAfterSeq: () => stateRef.current.lastSeq,
          onEvent: (event) => {
            setState((current) => {
              const next = applyCodexEvent(current, event);
              if (next.reset) {
                void hydrate(sessionId);
              }
              return next;
            });
          },
          signal: ac.signal,
        });
      } catch (err) {
        if (cancelled || ac.signal.aborted) {
          return;
        }
        setLoading(false);
        setError(err instanceof Error ? err.message : "无法订阅 Codex 事件");
      }
    })();
    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [client, hydrate, sessionId]);

  const send = useCallback(
    async (content: string, mode: InputMode) => {
      if (!sessionId || !content.trim()) {
        return;
      }
      setSending(true);
      setState((current) => applyOptimisticUser(current, content));
      try {
        const turn = await client.startTurn(sessionId, { content, mode });
        setState((current) => ({
          ...current,
          activeTurn: isLiveTurn(turn.status) ? turn : current.activeTurn,
        }));
        setError(null);
      } catch (err) {
        setState((current) => dropOptimisticUser(current));
        setError(err instanceof Error ? err.message : "发送失败");
      } finally {
        setSending(false);
      }
    },
    [client, sessionId],
  );

  const interrupt = useCallback(async () => {
    const turn = stateRef.current.activeTurn;
    if (!sessionId || !turn) {
      return;
    }
    try {
      await client.interruptTurn(turn.id, sessionId);
    } catch (err) {
      setError(err instanceof Error ? err.message : "打断失败");
    }
  }, [client, sessionId]);

  const decide = useCallback(
    async (requestId: string, answer: AskAnswer) => {
      try {
        await client.decideAsk(requestId, answer);
      } catch (err) {
        setError(err instanceof Error ? err.message : "作答失败");
      }
    },
    [client],
  );

  const expire = useCallback(
    async (requestId: string) => {
      try {
        await client.expireAsk(requestId);
      } catch (err) {
        setError(err instanceof Error ? err.message : "过期失败");
      }
    },
    [client],
  );

  const applySettings = useCallback(
    async (patch: Settings) => {
      if (!sessionId) {
        return;
      }
      try {
        const settings = await client.applySettings(sessionId, patch);
        setState((current) => ({ ...current, settings }));
      } catch (err) {
        setError(err instanceof Error ? err.message : "保存设置失败");
      }
    },
    [client, sessionId],
  );

  const runCommand = useCallback(
    async (name: string, args = "") => {
      if (!sessionId) {
        return;
      }
      try {
        const result = await client.invokeCommand(sessionId, name, args);
        if (result.hint) {
          setNotice(result.hint);
        }
        if (name === "fork") {
          return result;
        }
        await hydrate(sessionId);
      } catch (err) {
        setError(err instanceof Error ? err.message : "命令失败");
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

  const rename = useCallback(
    async (title: string) => {
      if (!sessionId) {
        return;
      }
      try {
        await client.renameSession(sessionId, title);
        await hydrate(sessionId);
      } catch (err) {
        setError(err instanceof Error ? err.message : "改标题失败");
      }
    },
    [client, hydrate, sessionId],
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

  const compact = useCallback(async () => {
    if (!sessionId) {
      return;
    }
    try {
      await client.compactSession(sessionId);
      setNotice("已请 Codex 压缩这条对话");
    } catch (err) {
      setError(err instanceof Error ? err.message : "压缩失败");
    }
  }, [client, sessionId]);

  const review = useCallback(async () => {
    if (!sessionId) {
      return;
    }
    try {
      await client.reviewSession(sessionId);
      setNotice("已请 Codex 评审未提交改动");
    } catch (err) {
      setError(err instanceof Error ? err.message : "评审失败");
    }
  }, [client, sessionId]);

  const refreshAsks = useCallback(async () => {
    if (!sessionId) {
      return;
    }
    const asks = await client.listAsks(sessionId);
    setState((current) => ({ ...current, asks }));
  }, [client, sessionId]);

  const running = isLiveTurn(state.activeTurn?.status);

  return {
    state,
    error,
    setError,
    notice,
    setNotice,
    sending,
    loading,
    running,
    send,
    interrupt,
    decide,
    expire,
    applySettings,
    runCommand,
    attachMention,
    attachImage,
    rename,
    fork,
    archive,
    compact,
    review,
    refreshAsks,
  };
}

function isEmptyThread(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err);
  return msg.includes("not materialized") || msg.includes("includeTurns") || msg.includes("thread not loaded");
}

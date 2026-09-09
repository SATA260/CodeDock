"use client";

import {
  applyEvent,
  applyOptimisticUser,
  dropOptimisticUser,
  emptyState,
  hydrate,
  isRecoverableRun,
  isTerminalRun,
  watchEvents,
  type AgentMode,
  type ApprovalDecision,
  type SessionState,
} from "@codedock/core/chat";
import { useCallback, useEffect, useRef, useState } from "react";

import { useAgent } from "../../provider.tsx";

const CACHE_LIMIT = 30;
const timelineCache = new Map<string, SessionState>();

function snapshot(state: SessionState): SessionState {
  return {
    lastSeq: state.lastSeq,
    activeRunId: state.activeRunId,
    runStatus: state.runStatus,
    items: state.items.slice(),
    messages: { ...state.messages },
  };
}

function cacheGet(sessionId: string): SessionState | undefined {
  const cached = timelineCache.get(sessionId);
  return cached ? snapshot(cached) : undefined;
}

function cacheSet(sessionId: string, state: SessionState) {
  if (timelineCache.has(sessionId)) {
    timelineCache.delete(sessionId);
  }
  timelineCache.set(sessionId, snapshot(state));
  if (timelineCache.size > CACHE_LIMIT) {
    const oldest = timelineCache.keys().next().value;
    if (oldest) {
      timelineCache.delete(oldest);
    }
  }
}

export function useSessionTimeline(sessionId: string | undefined) {
  const { client, userId } = useAgent();
  const [state, setState] = useState<SessionState>(() =>
    sessionId ? (cacheGet(sessionId) ?? emptyState()) : emptyState(),
  );
  const [error, setError] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [loading, setLoading] = useState(() => {
    if (!sessionId) {
      return false;
    }
    return !timelineCache.has(sessionId);
  });
  const [recoverableRunId, setRecoverableRunId] = useState<string | null>(null);
  const stateRef = useRef(state);
  const sessionRef = useRef(sessionId);
  stateRef.current = state;
  sessionRef.current = sessionId;

  useEffect(() => {
    if (!sessionId) {
      setState(emptyState());
      setLoading(false);
      setError(null);
      setRecoverableRunId(null);
      return;
    }

    const cached = cacheGet(sessionId);
    if (cached) {
      stateRef.current = cached;
      setState(cached);
      setLoading(false);
      setError(null);
    } else {
      setLoading(true);
    }

    const ac = new AbortController();
    let cancelled = false;
    void (async () => {
      try {
        const [messagesResult, eventsResult, sessionResult] = await Promise.allSettled([
          client.listMessages(sessionId, ac.signal),
          client.listEvents(sessionId, 0, ac.signal),
          client.getSession(sessionId),
        ]);
        if (sessionResult.status === "fulfilled" && sessionResult.value.active_run_id) {
          try {
            const run = await client.getRun(sessionResult.value.active_run_id);
            if (!cancelled && isRecoverableRun(run.status)) {
              setRecoverableRunId(run.id);
            } else if (!cancelled) {
              setRecoverableRunId(null);
            }
          } catch {
            if (!cancelled) {
              setRecoverableRunId(null);
            }
          }
        } else if (!cancelled) {
          setRecoverableRunId(null);
        }
        if (cancelled || ac.signal.aborted) {
          return;
        }
        if (messagesResult.status === "rejected") {
          if (!cached) {
            throw messagesResult.reason;
          }
        } else {
          const events = eventsResult.status === "fulfilled" ? eventsResult.value : [];
          const next = hydrate(messagesResult.value, events);
          cacheSet(sessionId, next);
          stateRef.current = next;
          setState(next);
          setLoading(false);
          setError(null);
        }
        await watchEvents({
          baseUrl: client.baseUrl,
          sessionId,
          getAfterSeq: () => stateRef.current.lastSeq,
          onEvent: (event) => {
            setState((current) => {
              const next = applyEvent(current, event);
              if (sessionRef.current === sessionId) {
                cacheSet(sessionId, next);
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
        setError(err instanceof Error ? err.message : "无法订阅会话事件");
      }
    })();
    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [client, sessionId]);

  const send = useCallback(
    async (content: string, mode: AgentMode) => {
      if (!sessionId || !content.trim()) {
        return;
      }
      setSending(true);
      setState((current) => {
        const next = applyOptimisticUser(current, { runId: "local", text: content });
        cacheSet(sessionId, next);
        return next;
      });
      try {
        await client.startRun(sessionId, { content, mode });
        setError(null);
      } catch (err) {
        setState((current) => {
          const next = dropOptimisticUser(current, "local");
          cacheSet(sessionId, next);
          return next;
        });
        setError(err instanceof Error ? err.message : "发送失败");
      } finally {
        setSending(false);
      }
    },
    [client, sessionId],
  );

  const cancel = useCallback(async () => {
    const runId = stateRef.current.activeRunId;
    if (!runId) {
      return;
    }
    try {
      await client.cancelRun(runId);
    } catch (err) {
      setError(err instanceof Error ? err.message : "取消失败");
    }
  }, [client]);

  const decide = useCallback(
    async (approvalId: string, decisions: ApprovalDecision[]) => {
      try {
        await client.decideApproval(approvalId, {
          decisions,
          actor_id: userId,
        });
      } catch (err) {
        setError(err instanceof Error ? err.message : "审批失败");
      }
    },
    [client, userId],
  );

  const recover = useCallback(async (runId?: string) => {
    const id = runId ?? recoverableRunId ?? stateRef.current.activeRunId;
    if (!id) {
      return;
    }
    try {
      await client.continueRun(id);
      setRecoverableRunId(null);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "恢复失败");
    }
  }, [client, recoverableRunId]);

  const running = Boolean(state.runStatus && !isTerminalRun(state.runStatus));
  const canRecover = Boolean(
    recoverableRunId &&
      state.runStatus &&
      isRecoverableRun(state.runStatus),
  ) || Boolean(recoverableRunId && !state.runStatus);

  return { state, error, sending, running, loading, canRecover, recoverableRunId, send, cancel, decide, recover };
}

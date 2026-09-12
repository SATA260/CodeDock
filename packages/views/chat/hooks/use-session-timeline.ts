"use client";

import {
  applyApprovalRecord,
  applyApprovals,
  applyEvent,
  decisionsForApproval,
  applyOptimisticUser,
  applyUserText,
  decodeText,
  dropOptimisticUser,
  emptyState,
  hydrate,
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
    setRecoverableRunId(null);
    if (!sessionId) {
      const empty = emptyState();
      stateRef.current = empty;
      setState(empty);
      setLoading(false);
      setError(null);
      return;
    }

    const cached = cacheGet(sessionId);
    if (cached) {
      stateRef.current = cached;
      setState(cached);
      setLoading(false);
      setError(null);
    } else {
      const empty = emptyState();
      stateRef.current = empty;
      setState(empty);
      setLoading(true);
      setError(null);
    }

    const ac = new AbortController();
    let cancelled = false;
    const applySessionRecover = (needsRecover: boolean | undefined, runId: string | undefined) => {
      if (cancelled || sessionRef.current !== sessionId) {
        return;
      }
      setRecoverableRunId(needsRecover && runId ? runId : null);
    };
    void (async () => {
      try {
        const [messagesResult, eventsResult, sessionResult, approvalsResult] = await Promise.allSettled([
          client.listMessages(sessionId, ac.signal),
          client.listEvents(sessionId, 0, ac.signal),
          client.getSession(sessionId, ac.signal),
          client.listApprovals(sessionId, ac.signal),
        ]);
        let recoverAfterSeq = 0;
        if (sessionResult.status === "fulfilled") {
          recoverAfterSeq = sessionResult.value.last_event_seq;
          applySessionRecover(sessionResult.value.needs_recover, sessionResult.value.active_run_id);
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
          const approvals = approvalsResult.status === "fulfilled" ? approvalsResult.value : [];
          const next = applyApprovals(hydrate(messagesResult.value, events), approvals);
          cacheSet(sessionId, next);
          stateRef.current = next;
          setState(next);
          setLoading(false);
          setError(null);
        }
        await watchEvents({
          baseUrl: client.baseUrl,
          sessionId,
          getAfterSeq: () =>
            sessionRef.current === sessionId ? stateRef.current.lastSeq : Number.MAX_SAFE_INTEGER,
          onEvent: (event) => {
            if (sessionRef.current !== sessionId) {
              return;
            }
            if (event.run_id && event.seq > recoverAfterSeq) {
              setRecoverableRunId(null);
            }
            setState((current) => {
              if (sessionRef.current !== sessionId) {
                return current;
              }
              const next = applyEvent(current, event);
              cacheSet(sessionId, next);
              return next;
            });
          },
          onStreamEnd: async () => {
            if (sessionRef.current !== sessionId || cancelled) {
              return;
            }
            try {
              const session = await client.getSession(sessionId, ac.signal);
              applySessionRecover(session.needs_recover, session.active_run_id);
            } catch {
              // 断连后探测失败等下次重试
            }
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
      const pendingRunId = `local:${crypto.randomUUID()}`;
      setSending(true);
      setState((current) => {
        if (sessionRef.current !== sessionId) {
          return current;
        }
        const next = applyOptimisticUser(current, { runId: pendingRunId, text: content });
        cacheSet(sessionId, next);
        return next;
      });
      try {
        await client.startRun(sessionId, { content, mode });
        setRecoverableRunId(null);
        setError(null);
      } catch (err) {
        setState((current) => {
          if (sessionRef.current !== sessionId) {
            return current;
          }
          const next = dropOptimisticUser(current, pendingRunId);
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
      if (!sessionId) {
        return;
      }
      try {
        let covered = decisions;
        try {
          const latest = await client.getApproval(approvalId);
          covered = decisionsForApproval(latest, decisions);
        } catch {
          covered = decisions;
        }
        const approval = await client.decideApproval(approvalId, {
          decisions: covered,
          actor_id: userId,
        });
        setState((current) => {
          if (sessionRef.current !== sessionId) {
            return current;
          }
          const next = applyApprovalRecord(current, approval);
          cacheSet(sessionId, next);
          return next;
        });
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "审批失败");
      }
    },
    [client, sessionId, userId],
  );

  const editQueued = useCallback(
    async (messageId: string, content: string) => {
      if (!sessionId) {
        return;
      }
      try {
        const message = await client.updateMessage(sessionId, messageId, content);
        setState((current) => {
          if (sessionRef.current !== sessionId) {
            return current;
          }
          const next = applyUserText(current, messageId, decodeText(message.content), true);
          cacheSet(sessionId, next);
          return next;
        });
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "无法改排队消息");
        throw err;
      }
    },
    [client, sessionId],
  );

  const recover = useCallback(async (runId?: string) => {
    const id = runId ?? recoverableRunId ?? stateRef.current.activeRunId;
    if (!id) {
      return;
    }
    try {
      await client.continueRun(id);
      if (!runId || runId === recoverableRunId) {
        setRecoverableRunId(null);
      }
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "恢复失败");
    }
  }, [client, recoverableRunId]);

  const running = Boolean(state.runStatus && !isTerminalRun(state.runStatus));
  const canRecover = Boolean(
    recoverableRunId &&
      state.runStatus !== "waiting_approval" &&
      state.runStatus !== "cancelling" &&
      (!state.runStatus || !isTerminalRun(state.runStatus)) &&
      (!state.activeRunId || state.activeRunId === recoverableRunId),
  );

  return { state, error, sending, running, loading, canRecover, recoverableRunId, send, cancel, decide, editQueued, recover };
}

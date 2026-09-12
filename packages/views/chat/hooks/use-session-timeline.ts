"use client";

import {
  applyApprovalRecord,
  applyApprovals,
  applyEvent,
  decisionsForApproval,
  applyLocalCancel,
  applyOptimisticUser,
  dropOptimisticUser,
  emptyState,
  hydrate,
  isTerminalRun,
  joinQueuedTexts,
  watchEvents,
  type ApprovalMode,
  type WorkMode,
  type ApprovalDecision,
  type SessionState,
  type TimelineItem,
} from "@codedock/core/chat";
import { useCallback, useEffect, useRef, useState } from "react";

import { useAgent } from "../../provider.tsx";

const CACHE_LIMIT = 30;
const timelineCache = new Map<string, SessionState>();
const pendingCache = new Map<string, PendingFollowup[]>();

export type PendingFollowup = {
  id: string;
  text: string;
  mode: WorkMode;
  approval: ApprovalMode;
};

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

function pendingGet(sessionId: string): PendingFollowup[] {
  return pendingCache.get(sessionId)?.map((item) => ({ ...item })) ?? [];
}

function pendingSet(sessionId: string, items: PendingFollowup[]) {
  pendingCache.set(sessionId, items.map((item) => ({ ...item })));
}

function isRunning(state: SessionState): boolean {
  return Boolean(state.runStatus && !isTerminalRun(state.runStatus));
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
  const [workspaceId, setWorkspaceId] = useState<string | null>(null);
  const [recoverableRunId, setRecoverableRunId] = useState<string | null>(null);
  const [pending, setPending] = useState<PendingFollowup[]>(() =>
    sessionId ? pendingGet(sessionId) : [],
  );
  const [editingId, setEditingId] = useState<string | null>(null);
  const stateRef = useRef(state);
  const sessionRef = useRef(sessionId);
  const pendingRef = useRef(pending);
  const editingRef = useRef(editingId);
  const flushingRef = useRef(false);
  stateRef.current = state;
  sessionRef.current = sessionId;
  pendingRef.current = pending;
  editingRef.current = editingId;

  const writePending = useCallback(
    (items: PendingFollowup[]) => {
      if (sessionId) {
        pendingSet(sessionId, items);
      }
      pendingRef.current = items;
      setPending(items);
    },
    [sessionId],
  );

  const restorePending = useCallback(
    (snapshot: PendingFollowup[]) => {
      const seen = new Set(snapshot.map((item) => item.id));
      const extras = pendingRef.current.filter((item) => !seen.has(item.id));
      writePending([...snapshot, ...extras]);
    },
    [writePending],
  );

  useEffect(() => {
    writePending(sessionId ? pendingGet(sessionId) : []);
    setEditingId(null);
    editingRef.current = null;
  }, [sessionId, writePending]);

  useEffect(() => {
    setRecoverableRunId(null);
    if (!sessionId) {
      const empty = emptyState();
      stateRef.current = empty;
      setState(empty);
      setWorkspaceId(null);
      setLoading(false);
      setError(null);
      return;
    }
    setWorkspaceId(null);

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
          if (!cancelled && sessionRef.current === sessionId) {
            setWorkspaceId(sessionResult.value.workspace_id || null);
          }
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

  const startTurn = useCallback(
    async (content: string, mode: WorkMode, approval: ApprovalMode) => {
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
        await client.startRun(sessionId, { content, mode, approval });
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
        throw err;
      } finally {
        setSending(false);
      }
    },
    [client, sessionId],
  );

  const send = useCallback(
    async (content: string, mode: WorkMode, approval: ApprovalMode) => {
      if (!sessionId || !content.trim()) {
        return;
      }
      if (isRunning(stateRef.current) || flushingRef.current) {
        writePending([...pendingRef.current, { id: crypto.randomUUID(), text: content, mode, approval }]);
        return;
      }
      flushingRef.current = true;
      try {
        await startTurn(content, mode, approval);
      } catch {
        // 错误已写入 state
      } finally {
        flushingRef.current = false;
      }
    },
    [sessionId, startTurn, writePending],
  );

  const beginEditPending = useCallback((id: string) => {
    setEditingId(id);
    editingRef.current = id;
  }, []);

  const cancelEditPending = useCallback(() => {
    setEditingId(null);
    editingRef.current = null;
  }, []);

  const savePending = useCallback(
    (id: string, text: string) => {
      const next = text.trim();
      if (!next) {
        return;
      }
      writePending(
        pendingRef.current.map((item) => (item.id === id ? { ...item, text: next } : item)),
      );
      setEditingId(null);
      editingRef.current = null;
    },
    [writePending],
  );

  const deletePending = useCallback(
    (id: string) => {
      writePending(pendingRef.current.filter((item) => item.id !== id));
      if (editingRef.current === id) {
        setEditingId(null);
        editingRef.current = null;
      }
    },
    [writePending],
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

  const denyPendingApprovals = useCallback(async () => {
    if (!sessionId) {
      return;
    }
    const pendingApprovals = stateRef.current.items.filter(
      (item): item is Extract<TimelineItem, { kind: "approval" }> =>
        item.kind === "approval" && item.status === "pending",
    );
    for (const item of pendingApprovals) {
      const decisions: ApprovalDecision[] = item.toolCalls
        .filter((call) => call.id)
        .map((call) => ({
          tool_call_id: call.id,
          status: "denied",
          reason: "发送排队消息，已中断审批",
        }));
      if (decisions.length === 0) {
        continue;
      }
      try {
        await decide(item.approvalId, decisions);
      } catch {
        // 单条审批失败不挡住后续拒绝与发送
      }
    }
  }, [decide, sessionId]);

  const flushPending = useCallback(async () => {
    if (!sessionId || flushingRef.current || editingRef.current) {
      return;
    }
    const items = pendingRef.current;
    const joined = joinQueuedTexts(items.map((item) => item.text));
    if (!joined) {
      return;
    }
    const last = items[items.length - 1];
    const mode = last?.mode ?? "agent";
    const approval = last?.approval ?? "manual";
    flushingRef.current = true;
    writePending([]);
    try {
      await denyPendingApprovals();
      await startTurn(joined, mode, approval);
    } catch {
      restorePending(items);
    } finally {
      flushingRef.current = false;
    }
  }, [denyPendingApprovals, restorePending, sessionId, startTurn, writePending]);

  useEffect(() => {
    if (!sessionId || !state.runStatus || !isTerminalRun(state.runStatus)) {
      return;
    }
    if (editingId || pending.length === 0) {
      return;
    }
    void flushPending();
  }, [editingId, flushPending, pending.length, sessionId, state.runStatus]);

  const sendNow = useCallback(async () => {
    if (!sessionId || flushingRef.current || editingRef.current) {
      return;
    }
    const items = pendingRef.current;
    const joined = joinQueuedTexts(items.map((item) => item.text));
    if (!joined) {
      return;
    }
    const last = items[items.length - 1];
    const mode = last?.mode ?? "agent";
    const approval = last?.approval ?? "manual";
    flushingRef.current = true;
    writePending([]);
    try {
      const runId = stateRef.current.activeRunId;
      if (runId && isRunning(stateRef.current)) {
        // 先取消，避免拒绝审批后 RecoverRun 把旧轮拉起来再 409
        await client.cancelRun(runId);
        if (sessionId) {
          setState((current) => {
            if (sessionRef.current !== sessionId) {
              return current;
            }
            const next = applyLocalCancel(current, runId);
            stateRef.current = next;
            cacheSet(sessionId, next);
            return next;
          });
        }
      }
      await denyPendingApprovals();
      await startTurn(joined, mode, approval);
    } catch (err) {
      restorePending(items);
      setError(err instanceof Error ? err.message : "发送失败");
    } finally {
      flushingRef.current = false;
    }
  }, [client, denyPendingApprovals, restorePending, sessionId, startTurn, writePending]);

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

  const running = isRunning(state);
  const canRecover = Boolean(
    recoverableRunId &&
      state.runStatus !== "waiting_approval" &&
      state.runStatus !== "cancelling" &&
      (!state.runStatus || !isTerminalRun(state.runStatus)) &&
      (!state.activeRunId || state.activeRunId === recoverableRunId),
  );

  return {
    state,
    error,
    sending,
    running,
    loading,
    workspaceId,
    canRecover,
    recoverableRunId,
    pending,
    editingId,
    send,
    sendNow,
    beginEditPending,
    cancelEditPending,
    savePending,
    deletePending,
    cancel,
    decide,
    recover,
  };
}

"use client";

import {
  applyEvent,
  applyOptimisticUser,
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

export function useSessionTimeline(sessionId: string | undefined) {
  const { client, userId } = useAgent();
  const [state, setState] = useState<SessionState>(emptyState());
  const [error, setError] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const stateRef = useRef(state);
  stateRef.current = state;

  useEffect(() => {
    if (!sessionId) {
      setState(emptyState());
      return;
    }
    const ac = new AbortController();
    let cancelled = false;
    void (async () => {
      try {
        const messages = await client.listMessages(sessionId);
        if (cancelled) {
          return;
        }
        setState(hydrate(messages, []));
        setError(null);
        await watchEvents({
          baseUrl: client.baseUrl,
          sessionId,
          getAfterSeq: () => stateRef.current.lastSeq,
          onEvent: (event) => {
            setState((current) => applyEvent(current, event));
          },
          signal: ac.signal,
        });
      } catch (err) {
        if (!ac.signal.aborted) {
          setError(err instanceof Error ? err.message : "无法订阅会话事件");
        }
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
      setState((current) => applyOptimisticUser(current, { runId: "local", text: content }));
      try {
        await client.startRun(sessionId, { content, mode });
        setError(null);
      } catch (err) {
        setState((current) => dropOptimisticUser(current, "local"));
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

  const running = Boolean(state.runStatus && !isTerminalRun(state.runStatus));

  return { state, error, sending, running, send, cancel, decide };
}

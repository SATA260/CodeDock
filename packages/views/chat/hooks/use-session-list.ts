"use client";

import type { Session } from "@codedock/core/chat";
import { useCallback, useEffect, useState } from "react";

import { useAgent } from "../../provider.tsx";

export function useSessionList() {
  const { client, userId } = useAgent();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const listed = await client.listSessions();
      setSessions(listed.sessions.filter((session) => session.last_event_seq > 0));
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法加载会话");
    }
  }, [client]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const createSession = useCallback(async () => {
    setBusy(true);
    try {
      const session = await client.createSession({ user_id: userId });
      await refresh();
      return session;
    } finally {
      setBusy(false);
    }
  }, [client, refresh, userId]);

  return { sessions, error, busy, refresh, createSession };
}

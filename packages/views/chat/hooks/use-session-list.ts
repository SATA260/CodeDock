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
      const visible: Session[] = [];
      let page = 1;
      while (true) {
        const listed = await client.listSessions(page, 50);
        visible.push(
          ...listed.sessions.filter(
            (session) => session.status !== "archived" && session.last_event_seq > 0,
          ),
        );
        const pageSize = listed.page.page_size || listed.sessions.length || 50;
        if (listed.sessions.length === 0 || page * pageSize >= listed.page.total) {
          break;
        }
        page += 1;
      }
      setSessions(visible);
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

  const removeSession = useCallback(
    async (session: Session) => {
      setBusy(true);
      try {
        if (session.active_run_id) {
          try {
            await client.cancelRun(session.active_run_id);
          } catch {
            // 已结束则继续归档
          }
        }
        await client.archiveSession(session.id);
        await refresh();
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "删除失败");
        throw err;
      } finally {
        setBusy(false);
      }
    },
    [client, refresh],
  );

  return { sessions, error, busy, refresh, createSession, removeSession };
}

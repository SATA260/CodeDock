"use client";

import type { Session, Settings } from "@codedock/core/codex";
import { useCallback, useEffect, useState } from "react";

import { useCodex } from "../provider.tsx";

export function useCodexSessionList() {
  const { client } = useCodex();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [archived, setArchived] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const page = await client.listSessions({ archived });
      setSessions(uniqueSessions(page.sessions));
      setCursor(page.next_cursor);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法加载 Codex 对话");
    } finally {
      setLoaded(true);
    }
  }, [archived, client]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const loadMore = useCallback(async () => {
    if (!cursor) {
      return;
    }
    try {
      const page = await client.listSessions({ archived, cursor });
      setSessions((current) => uniqueSessions([...current, ...page.sessions]));
      setCursor(page.next_cursor);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法继续加载");
    }
  }, [archived, client, cursor]);

  const createSession = useCallback(
    async (settings: Settings = {}) => {
      setBusy(true);
      try {
        const session = await client.createSession(compactSettings(settings));
        await refresh();
        return session;
      } finally {
        setBusy(false);
      }
    },
    [client, refresh],
  );

  return {
    sessions,
    error,
    busy,
    loaded,
    archived,
    setArchived,
    hasMore: Boolean(cursor),
    refresh,
    loadMore,
    createSession,
  };
}

function uniqueSessions(sessions: Session[]): Session[] {
  const seen = new Map<string, Session>();
  for (const session of sessions) {
    const prev = seen.get(session.id);
    if (!prev || (session.updated_at ?? 0) >= (prev.updated_at ?? 0)) {
      seen.set(session.id, session);
    }
  }
  return [...seen.values()];
}

function compactSettings(settings: Settings): Settings {
  const next: Settings = {};
  if (settings.model) {
    next.model = settings.model;
  }
  if (settings.effort) {
    next.effort = settings.effort;
  }
  if (settings.collaboration_mode) {
    next.collaboration_mode = settings.collaboration_mode;
  }
  if (settings.approval_policy) {
    next.approval_policy = settings.approval_policy;
  }
  if (settings.sandbox) {
    next.sandbox = settings.sandbox;
  }
  if (settings.cwd) {
    next.cwd = settings.cwd;
  }
  return next;
}

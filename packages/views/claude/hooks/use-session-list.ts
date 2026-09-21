"use client";

import type { ClaudeSession, ClaudeSettingsPatch } from "@codedock/core/claude";
import { useCallback, useEffect, useState } from "react";

import { useClaude } from "../provider.tsx";

// useClaudeSessionList 拉本机 Claude 对话列表；创建后可立刻套上目录覆盖。
export function useClaudeSessionList() {
  const { client } = useClaude();
  const [sessions, setSessions] = useState<ClaudeSession[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const next = await client.listSessions();
      setSessions(uniqueSessions(next).filter((session) => !session.archived));
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法加载 Claude 对话");
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const createSession = useCallback(
    async (settings: ClaudeSettingsPatch = {}) => {
      setBusy(true);
      try {
        const session = await client.createSession();
        const patch = compactSettings(settings);
        if (Object.keys(patch).length > 0) {
          await client.applySettings(session.id, patch);
        }
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
    refresh,
    createSession,
  };
}

// uniqueSessions 按 id 去重，后出现的覆盖先出现的。
function uniqueSessions(sessions: ClaudeSession[]): ClaudeSession[] {
  const seen = new Map<string, ClaudeSession>();
  for (const session of sessions) {
    seen.set(session.id, session);
  }
  return [...seen.values()];
}

// compactSettings 只留下真正改过的覆盖项。
function compactSettings(settings: ClaudeSettingsPatch): ClaudeSettingsPatch {
  const next: ClaudeSettingsPatch = {};
  if (settings.model) {
    next.model = settings.model;
  }
  if (settings.effort) {
    next.effort = settings.effort;
  }
  if (settings.permission_mode) {
    next.permission_mode = settings.permission_mode;
  }
  if (settings.cwd) {
    next.cwd = settings.cwd;
  }
  return next;
}

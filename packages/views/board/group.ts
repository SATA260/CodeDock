import type { Placement, Work } from "@codedock/core/board";

export type GroupedSession = {
  id: string;
  engine?: string;
  updated_at: string;
  summary?: string;
};

export type WorkGroup<T extends GroupedSession = GroupedSession> = {
  id: string | null;
  title: string;
  updatedAt: string;
  sessions: T[];
};

// sessionKey 用引擎+会话拼侧栏/看板主键。
export function sessionKey(engine: string | undefined, sessionId: string): string {
  return `${engine === "native" ? "agent" : engine ?? "agent"}:${sessionId}`;
}

// groupSessionsByWork 先按 Work 再按时间聚会话；未挂卡的进未归组。
export function groupSessionsByWork<T extends GroupedSession>(
  sessions: T[],
  placements: Placement[],
  works: Work[],
): WorkGroup<T>[] {
  const bySession = new Map<string, string>();
  for (const place of placements) {
    bySession.set(sessionKey(place.engine, place.session_id), place.work_id);
  }
  const buckets = new Map<string | null, T[]>();
  for (const work of works) {
    buckets.set(work.id, []);
  }
  buckets.set(null, []);
  for (const session of sessions) {
    const workId = bySession.get(sessionKey(session.engine, session.id)) ?? null;
    const list = buckets.get(workId) ?? buckets.get(null);
    if (!list) {
      continue;
    }
    if (!buckets.has(workId)) {
      buckets.set(null, list);
    }
    list.push(session);
  }
  const ranked = [...works].sort((left, right) => (left.updated_at < right.updated_at ? 1 : -1));
  const groups: WorkGroup<T>[] = ranked.map((work) => ({
    id: work.id,
    title: work.title || "未命名",
    updatedAt: work.updated_at,
    sessions: sortSessions(buckets.get(work.id) ?? []),
  }));
  const ungrouped = sortSessions(buckets.get(null) ?? []);
  groups.push({ id: null, title: "未分组", updatedAt: ungrouped[0]?.updated_at ?? "", sessions: ungrouped });
  return groups;
}

// sortSessions 组内按更新时间倒序。
function sortSessions<T extends GroupedSession>(sessions: T[]): T[] {
  return sessions.slice().sort((left, right) => (left.updated_at < right.updated_at ? 1 : -1));
}

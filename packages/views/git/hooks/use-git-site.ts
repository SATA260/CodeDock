"use client";

import type { BranchView, Commit, DiffFile, SiteState } from "@codedock/core/git";
import { useCallback, useEffect, useState } from "react";

import { localNameFromRemote } from "../lib/status.ts";
import { useGit } from "../provider.tsx";

const emptyState = (): SiteState => ({
  path: "",
  is_repo: false,
  empty: true,
  branch: "",
  head: "",
  detached: false,
  upstream: "",
  ahead: 0,
  behind: 0,
  upstream_gone: false,
  integrating: "",
  default_branch: "",
  files: [],
  remotes: [],
});

const emptyBranches = (): BranchView => ({
  current: "",
  locals: [],
  remotes: [],
  graph: { nodes: [], edges: [] },
});

const emptyDiffs = (): { staged: DiffFile[]; worktree: DiffFile[] } => ({
  staged: [],
  worktree: [],
});

export function useGitSite() {
  const { client } = useGit();
  const [state, setState] = useState<SiteState>(emptyState);
  const [branches, setBranches] = useState<BranchView>(emptyBranches);
  const [diffs, setDiffs] = useState(emptyDiffs);
  const [commits, setCommits] = useState<Commit[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const [nextState, nextBranches, staged, worktree, nextCommits] = await Promise.all([
        client.status(),
        client.listBranches(),
        client.diff("staged"),
        client.diff("worktree"),
        client.log(),
      ]);
      setState(nextState);
      setBranches({
        ...nextBranches,
        locals: nextBranches.locals ?? [],
        remotes: nextBranches.remotes ?? [],
        graph: nextBranches.graph ?? { nodes: [], edges: [] },
      });
      setDiffs({ staged, worktree });
      setCommits(nextCommits);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法读取仓库");
    } finally {
      setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const run = useCallback(
    async (action: () => Promise<unknown>) => {
      setBusy(true);
      try {
        await action();
        await refresh();
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "操作失败");
        throw err;
      } finally {
        setBusy(false);
      }
    },
    [refresh],
  );

  const stage = useCallback((paths: string[]) => run(() => client.stage(paths)), [client, run]);
  const unstage = useCallback((paths: string[]) => run(() => client.unstage(paths)), [client, run]);
  const discard = useCallback((paths: string[]) => run(() => client.discard(paths)), [client, run]);
  const commit = useCallback(
    (message: string) => run(() => client.commit({ message, paths: [] })),
    [client, run],
  );
  const push = useCallback(() => run(() => client.push()), [client, run]);
  const switchBranch = useCallback(
    (name: string) => run(() => client.switchBranch(name)),
    [client, run],
  );
  const reload = useCallback(async () => {
    setBusy(true);
    try {
      await refresh();
    } finally {
      setBusy(false);
    }
  }, [refresh]);

  const checkoutRemote = useCallback(
    (remoteName: string) =>
      run(async () => {
        const local = localNameFromRemote(remoteName);
        const listed = await client.listBranches();
        const exists = (listed.locals ?? []).some((branch) => branch.name === local);
        if (!exists) {
          await client.createBranch(local, remoteName);
        }
        await client.switchBranch(local);
      }),
    [client, run],
  );

  return {
    state,
    branches,
    diffs,
    commits,
    error,
    busy,
    loading,
    refresh,
    reload,
    stage,
    unstage,
    discard,
    commit,
    push,
    switchBranch,
    checkoutRemote,
  };
}

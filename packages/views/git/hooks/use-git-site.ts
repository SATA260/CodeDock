"use client";

import type { BranchView, Commit, DiffFile, MessageDraft, PromptConfig, SiteState } from "@codedock/core/git";
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
  const [prompt, setPrompt] = useState<PromptConfig | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const nextState = await client.status();
      setState(nextState);
      const extras = await Promise.allSettled([
        client.listBranches(),
        client.diff("staged"),
        client.diff("worktree"),
        client.log(),
      ]);
      const [nextBranches, staged, worktree, nextCommits] = extras;
      if (nextBranches.status === "fulfilled") {
        setBranches({
          ...nextBranches.value,
          locals: nextBranches.value.locals ?? [],
          remotes: nextBranches.value.remotes ?? [],
          graph: nextBranches.value.graph ?? { nodes: [], edges: [] },
        });
      }
      if (staged.status === "fulfilled" || worktree.status === "fulfilled") {
        setDiffs((prev) => ({
          staged: staged.status === "fulfilled" ? staged.value : prev.staged,
          worktree: worktree.status === "fulfilled" ? worktree.value : prev.worktree,
        }));
      }
      if (nextCommits.status === "fulfilled") {
        setCommits(nextCommits.value);
      }
      const failed = extras.find((item) => item.status === "rejected");
      if (failed && failed.status === "rejected") {
        setError(failed.reason instanceof Error ? failed.reason.message : "无法读取仓库");
        return;
      }
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

  useEffect(() => {
    let cancelled = false;
    void client
      .messagePrompt()
      .then((next) => {
        if (!cancelled) {
          setPrompt(next);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "无法读取提示词");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [client]);

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

  const generate = useCallback(async (): Promise<MessageDraft> => {
    setGenerating(true);
    try {
      const draft = await client.generateMessage();
      setError(null);
      return draft;
    } catch (err) {
      setError(err instanceof Error ? err.message : "生成失败");
      throw err;
    } finally {
      setGenerating(false);
    }
  }, [client]);

  const savePrompt = useCallback(
    async (selected: string, custom: string) => {
      try {
        const next = await client.saveMessagePrompt({ selected, custom });
        setPrompt(next);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "无法保存提示词");
        throw err;
      }
    },
    [client],
  );

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
    prompt,
    error,
    busy,
    generating,
    loading,
    refresh,
    reload,
    stage,
    unstage,
    discard,
    commit,
    generate,
    savePrompt,
    push,
    switchBranch,
    checkoutRemote,
  };
}

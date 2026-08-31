"use client";

import type { GitClient } from "@codedock/core/git";
import { createContext, useContext, type ReactNode } from "react";

type GitContextValue = {
  client: GitClient;
};

const GitContext = createContext<GitContextValue | null>(null);

export function GitProvider({ client, children }: { client: GitClient; children: ReactNode }) {
  return <GitContext.Provider value={{ client }}>{children}</GitContext.Provider>;
}

export function useGit(): GitContextValue {
  const ctx = useContext(GitContext);
  if (!ctx) {
    throw new Error("useGit must be used within GitProvider");
  }
  return ctx;
}

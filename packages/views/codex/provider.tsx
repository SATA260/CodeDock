"use client";

import type { CodexClient } from "@codedock/core/codex";
import { createContext, useContext, type ReactNode } from "react";

type CodexContextValue = {
  client: CodexClient;
};

const CodexContext = createContext<CodexContextValue | null>(null);

export function CodexProvider({ client, children }: { client: CodexClient; children: ReactNode }) {
  return <CodexContext.Provider value={{ client }}>{children}</CodexContext.Provider>;
}

export function useCodex(): CodexContextValue {
  const ctx = useContext(CodexContext);
  if (!ctx) {
    throw new Error("useCodex must be used within CodexProvider");
  }
  return ctx;
}

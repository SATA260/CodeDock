"use client";

import type { ClaudeClient } from "@codedock/core/claude";
import { createContext, useContext, type ReactNode } from "react";

type ClaudeContextValue = {
  client: ClaudeClient;
};

const ClaudeContext = createContext<ClaudeContextValue | null>(null);

// ClaudeProvider 只注入 ClaudeClient，不进 AgentContext。
export function ClaudeProvider({ client, children }: { client: ClaudeClient; children: ReactNode }) {
  return <ClaudeContext.Provider value={{ client }}>{children}</ClaudeContext.Provider>;
}

// useClaude 取出本页装配的 ClaudeClient。
export function useClaude(): ClaudeContextValue {
  const ctx = useContext(ClaudeContext);
  if (!ctx) {
    throw new Error("useClaude must be used within ClaudeProvider");
  }
  return ctx;
}

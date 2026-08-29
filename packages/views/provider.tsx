"use client";

import type { AgentClient } from "@codedock/core/chat";
import { createContext, useContext, type ReactNode } from "react";

type AgentContextValue = {
  client: AgentClient;
  userId: string;
};

const AgentContext = createContext<AgentContextValue | null>(null);

export function AgentProvider({
  client,
  userId,
  children,
}: {
  client: AgentClient;
  userId: string;
  children: ReactNode;
}) {
  return <AgentContext.Provider value={{ client, userId }}>{children}</AgentContext.Provider>;
}

export function useAgent(): AgentContextValue {
  const ctx = useContext(AgentContext);
  if (!ctx) {
    throw new Error("useAgent must be used within AgentProvider");
  }
  return ctx;
}

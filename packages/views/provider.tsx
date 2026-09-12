"use client";

import type { AgentClient } from "@codedock/core/chat";
import { createContext, useContext, type ReactNode } from "react";

export type DirectoryEntry = {
  name: string;
  path: string;
};

export type DirectoryListing = {
  path: string;
  parent?: string;
  entries: DirectoryEntry[];
};

type AgentContextValue = {
  client: AgentClient;
  userId: string;
  listDirectories?: (path?: string) => Promise<DirectoryListing>;
};

const AgentContext = createContext<AgentContextValue | null>(null);

export function AgentProvider({
  client,
  userId,
  listDirectories,
  children,
}: {
  client: AgentClient;
  userId: string;
  listDirectories?: (path?: string) => Promise<DirectoryListing>;
  children: ReactNode;
}) {
  return (
    <AgentContext.Provider value={{ client, userId, listDirectories }}>
      {children}
    </AgentContext.Provider>
  );
}

export function useAgent(): AgentContextValue {
  const ctx = useContext(AgentContext);
  if (!ctx) {
    throw new Error("useAgent must be used within AgentProvider");
  }
  return ctx;
}

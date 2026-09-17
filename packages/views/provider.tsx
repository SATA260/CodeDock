"use client";

import type { AgentClient } from "@codedock/core/chat";
import { createContext, useContext, type ReactNode } from "react";

export type PickedLocalFile = {
  path: string;
  name: string;
};

export type PickFilesOptions = {
  images?: boolean;
  multiple?: boolean;
  start?: string;
};

export type PickDirectoryOptions = {
  start?: string;
};

type AgentContextValue = {
  client: AgentClient;
  userId: string;
  pickDirectory?: (options?: PickDirectoryOptions) => Promise<string | undefined>;
  pickFiles?: (options?: PickFilesOptions) => Promise<PickedLocalFile[]>;
};

const AgentContext = createContext<AgentContextValue | null>(null);

export function AgentProvider({
  client,
  userId,
  pickDirectory,
  pickFiles,
  children,
}: {
  client: AgentClient;
  userId: string;
  pickDirectory?: (options?: PickDirectoryOptions) => Promise<string | undefined>;
  pickFiles?: (options?: PickFilesOptions) => Promise<PickedLocalFile[]>;
  children: ReactNode;
}) {
  return (
    <AgentContext.Provider value={{ client, userId, pickDirectory, pickFiles }}>
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

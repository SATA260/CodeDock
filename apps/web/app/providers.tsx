"use client";

import { AgentClient } from "@codedock/core/chat";
import { AgentProvider } from "@codedock/views";
import { useMemo, type ReactNode } from "react";

import { apiBase, defaultUserId } from "@/lib/env";

export function Providers({ children }: { children: ReactNode }) {
  const client = useMemo(() => new AgentClient({ baseUrl: apiBase }), []);
  return (
    <AgentProvider client={client} userId={defaultUserId}>
      {children}
    </AgentProvider>
  );
}

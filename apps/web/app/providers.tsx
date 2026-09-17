"use client";

import { AgentClient } from "@codedock/core/chat";
import { CodexClient } from "@codedock/core/codex";
import { CodexProvider } from "@codedock/views/codex";
import { AgentProvider } from "@codedock/views";
import { useMemo, type ReactNode } from "react";

import { apiBase, defaultUserId } from "@/lib/env";
import { pickDirectory } from "@/lib/pick-directory";
import { pickFiles } from "@/lib/pick-files";

export function Providers({ children }: { children: ReactNode }) {
  const client = useMemo(() => new AgentClient({ baseUrl: apiBase }), []);
  const codex = useMemo(() => new CodexClient({ baseUrl: apiBase }), []);
  return (
    <AgentProvider
      client={client}
      userId={defaultUserId}
      pickDirectory={pickDirectory}
      pickFiles={pickFiles}
    >
      <CodexProvider client={codex}>{children}</CodexProvider>
    </AgentProvider>
  );
}

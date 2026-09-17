"use client";

import { AgentClient } from "@codedock/core/chat";
import { ClaudeClient } from "@codedock/core/claude";
import { CodexClient } from "@codedock/core/codex";
import { ClaudeProvider } from "@codedock/views/claude";
import { CodexProvider } from "@codedock/views/codex";
import { AgentProvider } from "@codedock/views";
import { useMemo, type ReactNode } from "react";

import { apiBase, defaultUserId } from "@/lib/env";
import { pickDirectory } from "@/lib/pick-directory";
import { pickFiles } from "@/lib/pick-files";

// Providers 装配本机 Web 的 Agent / Codex / Claude 客户端，views 不读环境变量。
export function Providers({ children }: { children: ReactNode }) {
  const client = useMemo(() => new AgentClient({ baseUrl: apiBase }), []);
  const codex = useMemo(() => new CodexClient({ baseUrl: apiBase }), []);
  const claude = useMemo(() => new ClaudeClient({ baseUrl: apiBase, userId: defaultUserId }), []);
  return (
    <AgentProvider
      client={client}
      userId={defaultUserId}
      pickDirectory={pickDirectory}
      pickFiles={pickFiles}
    >
      <CodexProvider client={codex}>
        <ClaudeProvider client={claude}>{children}</ClaudeProvider>
      </CodexProvider>
    </AgentProvider>
  );
}

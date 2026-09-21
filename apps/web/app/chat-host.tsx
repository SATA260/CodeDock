"use client";

import { GitClient } from "@codedock/core/git";
import { ChatPage, type SessionEngine } from "@codedock/views/chat";
import { GitProvider } from "@codedock/views/git";
import { usePathname, useRouter } from "next/navigation";
import { useMemo } from "react";

import { apiBase } from "@/lib/env";
import { rememberSession } from "@/lib/session";

// ChatHost 解析对话路径并把导航收成回调；views 不知道具体 URL。
export function ChatHost() {
  const pathname = usePathname();
  const router = useRouter();
  const parsed = parseChatPath(pathname);
  if (parsed.sessionId && parsed.engine === "agent") {
    rememberSession(parsed.sessionId);
  }
  const gitSessionId = parsed.engine === "agent" ? parsed.sessionId : undefined;
  const git = useMemo(
    () => new GitClient({ baseUrl: apiBase, sessionId: gitSessionId }),
    [gitSessionId],
  );

  return (
    <GitProvider client={git} sessionId={gitSessionId}>
      <ChatPage
        sessionId={parsed.sessionId}
        engine={parsed.engine}
        brandSrc="/brand/codedock-berth-mark.svg"
        codexIconSrc="/brand/codex-app-icon.png"
        claudeIconSrc="/brand/claude-app-icon.svg"
        onOpenSession={(id, engine = parsed.engine ?? "agent") => {
          const path = pathFor(id, engine);
          if (pathname !== path) {
            router.push(path);
          }
        }}
        onNewConversation={() => {
          if (pathname !== "/") {
            router.push("/");
          }
        }}
      />
    </GitProvider>
  );
}

// parseChatPath 先认 /s/claude/:id，再认 /s/c/:id，避免吃掉 Claude 前缀。
function parseChatPath(pathname: string): { sessionId?: string; engine?: SessionEngine } {
  const claude = pathname.match(/^\/s\/claude\/([^/]+)/);
  if (claude?.[1]) {
    return { sessionId: decodeURIComponent(claude[1]), engine: "claude" };
  }
  const codex = pathname.match(/^\/s\/c\/([^/]+)/);
  if (codex?.[1]) {
    return { sessionId: decodeURIComponent(codex[1]), engine: "codex" };
  }
  const agent = pathname.match(/^\/s\/([^/]+)/);
  if (agent?.[1] && agent[1] !== "c" && agent[1] !== "claude") {
    return { sessionId: decodeURIComponent(agent[1]), engine: "agent" };
  }
  return {};
}

// pathFor 由 web 决定三套会话路径。
function pathFor(id: string, engine: SessionEngine): string {
  if (engine === "claude") {
    return `/s/claude/${id}`;
  }
  if (engine === "codex") {
    return `/s/c/${id}`;
  }
  return `/s/${id}`;
}

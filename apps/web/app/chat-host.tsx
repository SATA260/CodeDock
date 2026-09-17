"use client";

import { ChatPage, type SessionEngine } from "@codedock/views/chat";
import { usePathname, useRouter } from "next/navigation";

import { rememberSession } from "@/lib/session";

export function ChatHost() {
  const pathname = usePathname();
  const router = useRouter();
  const parsed = parseChatPath(pathname);
  if (parsed.sessionId && parsed.engine === "agent") {
    rememberSession(parsed.sessionId);
  }

  return (
    <ChatPage
      sessionId={parsed.sessionId}
      engine={parsed.engine}
      brandSrc="/brand/codedock-berth-mark.svg"
      onOpenSession={(id, engine = parsed.engine ?? "agent") => {
        const path = engine === "codex" ? `/s/c/${id}` : `/s/${id}`;
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
  );
}

function parseChatPath(pathname: string): { sessionId?: string; engine?: SessionEngine } {
  const codex = pathname.match(/^\/s\/c\/([^/]+)/);
  if (codex?.[1]) {
    return { sessionId: decodeURIComponent(codex[1]), engine: "codex" };
  }
  const agent = pathname.match(/^\/s\/([^/]+)/);
  if (agent?.[1] && agent[1] !== "c") {
    return { sessionId: decodeURIComponent(agent[1]), engine: "agent" };
  }
  return {};
}

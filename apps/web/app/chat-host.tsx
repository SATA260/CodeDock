"use client";

import { BoardClient } from "@codedock/core/board";
import { GitClient } from "@codedock/core/git";
import { BoardProvider } from "@codedock/views/board";
import { ChatPage, type SessionEngine } from "@codedock/views/chat";
import { GitProvider } from "@codedock/views/git";
import { usePathname, useRouter } from "next/navigation";
import { useMemo, useRef, useState } from "react";

import { apiBase, defaultUserId } from "@/lib/env";
import { rememberSession } from "@/lib/session";

// ChatHost 解析对话 / 看板路径并把导航收成回调；views 不知道具体 URL。
export function ChatHost() {
  const pathname = usePathname();
  const router = useRouter();
  const parsed = parseChatPath(pathname);
  const lastSession = useRef<{ id: string; engine: SessionEngine } | null>(null);
  if (parsed.sessionId && parsed.engine) {
    lastSession.current = { id: parsed.sessionId, engine: parsed.engine };
  }
  if (parsed.sessionId && parsed.engine === "agent") {
    rememberSession(parsed.sessionId);
  }
  const [boardGitSessionId, setBoardGitSessionId] = useState<string | undefined>();
  const routeGitSessionId = parsed.engine === "agent" ? parsed.sessionId : undefined;
  const gitSessionId = parsed.board ? boardGitSessionId : routeGitSessionId;
  const git = useMemo(
    () => new GitClient({ baseUrl: apiBase, sessionId: gitSessionId }),
    [gitSessionId],
  );
  const board = useMemo(
    () => new BoardClient({ baseUrl: apiBase, userId: defaultUserId }),
    [],
  );

  return (
    <GitProvider client={git} sessionId={gitSessionId}>
      <BoardProvider client={board}>
        <ChatPage
          sessionId={parsed.sessionId}
          engine={parsed.engine}
          boardMode={parsed.board}
          brandSrc="/brand/codedock-berth-mark.svg"
          codexIconSrc="/brand/codex-app-icon.png"
          claudeIconSrc="/brand/claude-app-icon.svg"
          onGitSession={setBoardGitSessionId}
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
          onOpenBoard={() => {
            if (pathname !== "/board") {
              router.push("/board");
            }
          }}
          onLeaveBoard={() => {
            const last = lastSession.current;
            const path = last ? pathFor(last.id, last.engine) : "/";
            if (pathname !== path) {
              router.push(path);
            }
          }}
        />
      </BoardProvider>
    </GitProvider>
  );
}

// parseChatPath 先认 /board，再认 /s/claude/:id 与 /s/c/:id。
function parseChatPath(pathname: string): { sessionId?: string; engine?: SessionEngine; board?: boolean } {
  if (pathname === "/board" || pathname.startsWith("/board/")) {
    return { board: true };
  }
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

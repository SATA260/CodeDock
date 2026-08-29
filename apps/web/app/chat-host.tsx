"use client";

import { ChatPage } from "@codedock/views/chat";
import { usePathname, useRouter } from "next/navigation";

export function ChatHost() {
  const pathname = usePathname();
  const router = useRouter();
  const match = pathname.match(/^\/s\/([^/]+)/);
  const sessionId = match?.[1] ? decodeURIComponent(match[1]) : undefined;

  return (
    <ChatPage
      sessionId={sessionId}
      brandSrc="/brand/codedock-berth-mark.svg"
      onOpenSession={(id) => {
        if (id !== sessionId) {
          router.push(`/s/${id}`);
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

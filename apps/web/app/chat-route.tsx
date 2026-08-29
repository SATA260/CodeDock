"use client";

import { ChatPage } from "@codedock/views/chat";
import { useRouter } from "next/navigation";

export function ChatRoute({ sessionId }: { sessionId?: string }) {
  const router = useRouter();
  return (
    <ChatPage
      sessionId={sessionId}
      onOpenSession={(id) => router.push(`/s/${id}`)}
      onNewConversation={() => router.push("/")}
    />
  );
}

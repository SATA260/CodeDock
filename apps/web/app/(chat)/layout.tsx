import type { ReactNode } from "react";

import { ChatHost } from "../chat-host";

export default function ChatLayout({ children }: { children: ReactNode }) {
  return (
    <div className="h-full">
      <ChatHost />
      {children}
    </div>
  );
}

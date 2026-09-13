"use client";

import { GitClient } from "@codedock/core/git";
import { GitPage, GitProvider } from "@codedock/views/git";
import { useMemo, useSyncExternalStore } from "react";

import { apiBase } from "@/lib/env";
import { readCurrentSession } from "@/lib/session";

function subscribeSession(onStoreChange: () => void) {
  if (typeof window === "undefined") {
    return () => {};
  }
  window.addEventListener("storage", onStoreChange);
  return () => window.removeEventListener("storage", onStoreChange);
}

export function GitHost() {
  const sessionId = useSyncExternalStore(subscribeSession, readCurrentSession, () => undefined);
  const client = useMemo(
    () => new GitClient({ baseUrl: apiBase, sessionId }),
    [sessionId],
  );

  return (
    <GitProvider client={client} sessionId={sessionId}>
      <div className="h-full">
        <GitPage />
      </div>
    </GitProvider>
  );
}

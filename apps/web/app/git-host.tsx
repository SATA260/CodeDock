"use client";

import { GitClient } from "@codedock/core/git";
import { GitPage, GitProvider } from "@codedock/views/git";
import { useEffect, useMemo, useState } from "react";

import { apiBase } from "@/lib/env";
import { readCurrentSession } from "@/lib/session";

export function GitHost() {
  const [sessionId, setSessionId] = useState<string | undefined>(undefined);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    setSessionId(readCurrentSession());
    setReady(true);
  }, []);

  const client = useMemo(
    () => new GitClient({ baseUrl: apiBase, sessionId }),
    [sessionId],
  );

  if (!ready) {
    return <p className="px-3 py-2 text-xs text-muted-foreground">正在读取仓库…</p>;
  }

  return (
    <GitProvider client={client} sessionId={sessionId}>
      <div className="h-full">
        <GitPage />
      </div>
    </GitProvider>
  );
}

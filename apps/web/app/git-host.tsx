"use client";

import { GitClient } from "@codedock/core/git";
import { GitPage, GitProvider } from "@codedock/views/git";
import { useMemo } from "react";

import { apiBase } from "@/lib/env";

export function GitHost() {
  const client = useMemo(() => new GitClient({ baseUrl: apiBase }), []);

  return (
    <GitProvider client={client}>
      <div className="h-full">
        <GitPage />
      </div>
    </GitProvider>
  );
}

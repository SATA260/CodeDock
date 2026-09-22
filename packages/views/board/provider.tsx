"use client";

import type { BoardClient } from "@codedock/core/board";
import { createContext, useContext, type ReactNode } from "react";

type BoardContextValue = {
  client: BoardClient;
};

const BoardContext = createContext<BoardContextValue | null>(null);

// BoardProvider 只注入 BoardClient，不扩 AgentContext。
export function BoardProvider({ client, children }: { client: BoardClient; children: ReactNode }) {
  return <BoardContext.Provider value={{ client }}>{children}</BoardContext.Provider>;
}

// useBoard 读取看板客户端。
export function useBoard(): BoardContextValue {
  const ctx = useContext(BoardContext);
  if (!ctx) {
    throw new Error("useBoard must be used within BoardProvider");
  }
  return ctx;
}

"use client";

import type { ClaudeCommand, ClaudeMode, ClaudeModel, ClaudeStatus } from "@codedock/core/claude";
import { useCallback, useEffect, useState } from "react";

import { useClaude } from "../provider.tsx";

// useClaudeCatalog 读本机模型、权限档和斜杠命令；窗口重新可见时再拉一次。
export function useClaudeCatalog() {
  const { client } = useClaude();
  const [status, setStatus] = useState<ClaudeStatus | null>(null);
  const [models, setModels] = useState<ClaudeModel[]>([]);
  const [modes, setModes] = useState<ClaudeMode[]>([]);
  const [commands, setCommands] = useState<ClaudeCommand[]>([]);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [nextStatus, nextModels, nextModes, nextCommands] = await Promise.all([
        client.probe(),
        client.listModels().catch(() => [] as ClaudeModel[]),
        client.listModes().catch(() => [] as ClaudeMode[]),
        client.listCommands(),
      ]);
      setStatus(nextStatus);
      setModels(nextModels);
      setModes(nextModes);
      setCommands(nextCommands);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法读取 Claude 状态");
    }
  }, [client]);

  useEffect(() => {
    void refresh();
    const onVisible = () => {
      if (document.visibilityState === "visible") {
        void refresh();
      }
    };
    window.addEventListener("focus", onVisible);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      window.removeEventListener("focus", onVisible);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refresh]);

  return { status, models, modes, commands, error, refresh };
}

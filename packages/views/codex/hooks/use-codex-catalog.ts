"use client";

import type { CommandSpec, EngineStatus, ModeInfo, ModelInfo } from "@codedock/core/codex";
import { useCallback, useEffect, useState } from "react";

import { useCodex } from "../provider.tsx";

export function useCodexCatalog() {
  const { client } = useCodex();
  const [status, setStatus] = useState<EngineStatus | null>(null);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [modes, setModes] = useState<ModeInfo[]>([]);
  const [commands, setCommands] = useState<CommandSpec[]>([]);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [nextStatus, nextModels, nextModes, nextCommands] = await Promise.all([
        client.status(),
        client.listModels().catch(() => [] as ModelInfo[]),
        client.listModes().catch(() => [] as ModeInfo[]),
        client.listCommands(),
      ]);
      setStatus(nextStatus);
      setModels(nextModels);
      setModes(nextModes);
      setCommands(nextCommands);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法读取 Codex 状态");
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

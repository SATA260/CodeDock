"use client";

import type { AgentMode } from "@codedock/core/chat";
import {
  Button,
  PromptInput,
  PromptInputFooter,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputTools,
} from "@codedock/ui";
import { useState } from "react";

const modes: { value: AgentMode; label: string }[] = [
  { value: "ask_for_approval", label: "需审批" },
  { value: "auto_approve", label: "自动过" },
  { value: "yolo", label: "YOLO" },
];

export function PromptBar({
  running,
  sending,
  onSend,
  onCancel,
}: {
  running: boolean;
  sending: boolean;
  onSend: (text: string, mode: AgentMode) => Promise<void>;
  onCancel: () => Promise<void>;
}) {
  const [text, setText] = useState("");
  const [mode, setMode] = useState<AgentMode>("ask_for_approval");

  return (
    <div className="mx-auto w-full max-w-3xl px-4 pb-4">
      <PromptInput
        onSend={async (message) => {
          const next = message.text.trim();
          if (!next) {
            return;
          }
          setText("");
          await onSend(next, mode);
        }}
      >
        <PromptInputTextarea
          value={text}
          disabled={sending}
          placeholder="给 Agent 发消息…"
          onChange={(event) => setText(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              event.currentTarget.form?.requestSubmit();
            }
          }}
        />
        <PromptInputFooter>
          <PromptInputTools>
            <select
              className="h-8 rounded-md border border-input bg-background px-2 text-xs text-muted-foreground"
              value={mode}
              onChange={(event) => setMode(event.target.value as AgentMode)}
            >
              {modes.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
            {running ? (
              <Button size="sm" variant="outline" onClick={() => void onCancel()}>
                取消
              </Button>
            ) : null}
          </PromptInputTools>
          <PromptInputSubmit
            status={sending ? "streaming" : "ready"}
            disabled={sending || !text.trim()}
          />
        </PromptInputFooter>
      </PromptInput>
    </div>
  );
}

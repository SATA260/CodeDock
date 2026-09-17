"use client";

import type { ApprovalAsk, AskAnswer, DecisionScope } from "@codedock/core/codex";
import {
  Button,
  Confirmation,
  ConfirmationAction,
  ConfirmationActions,
  ConfirmationRequest,
  ConfirmationTitle,
} from "@codedock/ui";
import { useState } from "react";

export function CodexAskDock({
  asks,
  onDecide,
  onExpire,
}: {
  asks: ApprovalAsk[];
  onDecide: (requestId: string, answer: AskAnswer) => Promise<void>;
  onExpire: (requestId: string) => Promise<void>;
}) {
  if (asks.length === 0) {
    return null;
  }
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-2 px-4 pb-2">
      {asks.map((ask) => (
        <AskCard key={ask.id} ask={ask} onDecide={onDecide} onExpire={onExpire} />
      ))}
    </div>
  );
}

function AskCard({
  ask,
  onDecide,
  onExpire,
}: {
  ask: ApprovalAsk;
  onDecide: (requestId: string, answer: AskAnswer) => Promise<void>;
  onExpire: (requestId: string) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [scope, setScope] = useState<DecisionScope>("once");
  const [choice, setChoice] = useState(ask.options?.[0] ?? "");
  const [values, setValues] = useState<string[]>(ask.fields?.map(() => "") ?? []);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    try {
      await fn();
    } finally {
      setBusy(false);
    }
  };

  return (
    <Confirmation>
      <ConfirmationTitle>{titleFor(ask)}</ConfirmationTitle>
      <ConfirmationRequest>
        {ask.command ? <pre className="font-mono text-xs">{ask.command}</pre> : null}
        {ask.prompt ? <p>{ask.prompt}</p> : null}
        {ask.paths?.length ? <p className="font-mono text-xs">{ask.paths.join("\n")}</p> : null}
        {ask.diff ? <pre className="max-h-32 overflow-auto font-mono text-[11px]">{ask.diff}</pre> : null}
        {ask.kind === "question" && ask.options?.length ? (
          <select
            className="mt-2 h-8 w-full rounded-md border border-border bg-background px-2 text-sm"
            value={choice}
            onChange={(event) => setChoice(event.target.value)}
          >
            {ask.options.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        ) : null}
        {ask.kind === "question" && !ask.options?.length ? (
          <input
            className="mt-2 h-8 w-full rounded-md border border-border bg-background px-2 text-sm"
            value={choice}
            placeholder="补一句"
            onChange={(event) => setChoice(event.target.value)}
          />
        ) : null}
        {ask.kind === "form"
          ? (ask.fields ?? []).map((field, index) => (
              <label key={field} className="mt-2 block text-xs">
                {field}
                <input
                  className="mt-1 h-8 w-full rounded-md border border-border bg-background px-2 text-sm"
                  value={values[index] ?? ""}
                  onChange={(event) => {
                    const next = values.slice();
                    next[index] = event.target.value;
                    setValues(next);
                  }}
                />
              </label>
            ))
          : null}
      </ConfirmationRequest>
      {ask.kind === "command" || ask.kind === "file_change" || ask.kind === "permissions" ? (
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={scope === "session"}
            onChange={(event) => setScope(event.target.checked ? "session" : "once")}
          />
          本会话不再问
        </label>
      ) : null}
      <ConfirmationActions>
        <ConfirmationAction
          variant="outline"
          disabled={busy}
          onClick={() => void run(() => onExpire(ask.id || ask.external_request_id))}
        >
          过期
        </ConfirmationAction>
        <ConfirmationAction
          variant="outline"
          disabled={busy}
          onClick={() => void run(() => onDecide(ask.id || ask.external_request_id, { approved: false, scope }))}
        >
          拒绝
        </ConfirmationAction>
        <ConfirmationAction
          disabled={busy}
          onClick={() =>
            void run(() =>
              onDecide(ask.id || ask.external_request_id, {
                approved: true,
                scope,
                choice: choice || undefined,
                values: ask.kind === "form" ? values : choice ? [choice] : undefined,
              }),
            )
          }
        >
          允许
        </ConfirmationAction>
      </ConfirmationActions>
    </Confirmation>
  );
}

function titleFor(ask: ApprovalAsk): string {
  switch (ask.kind) {
    case "command":
      return "能不能跑这条命令";
    case "file_change":
      return "能不能改这些文件";
    case "permissions":
      return "Codex 请求额外权限";
    case "question":
      return "需要你补一句";
    case "form":
      return "MCP 表单";
    default:
      return "需要你作答";
  }
}

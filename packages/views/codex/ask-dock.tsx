"use client";

import type { ApprovalAsk, AskAnswer, AskOption, AskQuestion, DecisionScope } from "@codedock/core/codex";
import {
  Button,
  Confirmation,
  ConfirmationAction,
  ConfirmationActions,
  ConfirmationRequest,
  ConfirmationTitle,
  cn,
} from "@codedock/ui";
import { useMemo, useState } from "react";

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
  const questions = useMemo(() => questionsOf(ask), [ask]);
  const [busy, setBusy] = useState(false);
  const [scope, setScope] = useState<DecisionScope>("once");
  const [choice, setChoice] = useState(ask.options?.[0] ?? "");
  const [values, setValues] = useState<string[]>(ask.fields?.map(() => "") ?? []);
  const [answers, setAnswers] = useState<Record<string, string>>(() => emptyAnswers(questions));

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    try {
      await fn();
    } finally {
      setBusy(false);
    }
  };

  const requestId = ask.id || ask.external_request_id;
  const canAllow =
    ask.kind !== "question" ||
    questions.every((question) => answers[questionKey(question, questions.indexOf(question))]?.trim());

  return (
    <Confirmation>
      <ConfirmationTitle>{titleFor(ask, questions)}</ConfirmationTitle>
      <ConfirmationRequest>
        {ask.command ? <pre className="font-mono text-xs">{ask.command}</pre> : null}
        {ask.kind !== "question" && ask.prompt ? <p>{ask.prompt}</p> : null}
        {ask.paths?.length ? <p className="font-mono text-xs">{ask.paths.join("\n")}</p> : null}
        {ask.diff ? <pre className="max-h-32 overflow-auto font-mono text-[11px]">{ask.diff}</pre> : null}
        {ask.kind === "question"
          ? questions.map((question, index) => (
              <QuestionBlock
                key={questionKey(question, index)}
                question={question}
                index={index}
                hideHeading={index === 0 && Boolean(question.header)}
                value={answers[questionKey(question, index)] ?? ""}
                disabled={busy}
                onChange={(next) => {
                  const key = questionKey(question, index);
                  setAnswers((current) => ({ ...current, [key]: next }));
                }}
              />
            ))
          : null}
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
          onClick={() => void run(() => onExpire(requestId))}
        >
          过期
        </ConfirmationAction>
        <ConfirmationAction
          variant="outline"
          disabled={busy}
          onClick={() => void run(() => onDecide(requestId, { approved: false, scope }))}
        >
          拒绝
        </ConfirmationAction>
        <ConfirmationAction
          disabled={busy || !canAllow}
          onClick={() =>
            void run(() =>
              onDecide(requestId, {
                approved: true,
                scope,
                choice: firstAnswer(answers, questions) || choice || undefined,
                values: ask.kind === "form" ? values : Object.values(answers).filter(Boolean),
                answers: ask.kind === "question" ? keyedAnswers(questions, answers) : undefined,
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

function QuestionBlock({
  question,
  index,
  hideHeading = false,
  value,
  disabled,
  onChange,
}: {
  question: AskQuestion;
  index: number;
  hideHeading?: boolean;
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const heading = hideHeading ? "" : question.header || question.prompt || (index === 0 ? "" : `问题 ${index + 1}`);
  const options = question.options ?? [];
  const presets = options.filter((option) => !option.other).map(optionValue);
  return (
    <div className={index === 0 ? "mt-1 space-y-2" : "mt-3 space-y-2"}>
      {heading ? <p className="text-sm text-foreground/90">{heading}</p> : null}
      {question.header && question.prompt && question.prompt !== question.header ? (
        <p className="text-xs text-muted-foreground">{question.prompt}</p>
      ) : null}
      {options.length > 0 ? (
        <div className="flex flex-col gap-1.5">
          {options.map((option) => {
            const selected = option.other
              ? Boolean(value.trim()) && !presets.includes(value)
              : value === optionValue(option);
            return (
              <button
                key={option.id || option.label}
                type="button"
                disabled={disabled}
                className={cn(
                  "w-full rounded-md border px-2.5 py-1.5 text-left text-sm transition-colors",
                  selected
                    ? "border-amber-400/70 bg-amber-950/40 text-amber-50"
                    : "border-border bg-background/60 text-foreground hover:border-amber-700/50 hover:bg-amber-950/20",
                )}
                onClick={() => onChange(option.other ? "" : optionValue(option))}
              >
                <span>{option.label}</span>
                {option.recommended ? (
                  <span className="ml-2 text-[10px] text-amber-200/80">推荐</span>
                ) : null}
                {option.other ? (
                  <span className="ml-2 text-[10px] text-muted-foreground">可改写</span>
                ) : null}
              </button>
            );
          })}
        </div>
      ) : null}
      <textarea
        className="min-h-16 w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
        value={value}
        disabled={disabled}
        placeholder={options.length > 0 ? "点选项填入，也可直接改写或自拟" : "补一句"}
        onChange={(event) => onChange(event.currentTarget.value)}
      />
    </div>
  );
}

function questionsOf(ask: ApprovalAsk): AskQuestion[] {
  if (ask.questions?.length) {
    return ask.questions;
  }
  if (ask.kind !== "question") {
    return [];
  }
  return [
    {
      id: ask.fields?.[0],
      header: ask.prompt,
      options: (ask.options ?? []).map((label) => ({
        id: label,
        label,
        other: isOtherLabel(label),
      })),
    },
  ];
}

function questionKey(question: AskQuestion, index: number): string {
  return question.id || `q${index}`;
}

function emptyAnswers(questions: AskQuestion[]): Record<string, string> {
  const next: Record<string, string> = {};
  for (const [index, question] of questions.entries()) {
    next[questionKey(question, index)] = "";
  }
  return next;
}

function keyedAnswers(questions: AskQuestion[], answers: Record<string, string>): Record<string, string> {
  const next: Record<string, string> = {};
  for (const [index, question] of questions.entries()) {
    const text = answers[questionKey(question, index)]?.trim();
    if (text) {
      next[question.id || questionKey(question, index)] = text;
    }
  }
  return next;
}

function firstAnswer(answers: Record<string, string>, questions: AskQuestion[]): string {
  for (const [index, question] of questions.entries()) {
    const text = answers[questionKey(question, index)]?.trim();
    if (text) {
      return text;
    }
  }
  return "";
}

function optionValue(option: AskOption): string {
  return option.id || option.label;
}

function isOtherLabel(label: string): boolean {
  return ["别的意思", "其他", "其它", "Other", "other"].includes(label.trim());
}

function titleFor(ask: ApprovalAsk, questions: AskQuestion[]): string {
  if (ask.kind === "question") {
    return questions[0]?.header || ask.prompt || "需要你作答";
  }
  switch (ask.kind) {
    case "command":
      return "能不能跑这条命令";
    case "file_change":
      return "能不能改这些文件";
    case "permissions":
      return "Codex 请求额外权限";
    case "form":
      return "MCP 表单";
    default:
      return "需要你作答";
  }
}

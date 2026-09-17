import type {
  ApprovalAsk,
  CodexEvent,
  CodexViewState,
  Progress,
  Session,
  Settings,
  TimelineItem,
  Turn,
} from "./types.ts";

export function emptyCodexState(): CodexViewState {
  return {
    session: null,
    settings: {},
    items: [],
    asks: [],
    lastSeq: 0,
    activeTurn: null,
    reset: false,
    usage: undefined,
  };
}

export function hydrateCodex(detail: {
  session: Session;
  progress: Progress[];
  asks: ApprovalAsk[];
  settings?: Settings;
  usage?: CodexViewState["usage"];
}): CodexViewState {
  return {
    session: detail.session,
    settings: detail.settings ?? {},
    items: detail.progress.map((item, index) => progressItem(item, index)),
    asks: detail.asks.slice(),
    lastSeq: 0,
    activeTurn: detail.session.active_turn_id
      ? {
          id: detail.session.active_turn_id,
          session_id: detail.session.id,
          status: "running",
        }
      : null,
    reset: false,
    usage: detail.usage,
  };
}

export function applyCodexEvent(state: CodexViewState, event: CodexEvent): CodexViewState {
  const next: CodexViewState = {
    ...state,
    items: state.items.slice(),
    asks: state.asks.slice(),
    lastSeq: Math.max(state.lastSeq, event.seq),
    reset: false,
  };
  switch (event.type) {
    case "reset":
      if (event.notice) {
        next.items.push(noticeItem(event.notice, event.seq));
      }
      return { ...next, reset: true };
    case "turn.queued":
    case "turn.started":
    case "turn.completed":
    case "turn.failed":
    case "turn.cancelled":
      if (event.turn) {
        next.activeTurn = isOpen(event.turn) ? event.turn : null;
        next.items.push(turnItem(event.turn));
      }
      if (event.progress) {
        if (event.progress.kind === "user") {
          next.items = next.items.filter((item) => item.id !== "local-user");
        }
        next.items = upsertProgress(next.items, event.progress);
      }
      if (event.notice) {
        next.items.push(noticeItem(event.notice, event.seq));
      }
      return next;
    case "progress":
      if (event.progress) {
        if (event.progress.kind === "user") {
          next.items = next.items.filter((item) => item.id !== "local-user");
        }
        next.items = upsertProgress(next.items, event.progress);
      }
      return next;
    case "ask.required":
      if (event.ask) {
        next.asks = next.asks.filter((ask) => ask.id !== event.ask?.id);
        next.asks.push(event.ask);
        if (next.activeTurn) {
          next.activeTurn = { ...next.activeTurn, status: "waiting_approval" };
        }
      }
      return next;
    case "ask.resolved":
      if (event.ask) {
        next.asks = next.asks.filter((ask) => ask.id !== event.ask?.id);
      }
      return next;
    case "notice":
      if (event.notice) {
        next.items.push(noticeItem(event.notice, event.seq));
      }
      return next;
    case "token.usage":
      if (event.usage) {
        next.usage = event.usage;
      }
      return next;
    default:
      return next;
  }
}

export function applyOptimisticUser(state: CodexViewState, text: string): CodexViewState {
  return {
    ...state,
    items: [...state.items, { id: "local-user", kind: "user", text, streaming: false }],
  };
}

export function dropOptimisticUser(state: CodexViewState): CodexViewState {
  return {
    ...state,
    items: state.items.filter((item) => item.id !== "local-user"),
  };
}

function isOpen(turn: Turn): boolean {
  return turn.status === "queued" || turn.status === "running" || turn.status === "waiting_approval";
}

function progressItem(progress: Progress, index: number): TimelineItem {
  return {
    id: progress.item_id || `progress-${index}`,
    kind: progress.kind,
    text: progress.text,
    command: progress.command,
    paths: progress.paths,
    diff: progress.diff,
    status: progress.status,
  };
}

function turnItem(turn: Turn): TimelineItem {
  return {
    id: `turn-${turn.id}-${turn.status}`,
    kind: "turn",
    status: turn.status,
    error: turn.error,
    text:
      turn.status === "queued"
        ? "已排队，等当前回合结束"
        : turn.status === "completed"
          ? "本轮完成"
          : turn.status === "cancelled"
            ? "已打断"
            : turn.status === "failed"
              ? turn.error || "本轮失败"
              : turn.status === "waiting_approval"
                ? "等待你的作答"
                : "Codex 正在工作",
  };
}

function noticeItem(text: string, seq: number): TimelineItem {
  return { id: `notice-${seq}`, kind: "notice", text };
}

function upsertProgress(items: TimelineItem[], progress: Progress): TimelineItem[] {
  if (progress.kind === "user") {
    const text = progress.text ?? "";
    if (items.some((item) => item.kind === "user" && item.text === text)) {
      return items.filter((item) => item.id !== "local-user");
    }
    return [...items.filter((item) => item.id !== "local-user"), progressItem(progress, items.length)];
  }
  const id = progress.item_id;
  if (!id) {
    return [...items, progressItem(progress, items.length)];
  }
  const index = items.findIndex((item) => item.id === id && item.kind !== "turn");
  if (index < 0) {
    return [...items, progressItem(progress, items.length)];
  }
  const current = items[index];
  const next = items.slice();
  next[index] = {
    ...current,
    kind: progress.kind || current.kind,
    text: append(current.text, progress.text),
    command: progress.command || current.command,
    paths: progress.paths?.length ? progress.paths : current.paths,
    diff: append(current.diff, progress.diff),
    status: progress.status || current.status,
    streaming: progress.kind === "text" || progress.kind === "reasoning",
  };
  return next;
}

function append(left: string | undefined, right: string | undefined): string | undefined {
  if (!right) {
    return left;
  }
  if (!left) {
    return right;
  }
  return left + right;
}

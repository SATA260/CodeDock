import { decodeText, parseDelta } from "./content.ts";
import {
  isTerminalRun,
  isThinkingPhase,
  type AgentEvent,
  type ApprovalDecidedPayload,
  type ApprovalRequiredPayload,
  type ApprovalToolCall,
  type AssistantCompletedPayload,
  type AssistantDeltaPayload,
  type AssistantStartedPayload,
  type ContextCompactedPayload,
  type Message,
  type RunCreatedPayload,
  type RunStateChangedPayload,
  type RunStatus,
  type RunTerminalPayload,
  type SessionState,
  type ThinkingPhase,
  type TimelineItem,
  type ToolCall,
  type ToolCallPayload,
} from "./types.ts";

export function emptyState(messages: Record<string, Message> = {}): SessionState {
  return {
    lastSeq: 0,
    activeRunId: null,
    runStatus: null,
    items: [],
    messages,
  };
}

export function indexMessages(messages: Message[]): Record<string, Message> {
  const byId: Record<string, Message> = {};
  for (const message of messages) {
    byId[message.id] = message;
  }
  return byId;
}

export function hydrate(messages: Message[], events: AgentEvent[]): SessionState {
  let state = emptyState(indexMessages(messages));
  for (const event of events) {
    state = applyEvent(state, event);
  }
  return state;
}

export function applyOptimisticUser(
  state: SessionState,
  input: { runId: string; text: string },
): SessionState {
  return upsertUser(state, {
    messageId: `pending:${input.runId}`,
    runId: input.runId,
    text: input.text,
    seq: state.lastSeq,
  });
}

export function dropOptimisticUser(state: SessionState, runId: string): SessionState {
  return removeItem(state, userId(`pending:${runId}`));
}

export function applyEvent(state: SessionState, event: AgentEvent): SessionState {
  if (event.seq <= state.lastSeq) {
    return state;
  }
  let next: SessionState = {
    ...state,
    lastSeq: event.seq,
    items: state.items.slice(),
    messages: state.messages,
    activeRunId: event.run_id || state.activeRunId,
  };

  switch (event.type) {
    case "run.created":
      next = applyRunCreated(next, event);
      break;
    case "run.state_changed":
      next = applyRunStateChanged(next, event);
      break;
    case "assistant.started":
      next = applyAssistantStarted(next, event);
      break;
    case "assistant.delta":
      next = applyAssistantDelta(next, event);
      break;
    case "assistant.completed":
      next = applyAssistantCompleted(next, event);
      break;
    case "tool.call_started":
      next = applyToolCall(next, event, "pending");
      break;
    case "tool.execution_started":
    case "tool.execution_retry":
      next = applyToolCall(next, event, "running");
      break;
    case "tool.execution_result":
      next = applyToolResult(next, event);
      break;
    case "tool.approval_required":
      next = applyApprovalRequired(next, event);
      break;
    case "tool.approval_decided":
      next = applyApprovalDecided(next, event);
      break;
    case "context.compacted":
      next = applyContextCompacted(next, event);
      break;
    case "run.completed":
    case "run.failed":
    case "run.cancelled":
      next = applyRunTerminal(next, event);
      break;
    default:
      break;
  }
  return next;
}

function applyRunCreated(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as RunCreatedPayload;
  const message = state.messages[payload.trigger_message_id];
  const pendingId = `pending:${event.run_id}`;
  const text =
    (message ? decodeText(message.content) : "") ||
    userTextByRun(state, event.run_id) ||
    pendingUserText(state);
  let next = replaceUser(state, pendingId, payload.trigger_message_id, event.run_id, text, event.seq);
  next = dropPendingUsers(next);
  next = upsertUser(next, {
    messageId: payload.trigger_message_id,
    runId: event.run_id,
    text,
    seq: event.seq,
  });
  next.runStatus = payload.status;
  next.activeRunId = event.run_id;
  if (isThinkingPhase(payload.status)) {
    next = upsertThinking(next, event.run_id, payload.status, event.seq);
  }
  return next;
}

function applyRunStateChanged(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as RunStateChangedPayload;
  let next: SessionState = { ...state, runStatus: payload.to, activeRunId: event.run_id };
  if (isThinkingPhase(payload.to)) {
    next = upsertThinking(next, event.run_id, payload.to, event.seq);
  } else {
    next = removeItem(next, thinkingId(event.run_id));
  }
  return next;
}

function applyAssistantStarted(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as AssistantStartedPayload;
  return upsertAssistant(state, {
    runId: event.run_id,
    messageId: payload.message_id,
    text: "",
    streaming: true,
    seq: event.seq,
  });
}

function applyAssistantDelta(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as AssistantDeltaPayload;
  const parsed = parseDelta(payload.delta);
  if (parsed.kind === "text") {
    const existing = findAssistant(state, payload.message_id);
    const text = (existing?.text ?? "") + parsed.text;
    let next = upsertAssistant(state, {
      runId: event.run_id,
      messageId: payload.message_id,
      text,
      streaming: true,
      seq: event.seq,
    });
    if (text) {
      next = removeItem(next, thinkingId(event.run_id));
    }
    return next;
  }
  if (parsed.kind === "tool") {
    return upsertTool(state, {
      runId: event.run_id,
      call: parsed.call,
      state: "pending",
      seq: event.seq,
    });
  }
  return state;
}

function applyAssistantCompleted(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as AssistantCompletedPayload;
  let next = upsertAssistant(state, {
    runId: event.run_id,
    messageId: payload.message_id,
    text: decodeText(payload.text),
    streaming: false,
    seq: event.seq,
  });
  next = removeItem(next, thinkingId(event.run_id));
  for (const call of payload.tool_calls ?? []) {
    next = upsertTool(next, { runId: event.run_id, call, state: "pending", seq: event.seq });
  }
  return next;
}

function applyToolCall(
  state: SessionState,
  event: AgentEvent,
  itemState: "pending" | "running",
): SessionState {
  const payload = event.payload as ToolCallPayload;
  return upsertTool(state, {
    runId: event.run_id,
    call: {
      id: payload.call_id,
      name: payload.name,
      arguments: payload.arguments,
      attempt: payload.attempt,
    },
    state: itemState,
    seq: event.seq,
  });
}

function applyToolResult(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as ToolCallPayload;
  const success = payload.success !== false && !payload.error;
  return upsertTool(state, {
    runId: event.run_id,
    call: {
      id: payload.call_id,
      name: payload.name,
      arguments: payload.arguments,
      attempt: payload.attempt,
    },
    state: success ? "completed" : "error",
    output: payload.output,
    error: payload.error,
    seq: event.seq,
  });
}

function applyApprovalRequired(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as ApprovalRequiredPayload;
  return upsertItem(state, {
    kind: "approval",
    id: approvalId(payload.approval_id),
    runId: event.run_id,
    approvalId: payload.approval_id,
    toolCalls: payload.tool_calls ?? [],
    status: "pending",
    seq: event.seq,
  });
}

function applyApprovalDecided(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as ApprovalDecidedPayload;
  let next = upsertItem(state, {
    kind: "approval",
    id: approvalId(payload.approval_id),
    runId: event.run_id,
    approvalId: payload.approval_id,
    toolCalls: payload.tool_calls ?? existingApprovalCalls(state, payload.approval_id),
    status: payload.status,
    seq: event.seq,
  });
  for (const decision of payload.decisions ?? []) {
    if (decision.status === "denied" || decision.status === "expired") {
      const current = findTool(next, decision.tool_call_id);
      next = upsertTool(next, {
        runId: event.run_id,
        call: {
          id: decision.tool_call_id,
          name: current?.name ?? decision.tool_call_id,
          arguments: current?.arguments,
        },
        state: "denied",
        error: decision.reason || "denied",
        seq: event.seq,
      });
    }
  }
  return next;
}

function applyContextCompacted(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as ContextCompactedPayload;
  return upsertItem(state, {
    kind: "context",
    id: `context:${payload.checkpoint_id}`,
    runId: event.run_id,
    checkpointId: payload.checkpoint_id,
    baseEventSeq: payload.base_event_seq,
    seq: event.seq,
  });
}

function applyRunTerminal(state: SessionState, event: AgentEvent): SessionState {
  const payload = (event.payload ?? {}) as RunTerminalPayload;
  const status = payload.status ?? (event.type.replace("run.", "") as RunStatus);
  let next = removeItem(state, thinkingId(event.run_id));
  next = finishStreaming(next, event.run_id);
  next = upsertItem(next, {
    kind: "terminal",
    id: `terminal:${event.run_id}`,
    runId: event.run_id,
    status,
    stopReason: payload.stop_reason,
    seq: event.seq,
  });
  next.runStatus = status;
  if (isTerminalRun(status) && next.activeRunId === event.run_id) {
    next.activeRunId = null;
  }
  return next;
}

function upsertUser(
  state: SessionState,
  input: { messageId: string; runId: string; text: string; seq: number },
): SessionState {
  if (!input.text) {
    return state;
  }
  return upsertItem(state, {
    kind: "user",
    id: userId(input.messageId),
    runId: input.runId,
    messageId: input.messageId,
    text: input.text,
    seq: input.seq,
  });
}

function replaceUser(
  state: SessionState,
  fromMessageId: string,
  toMessageId: string,
  runId: string,
  text: string,
  seq: number,
): SessionState {
  const from = userId(fromMessageId);
  const index = state.items.findIndex((item) => item.id === from);
  if (index < 0) {
    return state;
  }
  const items = state.items.slice();
  items[index] = {
    kind: "user",
    id: userId(toMessageId),
    runId,
    messageId: toMessageId,
    text: text || (items[index].kind === "user" ? items[index].text : ""),
    seq,
  };
  return { ...state, items };
}

function upsertThinking(
  state: SessionState,
  runId: string,
  phase: ThinkingPhase,
  seq: number,
): SessionState {
  return upsertItem(state, {
    kind: "thinking",
    id: thinkingId(runId),
    runId,
    phase,
    seq,
  });
}

function upsertAssistant(
  state: SessionState,
  input: { runId: string; messageId: string; text: string; streaming: boolean; seq: number },
): SessionState {
  return upsertItem(state, {
    kind: "assistant",
    id: assistantId(input.messageId),
    runId: input.runId,
    messageId: input.messageId,
    text: input.text,
    streaming: input.streaming,
    seq: input.seq,
  });
}

function upsertTool(
  state: SessionState,
  input: {
    runId: string;
    call: ToolCall;
    state: Extract<TimelineItem, { kind: "tool" }>["state"];
    output?: unknown;
    error?: string;
    seq: number;
  },
): SessionState {
  if (!input.call.id) {
    return state;
  }
  const existing = findTool(state, input.call.id);
  return upsertItem(state, {
    kind: "tool",
    id: toolItemId(input.call.id),
    runId: input.runId,
    callId: input.call.id,
    name: input.call.name || existing?.name || input.call.id,
    arguments: input.call.arguments ?? existing?.arguments,
    state: input.state,
    output: input.output ?? existing?.output,
    error: input.error,
    seq: input.seq,
  });
}

function upsertItem(state: SessionState, item: TimelineItem): SessionState {
  const items = state.items.slice();
  const index = items.findIndex((current) => current.id === item.id);
  if (index >= 0) {
    items[index] = { ...items[index], ...item };
  } else {
    items.push(item);
  }
  return { ...state, items };
}

function removeItem(state: SessionState, id: string): SessionState {
  if (!state.items.some((item) => item.id === id)) {
    return state;
  }
  return { ...state, items: state.items.filter((item) => item.id !== id) };
}

function finishStreaming(state: SessionState, runId: string): SessionState {
  let changed = false;
  const items = state.items.map((item) => {
    if (item.kind === "assistant" && item.runId === runId && item.streaming) {
      changed = true;
      return { ...item, streaming: false };
    }
    return item;
  });
  return changed ? { ...state, items } : state;
}

function findAssistant(
  state: SessionState,
  messageId: string,
): Extract<TimelineItem, { kind: "assistant" }> | undefined {
  return state.items.find(
    (item): item is Extract<TimelineItem, { kind: "assistant" }> =>
      item.kind === "assistant" && item.messageId === messageId,
  );
}

function findTool(
  state: SessionState,
  callId: string,
): Extract<TimelineItem, { kind: "tool" }> | undefined {
  return state.items.find(
    (item): item is Extract<TimelineItem, { kind: "tool" }> =>
      item.kind === "tool" && item.callId === callId,
  );
}

function existingApprovalCalls(state: SessionState, approvalId: string): ApprovalToolCall[] {
  const item = state.items.find(
    (current): current is Extract<TimelineItem, { kind: "approval" }> =>
      current.kind === "approval" && current.approvalId === approvalId,
  );
  return item?.toolCalls ?? [];
}

function userTextByRun(state: SessionState, runId: string): string {
  const pending = state.items.find(
    (item): item is Extract<TimelineItem, { kind: "user" }> =>
      item.kind === "user" && (item.runId === runId || item.messageId === `pending:${runId}`),
  );
  return pending?.text ?? "";
}

function pendingUserText(state: SessionState): string {
  const pending = state.items.find(
    (item): item is Extract<TimelineItem, { kind: "user" }> =>
      item.kind === "user" && item.messageId.startsWith("pending:") && Boolean(item.text),
  );
  return pending?.text ?? "";
}

function dropPendingUsers(state: SessionState): SessionState {
  const items = state.items.filter(
    (item) => !(item.kind === "user" && item.messageId.startsWith("pending:")),
  );
  return items.length === state.items.length ? state : { ...state, items };
}

function userId(messageId: string): string {
  return `user:${messageId}`;
}

function thinkingId(runId: string): string {
  return `thinking:${runId}`;
}

function assistantId(messageId: string): string {
  return `assistant:${messageId}`;
}

function toolItemId(callId: string): string {
  return `tool:${callId}`;
}

function approvalId(id: string): string {
  return `approval:${id}`;
}

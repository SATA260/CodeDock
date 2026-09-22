import { decodeText, parseDelta } from "./content.ts";
import {
  isTerminalRun,
  isThinkingPhase,
  type AgentEvent,
  type Approval,
  type ApprovalDecidedPayload,
  type ApprovalDecision,
  type ApprovalRequiredPayload,
  type ApprovalStatus,
  type ApprovalToolCall,
  type AssistantCompletedPayload,
  type AssistantDeltaPayload,
  type ApprovalKind,
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
  type VerifyEventPayload,
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
    queued: false,
    seq: state.lastSeq,
  });
}

/** 本地已发出取消后，先清掉执行中标记，避免新气泡被当成排队。 */
export function applyLocalCancel(state: SessionState, runId: string): SessionState {
  if (!runId || state.activeRunId !== runId) {
    return state;
  }
  return {
    ...state,
    activeRunId: null,
    runStatus: "cancelled",
  };
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
    case "verify.started":
      next = applyVerify(next, event, "started");
      break;
    case "verify.result":
    case "verify.skipped":
      next = applyVerify(next, event, event.type === "verify.skipped" ? "skipped" : "result");
      break;
    case "evaluate.started":
    case "evaluate.result":
      next = removeItem(next, thinkingId(event.run_id));
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
  const pending = findPendingUser(state, event.run_id, payload.text);
  const text =
    (message ? decodeText(message.content) : "") ||
    payload.text ||
    (pending?.kind === "user" ? pending.text : "") ||
    userTextByRun(state, event.run_id);
  const takeActive = canTakeActive(state, event.run_id);
  const queued = payload.status === "queued" && !takeActive;
  let next = state;
  if (pending?.kind === "user") {
    next = replaceUser(next, pending.messageId, payload.trigger_message_id, event.run_id, text, event.seq, queued);
  }
  next = upsertUser(next, {
    messageId: payload.trigger_message_id,
    runId: event.run_id,
    text,
    queued,
    seq: event.seq,
  });
  if (payload.trigger_message_id && text) {
    next = {
      ...next,
      messages: {
        ...next.messages,
        [payload.trigger_message_id]: {
          id: payload.trigger_message_id,
          session_id: event.session_id,
          run_id: event.run_id,
          role: "user",
          content: { text },
          event_seq: event.seq,
          created_at: event.occurred_at,
        },
      },
    };
  }
  if (takeActive) {
    next = { ...next, runStatus: payload.status, activeRunId: event.run_id };
    if (isThinkingPhase(payload.status) && payload.status !== "queued") {
      next = upsertThinking(next, event.run_id, payload.status, event.seq);
    }
  }
  return next;
}

function applyRunStateChanged(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as RunStateChangedPayload;
  const takeActive = canTakeActive(state, event.run_id);
  let next: SessionState = takeActive
    ? { ...state, runStatus: payload.to, activeRunId: event.run_id }
    : state;
  next = setUserQueued(next, event.run_id, false);
  if (isThinkingPhase(payload.to) && payload.to !== "queued") {
    next = upsertThinking(next, event.run_id, payload.to, event.seq);
  } else {
    next = removeItem(next, thinkingId(event.run_id));
  }
  return next;
}

// applyAssistantStarted 占一条流式助手气泡，正文和思考都先空着。
function applyAssistantStarted(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as AssistantStartedPayload;
  return upsertAssistant(state, {
    runId: event.run_id,
    messageId: payload.message_id,
    text: "",
    reasoning: "",
    streaming: true,
    seq: event.seq,
  });
}

// applyAssistantDelta 拼接正文或思考片段；任一到达就撤掉状态字 Thinking。
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
  if (parsed.kind === "reasoning") {
    const existing = findAssistant(state, payload.message_id);
    const reasoning = (existing?.reasoning ?? "") + parsed.text;
    let next = upsertAssistant(state, {
      runId: event.run_id,
      messageId: payload.message_id,
      reasoning,
      streaming: true,
      seq: event.seq,
    });
    if (reasoning) {
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

// applyAssistantCompleted 收齐本轮正文、思考和待执行工具，并结束流式。
function applyAssistantCompleted(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as AssistantCompletedPayload;
  let next = upsertAssistant(state, {
    runId: event.run_id,
    messageId: payload.message_id,
    text: decodeText(payload.text),
    reasoning: payload.reasoning,
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
    toolCalls: mergeApprovalCalls(existingApprovalCalls(state, payload.approval_id), payload.tool_calls ?? []),
    status: "pending",
    approvalKind: payload.kind,
    seq: event.seq,
  });
}

function applyApprovalDecided(state: SessionState, event: AgentEvent): SessionState {
  const payload = event.payload as ApprovalDecidedPayload;
  return applyApprovalDecision(state, {
    approvalId: payload.approval_id,
    runId: event.run_id,
    status: payload.status,
    toolCalls: payload.tool_calls ?? existingApprovalCalls(state, payload.approval_id),
    decisions: payload.decisions ?? [],
    approvalKind: payload.kind ?? existingApprovalKind(state, payload.approval_id),
    seq: event.seq,
  });
}

export function applyApprovals(state: SessionState, approvals: Approval[]): SessionState {
  let next = state;
  for (const approval of approvals) {
    next = applyApprovalRecord(next, approval);
  }
  return next;
}

export function applyApprovalRecord(state: SessionState, approval: Approval): SessionState {
  const toolCalls = mergeApprovalCalls(
    existingApprovalCalls(state, approval.id),
    approval.tool_calls ?? [],
  );
  return applyApprovalDecision(state, {
    approvalId: approval.id,
    runId: approval.run_id,
    status: approval.status,
    toolCalls,
    decisions: toolCalls.map((call) => ({
      tool_call_id: call.id,
      status: call.status ?? approval.status,
      reason: call.reason,
    })),
    approvalKind: approval.kind,
    seq: state.lastSeq,
  });
}

function applyApprovalDecision(
  state: SessionState,
  input: {
    approvalId: string;
    runId: string;
    status: ApprovalStatus;
    toolCalls: ApprovalToolCall[];
    decisions: ApprovalDecision[];
    approvalKind?: ApprovalKind;
    seq: number;
  },
): SessionState {
  let next = upsertItem(state, {
    kind: "approval",
    id: approvalId(input.approvalId),
    runId: input.runId,
    approvalId: input.approvalId,
    toolCalls: input.toolCalls,
    status: input.status,
    approvalKind: input.approvalKind ?? existingApprovalKind(state, input.approvalId),
    seq: input.seq,
  });
  for (const decision of input.decisions) {
    if (decision.status === "denied" || decision.status === "expired") {
      const current = findTool(next, decision.tool_call_id);
      next = upsertTool(next, {
        runId: input.runId,
        call: {
          id: decision.tool_call_id,
          name: current?.name ?? decision.tool_call_id,
          arguments: current?.arguments,
        },
        state: "denied",
        error: decision.reason || "denied",
        seq: input.seq,
      });
    }
  }
  return next;
}

/** 把收尾验证事件折成时间线卡片；跳过验证不画卡片。 */
function applyVerify(
  state: SessionState,
  event: AgentEvent,
  stage: "started" | "result" | "skipped",
): SessionState {
  const payload = (event.payload ?? {}) as VerifyEventPayload;
  let next = removeItem(state, thinkingId(event.run_id));
  if (stage === "skipped" || payload.skipped) {
    return removeItem(next, `verify:${event.run_id}`);
  }
  let status: Extract<TimelineItem, { kind: "verify" }>["status"] = "started";
  if (stage === "result") {
    status = payload.status === "failed" || payload.status === "cannot_run" ? payload.status : "passed";
  }
  return upsertItem(next, {
    kind: "verify",
    id: `verify:${event.run_id}`,
    runId: event.run_id,
    status,
    output: payload.output,
    round: payload.round,
    seq: event.seq,
  });
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
    error: payload.error,
    seq: event.seq,
  });
  if (canTakeActive(state, event.run_id)) {
    next.runStatus = status;
  }
  next = setUserQueued(next, event.run_id, false);
  if (isTerminalRun(status) && next.activeRunId === event.run_id) {
    next.activeRunId = null;
  }
  return next;
}

export function applyUserText(
  state: SessionState,
  messageId: string,
  text: string,
  queued = true,
): SessionState {
  const existing = state.items.find(
    (item): item is Extract<TimelineItem, { kind: "user" }> =>
      item.kind === "user" && item.messageId === messageId,
  );
  if (!existing) {
    return state;
  }
  let next = upsertUser(state, {
    messageId,
    runId: existing.runId,
    text,
    queued,
    seq: existing.seq,
  });
  const current = next.messages[messageId];
  if (current) {
    next = {
      ...next,
      messages: { ...next.messages, [messageId]: { ...current, content: { text } } },
    };
  }
  return next;
}

function upsertUser(
  state: SessionState,
  input: { messageId: string; runId: string; text: string; seq: number; queued?: boolean },
): SessionState {
  if (!input.text) {
    return state;
  }
  const existing = state.items.find(
    (item): item is Extract<TimelineItem, { kind: "user" }> =>
      item.kind === "user" && item.messageId === input.messageId,
  );
  return upsertItem(state, {
    kind: "user",
    id: userId(input.messageId),
    runId: input.runId,
    messageId: input.messageId,
    text: input.text,
    queued: input.queued ?? existing?.queued,
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
  queued?: boolean,
): SessionState {
  const from = userId(fromMessageId);
  const index = state.items.findIndex((item) => item.id === from);
  if (index < 0) {
    return state;
  }
  const items = state.items.slice();
  const current = items[index];
  items[index] = {
    kind: "user",
    id: userId(toMessageId),
    runId,
    messageId: toMessageId,
    text: text || (current.kind === "user" ? current.text : ""),
    queued: queued ?? (current.kind === "user" ? current.queued : undefined),
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

// upsertAssistant 更新同一条助手消息；未传入的正文或思考沿用已有值。
function upsertAssistant(
  state: SessionState,
  input: {
    runId: string;
    messageId: string;
    text?: string;
    reasoning?: string;
    streaming: boolean;
    seq: number;
  },
): SessionState {
  const existing = findAssistant(state, input.messageId);
  return upsertItem(state, {
    kind: "assistant",
    id: assistantId(input.messageId),
    runId: input.runId,
    messageId: input.messageId,
    text: input.text ?? existing?.text ?? "",
    reasoning: input.reasoning ?? existing?.reasoning,
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
    const current = items[index];
    items[index] = current.kind === item.kind ? ({ ...current, ...item } as TimelineItem) : item;
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

export function decisionsForApproval(
  approval: Approval,
  requested: ApprovalDecision[],
): ApprovalDecision[] {
  const calls = approval.tool_calls ?? [];
  if (calls.length === 0) {
    return requested;
  }
  const byId = new Map(requested.map((item) => [item.tool_call_id, item]));
  const fallback = requested[0]?.status ?? approval.status;
  const status = fallback === "denied" || fallback === "approved" ? fallback : "approved";
  return calls
    .filter((call) => Boolean(call.id))
    .map((call) => byId.get(call.id) ?? { tool_call_id: call.id, status });
}

function mergeApprovalCalls(current: ApprovalToolCall[], incoming: ApprovalToolCall[]): ApprovalToolCall[] {
  const byId = new Map<string, ApprovalToolCall>();
  for (const call of [...current, ...incoming]) {
    if (!call.id) {
      continue;
    }
    byId.set(call.id, { ...byId.get(call.id), ...call });
  }
  return [...byId.values()];
}

/** 读出时间线上已有审批卡片的 kind。 */
function existingApprovalKind(state: SessionState, approvalId: string): ApprovalKind | undefined {
  const item = state.items.find(
    (current): current is Extract<TimelineItem, { kind: "approval" }> =>
      current.kind === "approval" && current.approvalId === approvalId,
  );
  return item?.approvalKind;
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

function hasExecutingRun(state: SessionState, exceptRunId?: string): boolean {
  if (!state.activeRunId || state.activeRunId === exceptRunId || !state.runStatus) {
    return false;
  }
  return !isTerminalRun(state.runStatus);
}

function canTakeActive(state: SessionState, runId: string): boolean {
  return !hasExecutingRun(state, runId);
}

function findPendingUser(
  state: SessionState,
  runId: string,
  text?: string,
): Extract<TimelineItem, { kind: "user" }> | undefined {
  const exact = state.items.find(
    (item): item is Extract<TimelineItem, { kind: "user" }> =>
      item.kind === "user" && (item.runId === runId || item.messageId === `pending:${runId}`),
  );
  if (exact) {
    return exact;
  }
  if (text) {
    const byText = state.items.find(
      (item): item is Extract<TimelineItem, { kind: "user" }> =>
        item.kind === "user" && item.messageId.startsWith("pending:") && item.text === text,
    );
    if (byText) {
      return byText;
    }
  }
  return state.items.find(
    (item): item is Extract<TimelineItem, { kind: "user" }> =>
      item.kind === "user" && item.messageId.startsWith("pending:"),
  );
}

function setUserQueued(state: SessionState, runId: string, queued: boolean): SessionState {
  let changed = false;
  const items = state.items.map((item) => {
    if (item.kind === "user" && item.runId === runId && item.queued !== queued) {
      changed = true;
      return { ...item, queued };
    }
    return item;
  });
  return changed ? { ...state, items } : state;
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

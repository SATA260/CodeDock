export type SessionStatus = "active" | "archived";

export type AgentMode =
  | "ask_for_approval"
  | "auto_approve"
  | "yolo"
  | "ask"
  | "plan";

export type RunStatus =
  | "queued"
  | "loading_context"
  | "running_llm"
  | "executing_tools"
  | "waiting_approval"
  | "cancelling"
  | "completed"
  | "failed"
  | "cancelled";

export type ApprovalStatus = "pending" | "approved" | "denied" | "expired";

export type ApprovalScope = "once" | "run" | "session";

export type MessageRole = "user" | "assistant" | "tool" | "system";

export type EventType =
  | "run.created"
  | "run.state_changed"
  | "turn.started"
  | "assistant.started"
  | "assistant.delta"
  | "assistant.completed"
  | "tool.call_started"
  | "tool.approval_required"
  | "tool.approval_decided"
  | "tool.execution_started"
  | "tool.execution_retry"
  | "tool.execution_result"
  | "turn.usage_recorded"
  | "context.compacted"
  | "turn.completed"
  | "run.completed"
  | "run.failed"
  | "run.cancelled";

export type ThinkingPhase = "queued" | "loading_context" | "running_llm";

export type ToolItemState =
  | "pending"
  | "running"
  | "completed"
  | "error"
  | "denied";

export interface Session {
  id: string;
  tenant_id: string;
  user_id: string;
  agent_id: string;
  workspace_id: string;
  status: SessionStatus;
  active_run_id?: string;
  last_event_seq: number;
  compaction_seq: number;
  summary?: string;
  created_at: string;
  updated_at: string;
}

export interface PageInfo {
  page: number;
  page_size: number;
  sort_by: string;
  sort_order: string;
  total: number;
}

export interface ToolCall {
  id: string;
  name: string;
  arguments?: unknown;
  attempt?: number;
  idempotency_key?: string;
}

export interface Message {
  id: string;
  session_id: string;
  run_id?: string;
  turn_id?: string;
  role: MessageRole;
  content: unknown;
  attachments?: unknown[];
  tool_calls?: ToolCall[];
  event_seq: number;
  created_at: string;
}

export interface ApprovalToolCall {
  id: string;
  name: string;
  arguments?: unknown;
  status?: ApprovalStatus;
  reason?: string;
}

export interface Approval {
  id: string;
  session_id: string;
  run_id: string;
  tool_call_id: string;
  tool_calls: ApprovalToolCall[];
  scope: ApprovalScope;
  status: ApprovalStatus;
  expires_at: string;
}

export interface AgentEvent<T = unknown> {
  event_id: string;
  session_id: string;
  run_id: string;
  turn_id?: string;
  seq: number;
  type: EventType;
  version: number;
  occurred_at: string;
  payload: T;
}

export interface RunCreatedPayload {
  trigger_message_id: string;
  mode: AgentMode;
  status: RunStatus;
}

export interface RunStateChangedPayload {
  from: RunStatus;
  to: RunStatus;
  reason: string;
}

export interface AssistantStartedPayload {
  message_id: string;
}

export interface AssistantDeltaPayload {
  message_id: string;
  delta: unknown;
}

export interface AssistantCompletedPayload {
  message_id: string;
  text: string;
  tool_calls?: ToolCall[];
}

export interface ToolCallPayload {
  call_id: string;
  name: string;
  arguments?: unknown;
  attempt?: number;
  success?: boolean;
  error?: string;
  output?: unknown;
  approval_id?: string;
}

export interface ApprovalRequiredPayload {
  approval_id: string;
  tool_calls: ApprovalToolCall[];
}

export interface ApprovalDecision {
  tool_call_id: string;
  status: ApprovalStatus;
  reason?: string;
}

export interface ApprovalDecidedPayload {
  approval_id: string;
  tool_call_id?: string;
  status: ApprovalStatus;
  scope: ApprovalScope;
  reason?: string;
  decisions?: ApprovalDecision[];
  tool_calls?: ApprovalToolCall[];
}

export interface ContextCompactedPayload {
  checkpoint_id: string;
  base_event_seq: number;
}

export interface RunTerminalPayload {
  status: RunStatus;
  stop_reason?: string;
}

export interface CreateSessionRequest {
  tenant_id?: string;
  user_id: string;
  agent_id?: string;
  workspace_id?: string;
}

export interface StartRunRequest {
  content: string;
  input_mode?: "interrupt" | "queue";
  mode?: AgentMode;
}

export interface StartRunResponse {
  session_id: string;
  run_id: string;
}

export interface DecideApprovalRequest {
  decisions: ApprovalDecision[];
  scope?: ApprovalScope;
  actor_id?: string;
  reason?: string;
}

export type TimelineItem =
  | {
      kind: "user";
      id: string;
      runId: string;
      messageId: string;
      text: string;
      seq: number;
    }
  | {
      kind: "thinking";
      id: string;
      runId: string;
      phase: ThinkingPhase;
      seq: number;
    }
  | {
      kind: "assistant";
      id: string;
      runId: string;
      messageId: string;
      text: string;
      streaming: boolean;
      seq: number;
    }
  | {
      kind: "tool";
      id: string;
      runId: string;
      callId: string;
      name: string;
      arguments?: unknown;
      state: ToolItemState;
      output?: unknown;
      error?: string;
      seq: number;
    }
  | {
      kind: "approval";
      id: string;
      runId: string;
      approvalId: string;
      toolCalls: ApprovalToolCall[];
      status: ApprovalStatus;
      seq: number;
    }
  | {
      kind: "context";
      id: string;
      runId: string;
      checkpointId: string;
      baseEventSeq: number;
      seq: number;
    }
  | {
      kind: "terminal";
      id: string;
      runId: string;
      status: RunStatus;
      stopReason?: string;
      seq: number;
    };

export interface SessionState {
  lastSeq: number;
  activeRunId: string | null;
  runStatus: RunStatus | null;
  items: TimelineItem[];
  messages: Record<string, Message>;
}

export const THINKING_PHASES: readonly ThinkingPhase[] = [
  "queued",
  "loading_context",
  "running_llm",
];

export const TERMINAL_RUN_STATUSES: readonly RunStatus[] = [
  "completed",
  "failed",
  "cancelled",
];

export function isThinkingPhase(status: string): status is ThinkingPhase {
  return (THINKING_PHASES as readonly string[]).includes(status);
}

export function isTerminalRun(status: string): boolean {
  return (TERMINAL_RUN_STATUSES as readonly string[]).includes(status);
}

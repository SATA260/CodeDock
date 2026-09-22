export type SessionStatus = "active" | "archived";

export type WorkMode = "ask" | "plan" | "agent";

export type ApprovalMode = "manual" | "auto" | "yolo";

export type RunStatus =
  | "queued"
  | "loading_context"
  | "running_llm"
  | "executing_tools"
  | "waiting_approval"
  | "verifying"
  | "evaluating"
  | "cancelling"
  | "completed"
  | "failed"
  | "cancelled";

export type ApprovalKind = "tools" | "verify" | "evaluate";

export type OverrideAction = "accept" | "retry" | "abort";

export type RestoreMode = "restore_files" | "restore_messages" | "restore_all";

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
  | "run.cancelled"
  | "verify.started"
  | "verify.result"
  | "verify.skipped"
  | "evaluate.started"
  | "evaluate.result"
  | "snapshot.skipped";

export type ThinkingPhase =
  | "queued"
  | "loading_context"
  | "running_llm"
  | "executing_tools"
  | "waiting_approval"
  | "verifying"
  | "evaluating"
  | "cancelling";

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
  /** 创建会话时冻结的工作目录（绝对路径）；本会话权限只覆盖该目录。 */
  workspace_id: string;
  status: SessionStatus;
  active_run_id?: string;
  /** 当前 active Run 已中断且 Worker 不在跑，界面才应显示「恢复」。 */
  needs_recover?: boolean;
  /** 当前 active Run 还在执行。等审批、已结束、需要恢复时为 false；缺省表示这条响应没带。 */
  executing?: boolean;
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
  kind?: ApprovalKind;
  override?: OverrideAction;
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
  mode: WorkMode;
  approval?: ApprovalMode;
  status: RunStatus;
  text?: string;
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
  reasoning?: string;
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
  kind?: ApprovalKind;
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
  kind?: ApprovalKind;
  override?: OverrideAction;
}

export interface VerifyEventPayload {
  round?: number;
  status?: "passed" | "failed" | "cannot_run";
  output?: string;
  skipped?: boolean;
  fingerprint?: string;
}

export interface EvaluateEventPayload {
  round?: number;
  verdict?: "pass" | "needs_work" | "escalate";
  summary?: string;
  issues?: Array<{ file_path?: string; line?: number; category?: string; reason?: string }>;
}

export interface ContextCompactedPayload {
  checkpoint_id: string;
  base_event_seq: number;
}

export interface RunTerminalPayload {
  status: RunStatus;
  stop_reason?: string;
  error?: string;
}

export interface CreateSessionRequest {
  tenant_id?: string;
  user_id: string;
  agent_id?: string;
  /** 工作目录。显式路径必须已存在，服务端冻结为绝对路径；省略则 GIT_REPO / cwd。 */
  workspace_id?: string;
}

export interface Run {
  id: string;
  session_id: string;
  status: RunStatus;
  mode?: WorkMode;
  approval?: ApprovalMode;
  cancel_requested?: boolean;
  needs_recover?: boolean;
}

/** RecoverRun 能接着跑的状态；界面是否显示「恢复」还要看 needs_recover（Worker 已不在跑）。 */
export const RECOVERABLE_RUN_STATUSES: readonly RunStatus[] = [
  "queued",
  "loading_context",
  "running_llm",
  "executing_tools",
  "verifying",
  "evaluating",
];

export function isRecoverableRun(status: string): boolean {
  return (RECOVERABLE_RUN_STATUSES as readonly string[]).includes(status);
}

export interface StartRunRequest {
  content: string;
  mode?: WorkMode;
  approval?: ApprovalMode;
}

export interface StartRunResponse {
  session_id: string;
  run_id?: string;
  handled?: boolean;
}

export interface DecideApprovalRequest {
  decisions?: ApprovalDecision[];
  scope?: ApprovalScope;
  actor_id?: string;
  reason?: string;
  override?: OverrideAction;
}

export type TimelineItem =
  | {
      kind: "user";
      id: string;
      runId: string;
      messageId: string;
      text: string;
      queued?: boolean;
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
      reasoning?: string;
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
      approvalKind?: ApprovalKind;
      seq: number;
    }
    | {
      kind: "verify";
      id: string;
      runId: string;
      status: "started" | "passed" | "failed" | "skipped" | "cannot_run";
      output?: string;
      round?: number;
      seq: number;
    }
    | {
      kind: "evaluate";
      id: string;
      runId: string;
      status: "started" | "pass" | "needs_work" | "escalate";
      summary?: string;
      round?: number;
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
      error?: string;
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
  "executing_tools",
  "waiting_approval",
  "verifying",
  "evaluating",
  "cancelling",
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

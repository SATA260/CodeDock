export { AgentClient, AgentClientError, type AgentClientOptions } from "./client.ts";
export { decodeText, firstLine, parseDelta } from "./content.ts";
export { joinQueuedTexts } from "./queue.ts";
export {
  applyApprovalRecord,
  applyApprovals,
  applyEvent,
  decisionsForApproval,
  applyOptimisticUser,
  applyUserText,
  dropOptimisticUser,
  emptyState,
  hydrate,
  indexMessages,
} from "./reducer.ts";
export { parseSSEBlock, parseSSEChunk, watchEvents, type WatchEventsOptions } from "./sse.ts";
export type {
  AgentEvent,
  AgentMode,
  Approval,
  ApprovalDecision,
  ApprovalStatus,
  ApprovalToolCall,
  CreateSessionRequest,
  DecideApprovalRequest,
  EventType,
  Message,
  PageInfo,
  Run,
  RunStatus,
  Session,
  SessionState,
  StartRunRequest,
  StartRunResponse,
  ThinkingPhase,
  TimelineItem,
  ToolCall,
  ToolItemState,
} from "./types.ts";
export {
  isRecoverableRun,
  isTerminalRun,
  isThinkingPhase,
  RECOVERABLE_RUN_STATUSES,
  TERMINAL_RUN_STATUSES,
  THINKING_PHASES,
} from "./types.ts";

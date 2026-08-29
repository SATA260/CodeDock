export { AgentClient, AgentClientError, type AgentClientOptions } from "./client.ts";
export { decodeText, firstLine, parseDelta } from "./content.ts";
export {
  applyEvent,
  applyOptimisticUser,
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
export { isTerminalRun, isThinkingPhase, TERMINAL_RUN_STATUSES, THINKING_PHASES } from "./types.ts";

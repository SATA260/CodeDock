export { AgentClient, AgentClientError, type AgentClientOptions } from "./client.ts";
export { decodeText, firstLine, parseDelta } from "./content.ts";
export {
  collectRunFileGroups,
  compactToolDump,
  diffViewHunks,
  fileChangeFromTool,
  fileKey,
  isFileChangeTool,
  unifiedPatch,
  type FileChangePreview,
  type RunFileGroup,
} from "./files.ts";
export {
  isPlanTool,
  latestPlanDocIds,
  normalizePlanName,
  planPreviewFromTool,
  planReadableContent,
  planToolDump,
  type PlanDocPreview,
  type PlanPreview,
} from "./plan.ts";
export { joinQueuedTexts } from "./queue.ts";
export {
  applyApprovalRecord,
  applyApprovals,
  applyEvent,
  decisionsForApproval,
  applyLocalCancel,
  applyOptimisticUser,
  applyUserText,
  dropOptimisticUser,
  emptyState,
  hydrate,
  indexMessages,
} from "./reducer.ts";
export { parseSSEBlock, parseSSEChunk, watchEvents, type WatchEventsOptions } from "./sse.ts";
export { waitUntilRunReleased } from "./wait-run.ts";
export type {
  AgentEvent,
  ApprovalMode,
  WorkMode,
  Approval,
  ApprovalDecision,
  ApprovalKind,
  ApprovalStatus,
  ApprovalToolCall,
  CreateSessionRequest,
  DecideApprovalRequest,
  EventType,
  Message,
  OverrideAction,
  PageInfo,
  RestoreMode,
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

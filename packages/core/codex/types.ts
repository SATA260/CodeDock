export type EngineStatus = {
  available: boolean;
  authorized: boolean;
  version?: string;
  hint?: string;
};

export type ModelInfo = {
  id: string;
  display_name?: string;
  efforts: string[];
  default_effort: string;
  hidden: boolean;
  is_default: boolean;
};

export type ModeInfo = {
  id: string;
  label?: string;
  kind: "collaboration" | "permission" | string;
  approval?: string;
  sandbox?: string;
  allowed: boolean;
};

export type CommandAction = "apply_settings" | "turn" | "session" | "attach" | "hint" | string;

export type CommandSpec = {
  name: string;
  action: CommandAction;
  hint?: string;
  field?: string;
};

export type CommandResult = {
  hint?: string;
  action?: CommandAction;
  handled: boolean;
};

export type Session = {
  id: string;
  thread_id: string;
  title?: string;
  preview?: string;
  cwd?: string;
  active_turn_id?: string;
  /** 有进行中的回合且没有待审批。缺省表示这条响应没带。 */
  running?: boolean;
  archived: boolean;
  ephemeral?: boolean;
  created_at?: number;
  updated_at?: number;
};

export type SessionPage = {
  sessions: Session[];
  next_cursor?: string;
};

export type Settings = {
  model?: string;
  effort?: string;
  collaboration_mode?: string;
  approval_policy?: string;
  sandbox?: string;
  cwd?: string;
  overridden?: string[];
};

export type InputMode = "start" | "queue";

export type TurnStatus =
  | "queued"
  | "running"
  | "waiting_approval"
  | "completed"
  | "failed"
  | "cancelled";

export type Turn = {
  id: string;
  session_id: string;
  codex_id?: string;
  status: TurnStatus;
  error?: string;
};

export type Input = {
  text?: string;
  mentions?: string[];
  images?: string[];
};

export type StartTurnRequest = {
  content?: string;
  input?: Input;
  mode?: InputMode;
};

export type ProgressKind = "user" | "text" | "reasoning" | "command" | "file_change" | "plan" | "notice";

export type Progress = {
  kind: ProgressKind;
  item_id?: string;
  text?: string;
  command?: string;
  paths?: string[];
  diff?: string;
  status?: string;
};

export type AskKind = "command" | "file_change" | "question" | "form" | "permissions";

export type DecisionScope = "once" | "session";

export type AskOption = {
  id?: string;
  label: string;
  recommended?: boolean;
  other?: boolean;
};

export type AskQuestion = {
  id?: string;
  header?: string;
  prompt?: string;
  options?: AskOption[];
};

export type ApprovalAsk = {
  id: string;
  kind: AskKind;
  thread_id?: string;
  turn_id?: string;
  method?: string;
  command?: string;
  paths?: string[];
  diff?: string;
  prompt?: string;
  options?: string[];
  questions?: AskQuestion[];
  fields?: string[];
  external_request_id: string;
};

export type AskAnswer = {
  approved: boolean;
  scope?: DecisionScope;
  choice?: string;
  values?: string[];
  answers?: Record<string, string>;
};

export type CodexEventType =
  | "turn.queued"
  | "turn.started"
  | "turn.completed"
  | "turn.failed"
  | "turn.cancelled"
  | "progress"
  | "ask.required"
  | "ask.resolved"
  | "notice"
  | "reset"
  | "token.usage";

export type TokenUsage = {
  used: number;
  window: number;
};

export type CodexEvent = {
  seq: number;
  type: CodexEventType;
  session_id: string;
  turn_id?: string;
  progress?: Progress;
  turn?: Turn;
  ask?: ApprovalAsk;
  notice?: string;
  usage?: TokenUsage;
};

export type SessionDetail = {
  session: Session;
  progress: Progress[];
  asks: ApprovalAsk[];
  usage?: TokenUsage;
};

export type TimelineItem = {
  id: string;
  kind: ProgressKind | "turn";
  text?: string;
  command?: string;
  paths?: string[];
  diff?: string;
  status?: string;
  streaming?: boolean;
  error?: string;
};

export type CodexViewState = {
  session: Session | null;
  settings: Settings;
  items: TimelineItem[];
  asks: ApprovalAsk[];
  lastSeq: number;
  activeTurn: Turn | null;
  reset: boolean;
  usage?: TokenUsage;
};

export const LIVE_TURN_STATUSES: readonly TurnStatus[] = ["queued", "running", "waiting_approval"];

export function isLiveTurn(status: TurnStatus | undefined): boolean {
  return Boolean(status && (LIVE_TURN_STATUSES as readonly string[]).includes(status));
}

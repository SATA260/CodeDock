export type ClaudeStatus = {
  available: boolean;
  authorized: boolean;
  version: string;
  hint: string;
};

export type ClaudeModel = {
  id: string;
  efforts: string[];
  default_effort: string;
  hidden: boolean;
  is_default: boolean;
};

export type ClaudeMode = {
  id: string;
  kind: string;
};

export type ClaudeCommand = {
  name: string;
  action: string;
  hint: string;
};

export type ClaudeSession = {
  id: string;
  claude_session_id: string;
  title: string;
  active_turn_id: string;
  archived: boolean;
  /** 本机实录第一条时间，Unix 秒。 */
  created_at?: number;
  /** 本机实录最近一条时间，Unix 秒。 */
  updated_at?: number;
};

export type ClaudeSettings = {
  model: string;
  effort: string;
  permission_mode: string;
  cwd: string;
  overridden: string[];
};

export type ClaudeSettingsPatch = {
  model?: string;
  effort?: string;
  permission_mode?: string;
  cwd?: string;
  overridden?: string[];
};

export type ClaudeInput = {
  text: string;
  mentions: string[];
  images: string[];
};

export type ClaudeStartTurnRequest = {
  content: string;
  input?: Partial<ClaudeInput>;
  mode?: "start" | "queue";
};

export type ClaudeProgress = {
  kind: "user" | "text" | "reasoning" | "command" | "file_change" | "plan" | "notice" | string;
  text: string;
  command: string;
  paths: string[];
  diff: string;
};

export type ClaudeTimelineItem = ClaudeProgress & {
  id: string;
  streaming?: boolean;
};

export type ClaudeTokenUsage = {
  used: number;
  window: number;
};

export type ClaudeTranscript = {
  items: ClaudeProgress[];
  usage?: ClaudeTokenUsage;
};

export type ClaudeDecision = {
  approved: boolean;
  scope?: "once" | "session" | string;
  choice?: string;
  values?: string[];
};

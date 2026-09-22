export type BoardEngine = "agent" | "claude" | "codex";

export type CheckoutKind = "folder" | "primary" | "worktree";

export type Work = {
  id: string;
  tenant_id: string;
  user_id: string;
  title: string;
  created_at: string;
  updated_at: string;
};

export type WorkInfo = {
  work_id: string;
  checkout: string;
  body: string;
  updated_at: string;
};

export type Checkout = {
  work_id: string;
  path: string;
  kind: CheckoutKind;
};

export type Placement = {
  engine: BoardEngine | "native";
  session_id: string;
  work_id: string;
  checkout: string;
};

export type IssueSnap = {
  engine: string;
  session_id: string;
  repo: string;
  number: number;
  title: string;
  body: string;
  url: string;
  state: string;
  updated_at: string;
};

export type PullSnap = {
  engine: string;
  session_id: string;
  repo: string;
  number: number;
  title: string;
  body: string;
  url: string;
  state: string;
  updated_at: string;
};

export type SessionLinks = {
  issue?: IssueSnap | null;
  pulls: PullSnap[];
};

export type SessionView = {
  engine: BoardEngine;
  session_id: string;
  summary: string;
  checkout: string;
  running: boolean;
  pending: number;
  updated_at: string;
};

export type DirView = {
  path: string;
  kind: string;
  info: WorkInfo;
  branch: string;
  dirty: boolean;
  shared_writers: string[];
};

export type Card = {
  work: Work;
  info: WorkInfo;
  dirs: DirView[];
  sessions: SessionView[];
  running: number;
  pending: number;
};

export type BoardView = {
  cards: Card[];
  ungrouped: SessionView[];
};

export type InboxItem = {
  engine: BoardEngine;
  session_id: string;
  ticket_id: string;
  summary: string;
  payload: unknown;
};

export type Packet = {
  work_info: WorkInfo;
  dir_info?: WorkInfo | null;
  issue?: IssueSnap | null;
  pulls: PullSnap[];
  text: string;
};

export type StartWorkSessionRequest = {
  engine: BoardEngine;
  kind: "talk" | "dir";
  checkout?: string;
  agent_id?: string;
};

export type InboxDecideRequest = {
  engine: BoardEngine;
  ticket_id: string;
  decisions?: { tool_call_id: string; status: string; reason?: string }[];
  status?: string;
  scope?: string;
  actor_id?: string;
  reason?: string;
  override?: string;
  approved?: boolean;
  choice?: string;
  values?: string[];
  answers?: Record<string, string>;
};

export type PutLinkRequest = {
  repo?: string;
  number?: number;
  ref?: string;
};

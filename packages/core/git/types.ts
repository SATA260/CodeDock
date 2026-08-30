export type Remote = {
  name: string;
  fetch_url: string;
  push_url: string;
};

export type FileStatus = {
  path: string;
  orig_path: string;
  staged_status: string;
  worktree_status: string;
  unmerged: boolean;
};

export type SiteState = {
  path: string;
  is_repo: boolean;
  empty: boolean;
  branch: string;
  head: string;
  detached: boolean;
  upstream: string;
  ahead: number;
  behind: number;
  upstream_gone: boolean;
  integrating: string;
  default_branch: string;
  files: FileStatus[];
  remotes: Remote[];
};

export type Commit = {
  id: string;
  parents: string[];
  title: string;
  body: string;
  author: string;
  date: string;
};

export type Ref = {
  name: string;
  kind: "local" | "remote" | "tag" | "head" | string;
};

export type GraphNode = {
  commit: Commit;
  refs: Ref[];
};

export type GraphEdge = {
  child: string;
  parent: string;
};

export type Graph = {
  nodes: GraphNode[];
  edges: GraphEdge[];
};

export type Branch = {
  name: string;
  head: string;
  is_current: boolean;
  is_remote: boolean;
  upstream: string;
  ahead: number;
  behind: number;
  upstream_gone: boolean;
  worktree_path: string;
  title: string;
};

export type BranchView = {
  current: string;
  locals: Branch[];
  remotes: Branch[];
  graph: Graph;
};

export type CommitRequest = {
  message: string;
  paths?: string[];
  checkout?: string;
};

export type DiffScope = "staged" | "worktree";

export type DiffFile = {
  path: string;
  orig_path: string;
  kind: string;
  binary: boolean;
  patch: string;
};

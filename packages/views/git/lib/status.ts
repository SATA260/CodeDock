import type { FileStatus } from "@codedock/core/git";

export function letterDirty(letter: string): boolean {
  return letter !== "" && letter !== " " && letter !== ".";
}

export function isStaged(file: FileStatus): boolean {
  return letterDirty(file.staged_status);
}

export function isWorktreeDirty(file: FileStatus): boolean {
  return letterDirty(file.worktree_status);
}

export function splitWorkspaceFiles(files: FileStatus[]): {
  staged: FileStatus[];
  worktree: FileStatus[];
} {
  return {
    staged: files.filter((file) => isStaged(file) && !file.unmerged),
    worktree: files.filter((file) => isWorktreeDirty(file) || file.unmerged),
  };
}

export function stagedLabel(file: FileStatus): string {
  if (file.unmerged) {
    return "冲突";
  }
  switch (file.staged_status) {
    case "A":
      return "新增";
    case "D":
      return "删除";
    case "R":
      return "重命名";
    case "C":
      return "复制";
    default:
      return "已修改";
  }
}

export function worktreeLabel(file: FileStatus): string {
  if (file.unmerged) {
    return "冲突";
  }
  if (file.worktree_status === "?") {
    return "未跟踪";
  }
  if (file.worktree_status === "D") {
    return "删除";
  }
  return "已修改";
}

export function localNameFromRemote(remoteName: string): string {
  const slash = remoteName.indexOf("/");
  return slash === -1 ? remoteName : remoteName.slice(slash + 1);
}

export function shortHead(head: string): string {
  return head.slice(0, 8);
}

export function trackLabel(ahead: number, behind: number, upstream: string, gone: boolean): string {
  if (!upstream) {
    return "未跟踪远程";
  }
  if (gone) {
    return `${upstream} 已删除`;
  }
  const bits: string[] = [];
  if (ahead > 0) {
    bits.push(`超前 ${ahead}`);
  }
  if (behind > 0) {
    bits.push(`落后 ${behind}`);
  }
  return bits.length > 0 ? `${upstream} · ${bits.join(" · ")}` : upstream;
}

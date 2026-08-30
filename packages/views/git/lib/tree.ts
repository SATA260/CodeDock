import type { FileStatus } from "@codedock/core/git";

export type FileTreeNode = {
  name: string;
  path: string;
  file?: FileStatus;
  children: FileTreeNode[];
};

export function buildFileTree(files: FileStatus[]): FileTreeNode[] {
  const root: FileTreeNode[] = [];
  const dirs = new Map<string, FileTreeNode>();

  const ensureDir = (dirPath: string): FileTreeNode[] => {
    if (!dirPath) {
      return root;
    }
    const existing = dirs.get(dirPath);
    if (existing) {
      return existing.children;
    }
    const parts = dirPath.split("/");
    const name = parts[parts.length - 1] ?? dirPath;
    const parentPath = parts.slice(0, -1).join("/");
    const node: FileTreeNode = { name, path: dirPath, children: [] };
    dirs.set(dirPath, node);
    ensureDir(parentPath).push(node);
    return node.children;
  };

  const sorted = [...files].sort((a, b) => a.path.localeCompare(b.path));
  for (const file of sorted) {
    const normalized = file.path.replace(/\/+$/, "");
    if (!normalized) {
      continue;
    }
    if (file.path.endsWith("/")) {
      ensureDir(normalized);
      continue;
    }
    const parts = normalized.split("/").filter(Boolean);
    const name = parts[parts.length - 1] ?? normalized;
    const dir = parts.slice(0, -1).join("/");
    ensureDir(dir).push({ name, path: file.path, file, children: [] });
  }

  const sortNodes = (nodes: FileTreeNode[]) => {
    nodes.sort((a, b) => {
      const dirFirst = Number(Boolean(a.file)) - Number(Boolean(b.file));
      if (dirFirst !== 0) {
        return dirFirst;
      }
      return a.name.localeCompare(b.name);
    });
    for (const node of nodes) {
      sortNodes(node.children);
    }
  };
  sortNodes(root);
  return root;
}

export function collectFilePaths(node: FileTreeNode): string[] {
  if (node.file) {
    return [node.file.path];
  }
  return node.children.flatMap(collectFilePaths);
}

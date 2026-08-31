import type { DiffScope } from "@codedock/core/git";

export type PreviewTarget = {
  path: string;
  scope: DiffScope;
};

export function isPreviewablePath(path: string): boolean {
  return path !== "" && !path.endsWith("/");
}

export function scopeLabel(scope: DiffScope): string {
  return scope === "staged" ? "已暂存" : "当前目录";
}

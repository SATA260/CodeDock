import type { DirectoryListing } from "@codedock/views";

export async function listDirectories(path?: string): Promise<DirectoryListing> {
  const query = path?.trim() ? `?path=${encodeURIComponent(path.trim())}` : "";
  const res = await fetch(`/api/directories${query}`);
  const body = (await res.json().catch(() => ({}))) as DirectoryListing & { error?: string };
  if (!res.ok) {
    throw new Error(body.error || "无法列出目录");
  }
  return {
    path: body.path,
    parent: body.parent,
    entries: body.entries ?? [],
  };
}

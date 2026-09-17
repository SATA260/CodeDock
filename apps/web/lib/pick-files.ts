import type { PickedLocalFile } from "@codedock/views";

export async function pickFiles(options?: {
  images?: boolean;
  multiple?: boolean;
  start?: string;
}): Promise<PickedLocalFile[]> {
  const res = await fetch("/api/files/pick", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      images: options?.images,
      multiple: options?.multiple ?? true,
      start: options?.start,
    }),
  });
  const body = (await res.json().catch(() => ({}))) as { paths?: string[]; error?: string };
  if (!res.ok) {
    throw new Error(body.error || "无法打开系统文件选择框");
  }
  return (body.paths ?? []).map((path) => ({
    path,
    name: path.split(/[/\\]/).filter(Boolean).at(-1) || path,
  }));
}

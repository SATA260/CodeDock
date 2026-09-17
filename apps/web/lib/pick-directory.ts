export async function pickDirectory(options?: { start?: string }): Promise<string | undefined> {
  const res = await fetch("/api/directories/pick", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ start: options?.start }),
  });
  const body = (await res.json().catch(() => ({}))) as { path?: string; error?: string };
  if (!res.ok) {
    throw new Error(body.error || "无法打开系统目录选择框");
  }
  return body.path || undefined;
}

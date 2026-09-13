import { readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { NextResponse } from "next/server";

export async function GET(req: Request) {
  const raw = new URL(req.url).searchParams.get("path")?.trim() || homedir();
  const current = resolve(raw);
  try {
    const dirents = await readdir(current, { withFileTypes: true });
    const entries = dirents
      .filter((entry) => !entry.name.startsWith(".") && (entry.isDirectory() || entry.isSymbolicLink()))
      .map((entry) => ({ name: entry.name, path: join(current, entry.name) }))
      .sort((a, b) => a.name.localeCompare(b.name, "zh"));
    const parent = dirname(current);
    return NextResponse.json({
      path: current,
      parent: parent === current ? undefined : parent,
      entries,
    });
  } catch {
    return NextResponse.json({ error: "无法打开该目录" }, { status: 400 });
  }
}

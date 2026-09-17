import { NextResponse } from "next/server";

import { pickDirectoryNative } from "@/lib/native-file-pick";

export const maxDuration = 300;

export async function POST(req: Request) {
  const body = (await req.json().catch(() => ({}))) as { start?: string };
  try {
    const path = await pickDirectoryNative({ start: body.start });
    return NextResponse.json({ path });
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : "无法打开系统目录选择框" },
      { status: 500 },
    );
  }
}

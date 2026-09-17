import { NextResponse } from "next/server";

import { pickFilesNative } from "@/lib/native-file-pick";

export const maxDuration = 300;

export async function POST(req: Request) {
  const body = (await req.json().catch(() => ({}))) as {
    images?: boolean;
    multiple?: boolean;
    start?: string;
  };
  try {
    const paths = await pickFilesNative({
      images: Boolean(body.images),
      multiple: body.multiple !== false,
      start: body.start,
    });
    return NextResponse.json({ paths });
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : "无法打开系统文件选择框" },
      { status: 500 },
    );
  }
}

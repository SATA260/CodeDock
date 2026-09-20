import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const coreRoot = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(coreRoot, "../..");

/** 收集目录下源码文件的 import 语句。 */
async function collectImports(dir: string, ext = [".ts", ".tsx"]): Promise<string[]> {
  const out: string[] = [];
  const entries = await readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "node_modules" || entry.name === "dist") {
        continue;
      }
      out.push(...(await collectImports(full, ext)));
      continue;
    }
    if (!ext.some((item) => entry.name.endsWith(item))) {
      continue;
    }
    const text = await readFile(full, "utf8");
    for (const match of text.matchAll(/from\s+["']([^"']+)["']/g)) {
      if (match[1]) {
        out.push(match[1]);
      }
    }
  }
  return out;
}

test("frontend import direction stays layered", async () => {
  const coreImports = await collectImports(path.join(repoRoot, "packages/core"));
  for (const item of coreImports) {
    assert.equal(
      /^(react|next|@codedock\/ui|@codedock\/views)/.test(item),
      false,
      `packages/core 不可依赖 ${item}`,
    );
  }
  const uiImports = await collectImports(path.join(repoRoot, "packages/ui"));
  for (const item of uiImports) {
    assert.equal(
      /^(next|@codedock\/core|@codedock\/views)/.test(item),
      false,
      `packages/ui 不可依赖 ${item}`,
    );
  }
  const viewImports = await collectImports(path.join(repoRoot, "packages/views"));
  for (const item of viewImports) {
    assert.equal(item.startsWith("next/"), false, `packages/views 不可 import ${item}`);
  }
});

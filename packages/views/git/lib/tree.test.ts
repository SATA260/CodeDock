import assert from "node:assert/strict";
import { test } from "node:test";

import { isPreviewablePath } from "./preview.ts";
import { localNameFromRemote, splitWorkspaceFiles } from "./status.ts";
import { buildFileTree, collectFilePaths } from "./tree.ts";

function file(
  path: string,
  staged = " ",
  worktree = "M",
): {
  path: string;
  orig_path: string;
  staged_status: string;
  worktree_status: string;
  unmerged: boolean;
} {
  return {
    path,
    orig_path: "",
    staged_status: staged,
    worktree_status: worktree,
    unmerged: false,
  };
}

test("buildFileTree groups files under directories", () => {
  const tree = buildFileTree([
    file("apps/web/app/page.tsx"),
    file("apps/web/package.json"),
    file("README.md"),
  ]);
  assert.equal(tree.length, 2);
  assert.equal(tree[0]?.name, "apps");
  assert.equal(tree[0]?.children[0]?.name, "web");
  assert.deepEqual(
    tree[0]?.children[0]?.children.map((node) => node.name),
    ["app", "package.json"],
  );
  assert.equal(tree[1]?.name, "README.md");
  assert.equal(tree[1]?.file?.path, "README.md");
});

test("collectFilePaths walks a folder", () => {
  const tree = buildFileTree([file("a/b.txt"), file("a/c.txt")]);
  assert.deepEqual(collectFilePaths(tree[0]!), ["a/b.txt", "a/c.txt"]);
});

test("splitWorkspaceFiles separates staged and worktree", () => {
  const split = splitWorkspaceFiles([
    file("staged.ts", "M", " "),
    file("dirty.ts", " ", "M"),
    file("both.ts", "M", "M"),
    { ...file("conflict.ts", "U", "U"), unmerged: true },
  ]);
  assert.deepEqual(
    split.staged.map((item) => item.path),
    ["staged.ts", "both.ts"],
  );
  assert.deepEqual(
    split.worktree.map((item) => item.path),
    ["dirty.ts", "both.ts", "conflict.ts"],
  );
});

test("buildFileTree treats trailing-slash paths as folders", () => {
  const tree = buildFileTree([file("apps/web/app/git/")]);
  const git = tree[0]?.children[0]?.children[0]?.children[0];
  assert.equal(git?.name, "git");
  assert.equal(git?.file, undefined);
  assert.deepEqual(git?.children, []);
});

test("buildFileTree nests untracked files under folders", () => {
  const tree = buildFileTree([
    file("packages/views/git/file-tree.tsx", " ", "?"),
    file("packages/views/git/git-page.tsx", " ", "?"),
  ]);
  const git = tree[0]?.children[0]?.children[0];
  assert.equal(git?.name, "git");
  assert.equal(git?.file, undefined);
  assert.deepEqual(
    git?.children.map((node) => node.name),
    ["file-tree.tsx", "git-page.tsx"],
  );
});

test("localNameFromRemote strips the remote prefix", () => {
  assert.equal(localNameFromRemote("origin/feat/git"), "feat/git");
  assert.equal(localNameFromRemote("main"), "main");
});

test("isPreviewablePath skips folders", () => {
  assert.equal(isPreviewablePath("apps/web/app/page.tsx"), true);
  assert.equal(isPreviewablePath("apps/web/app/git/"), false);
  assert.equal(isPreviewablePath(""), false);
});

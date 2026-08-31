#!/bin/sh
# 从仓库根启动 API。未设 GIT_REPO 时只用 tmp/git-sandbox，没有就先初始化。
# 撤回会改磁盘，不要默认对着 CodeDock 工作区。要操作本仓：GIT_REPO=$root pnpm dev:api
set -e
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
sandbox="$root/tmp/git-sandbox"
if [ -z "$GIT_REPO" ]; then
  if [ ! -d "$sandbox/.git" ]; then
    sh "$root/scripts/git-sandbox.sh" >/dev/null
  fi
  GIT_REPO="$sandbox"
fi
export GIT_REPO
cd "$root/server"
exec go run ./cmd/server

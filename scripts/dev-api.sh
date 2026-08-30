#!/bin/sh
# 从仓库根启动 API。未设 GIT_REPO 时：有 tmp/git-sandbox 用沙箱，否则才指到本仓。
# 撤回会改磁盘，不要默认对着 CodeDock 工作区试。要操作本仓：GIT_REPO=$root pnpm dev:api
set -e
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
sandbox="$root/tmp/git-sandbox"
if [ -z "$GIT_REPO" ]; then
  if [ -d "$sandbox/.git" ]; then
    GIT_REPO="$sandbox"
  else
    GIT_REPO="$root"
  fi
fi
export GIT_REPO
cd "$root/server"
exec go run ./cmd/server

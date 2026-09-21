#!/bin/sh
# 为 Playwright 拉起独立 API：独立库与 Git 沙箱，模型仍读仓库根 .env。
set -e
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
dir="$root/tmp/e2e"
mkdir -p "$dir/git"
if [ ! -d "$dir/git/.git" ]; then
  git -C "$dir/git" init -b main >/dev/null
  git -C "$dir/git" config user.email tester@example.com
  git -C "$dir/git" config user.name tester
  git -C "$dir/git" commit --allow-empty -m init >/dev/null
fi
export HTTP_ADDR="${HTTP_ADDR:-:18080}"
export DB_DSN="${DB_DSN:-file:$dir/codedock.db}"
export GIT_REPO="${GIT_REPO:-$dir/git}"
cd "$root/server"
exec go run ./cmd/server

#!/bin/sh
# 同时起 API 与 Web，供本机把前后端调通。
set -e
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root"
sandbox="$root/tmp/git-sandbox"
if [ -z "$GIT_REPO" ]; then
  if [ -d "$sandbox/.git" ]; then
    GIT_REPO="$sandbox"
  else
    GIT_REPO="$root"
  fi
fi
export GIT_REPO
(cd "$root/server" && go run ./cmd/server) &
api=$!
trap 'kill "$api" 2>/dev/null || true' EXIT INT TERM
pnpm --filter web dev

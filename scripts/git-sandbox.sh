#!/bin/sh
# 造一个可随意暂存 / 撤回的独立 Git 仓。不要对着 CodeDock 本仓试破坏性操作。
set -e
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
dir="$root/tmp/git-sandbox"
rm -rf "$dir"
mkdir -p "$dir/src" "$dir/notes"
cd "$dir"
git init -b main >/dev/null
git config user.name tester
git config user.email tester@example.com
printf 'hello sandbox\n' > README.txt
printf 'version 1\n' > src/app.txt
git add README.txt src/app.txt
git commit -m "init sandbox" >/dev/null
origin="$root/tmp/git-sandbox-origin.git"
rm -rf "$origin"
git init --bare "$origin" >/dev/null
git remote add origin "$origin"
git push -u origin main >/dev/null
printf 'version 2 (dirty)\n' > src/app.txt
printf 'scratch\n' > notes/todo.txt
printf 'untracked\n' > new.txt
echo "$dir"

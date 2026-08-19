#!/usr/bin/env bash
# Switch to main and fast-forward it from the remote.
# Refuses to run with a dirty working tree so nothing local is silently lost.
set -euo pipefail
cd "$(dirname "$0")"

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "sync-main: working tree has uncommitted changes; commit or stash first." >&2
  exit 1
fi

git checkout main
git pull --ff-only

#!/usr/bin/env bash
# PreToolUse guard for the path-carrying write tools: Edit|Write|MultiEdit|
# NotebookEdit plus the IDE MCP file tools, which name their path under other
# keys (filePath / path / pathInProject / …).
# Blocks edits when the target file's git repo is on main/master, forcing a
# feature branch to be created first. Reads the hook JSON on stdin.
set -uo pipefail

project="${CLAUDE_PROJECT_DIR:-/Volumes/Lynx/Development/GO/authcore}"

input=$(cat)
file_path=$(printf '%s' "$input" | jq -r '
  .tool_input.file_path // .tool_input.filePath // .tool_input.path //
  .tool_input.pathInProject // .tool_input.filePathInProject //
  .tool_input.notebook_path // .tool_input.relativePath // empty')

# No recognizable path on the tool call. For a tool whose whole job is writing,
# staying silent would be the same gap the shell guard exists to close, so fall
# back to the project repository itself.
if [ -z "$file_path" ]; then
  b=$(git -C "$project" branch --show-current 2>/dev/null) || exit 0
  case "$b" in
    main | master)
      echo "BLOCKED: '$project' is on '$b' and this tool call carries no readable path. Open a feature branch first: git -C '$project' checkout -b feature/<kebab-outcome>." >&2
      exit 2
      ;;
  esac
  exit 0
fi

# An IDE MCP tool may hand a path relative to the project root.
case "$file_path" in
  /*) ;;
  *) file_path="$project/$file_path" ;;
esac

# Resolve a real directory to query git from. On a Write the file may not exist
# yet, and its parent dir may be new too — walk up to the nearest existing dir.
dir=$(dirname "$file_path")
while [ ! -d "$dir" ] && [ "$dir" != "/" ] && [ -n "$dir" ]; do
  dir=$(dirname "$dir")
done

# Outside any git repo → allow (e.g. a scratchpad file).
repo=$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null) || exit 0

branch=$(git -C "$repo" branch --show-current 2>/dev/null) || exit 0

case "$branch" in
  main|master)
    echo "BLOCKED: '$repo' is on '$branch'. Per project rule, open a feature branch BEFORE editing — e.g. git -C '$repo' checkout -b docs/<kebab-outcome> (prefix: feature|fix|docs|refactor). Then retry the edit." >&2
    exit 2
    ;;
esac
exit 0

#!/usr/bin/env bash
# PreToolUse guard for Bash and the IDE MCP terminal tools.
#
# The path-based guard (guard-branch.sh) only sees Edit|Write|MultiEdit. A shell
# command that rewrites a file — sed -i, a heredoc, python, a redirect — reaches
# the same repository without ever touching those tools, which is exactly how a
# whole session once landed on main. This closes that path.
#
# Reads the hook JSON on stdin. Allows anything that does not look like a write,
# so reads (git status, go test, grep, cat) stay unblocked, and always allows the
# branch commands themselves — otherwise being on main would be a deadlock.
set -uo pipefail

project="${CLAUDE_PROJECT_DIR:-/Volumes/Lynx/Development/GO/authcore}"

input=$(cat)
cmd=$(printf '%s' "$input" | jq -r '.tool_input.command // empty')
[ -z "$cmd" ] && exit 0

# 1. The escape hatch, always allowed: creating or renaming a branch is how the
#    user gets OUT of the blocked state.
if printf '%s' "$cmd" | grep -qE 'git ([^|;&]* )?(checkout -b|switch -c|branch -m)'; then
  exit 0
fi

# 2. Does this command write to a file? Anything not on this list is a read as
#    far as this guard is concerned.
writes_re='(sed +(-i|--in-place))|(^|[^0-9&])>>?[[:space:]]*[^&|]|(^|[[:space:]])tee[[:space:]]|(^|[[:space:]])(cp|mv|rm|mkdir|touch|install|truncate|dd|ln)[[:space:]]|(python3?|perl|ruby|node)[[:space:]]|(gofmt|goimports)[[:space:]]+-w|go[[:space:]]+(mod[[:space:]]+tidy|generate|fmt)|git[[:space:]]([^|;&]*[[:space:]])?(apply|am|restore|stash|reset|revert|clean|rm|mv|add|commit)([[:space:]]|$)|(^|[[:space:]])patch[[:space:]]|<<'
printf '%s' "$cmd" | grep -qE "$writes_re" || exit 0

# 3. Writes aimed at scratch space are never a repository edit.
if printf '%s' "$cmd" | grep -qE '(/private)?/tmp/|/scratchpad/|/dev/null'; then
  # Only bail out when the command touches NOTHING else — a command that writes
  # to both a repo file and /tmp still has to be checked.
  printf '%s' "$cmd" | grep -qE '\.(go|md|html|json|ya?ml|sql|sh|ts|js|tsx|jsx)([^a-zA-Z0-9]|$)' || exit 0
fi

# 4. Which repository would this touch? Whoever takes the write is who judges
#    it, so the working directory's repository decides — never this project's
#    branch standing in for someone else's.
blocked=""

check_repo() {
  local repo="$1"
  local branch
  branch=$(git -C "$repo" branch --show-current 2>/dev/null) || return 0
  case "$branch" in
    main | master) blocked="$repo ($branch)" ;;
  esac
}

if repo=$(git -C "$PWD" rev-parse --show-toplevel 2>/dev/null); then
  # Inside a repository — that one answers for the write, even when it is not
  # this project. Consulting the project's branch here would block edits the
  # other repository has every right to accept.
  check_repo "$repo"
else
  # Outside every repository. An ambiguous write ("git commit -m x") lands
  # wherever the shell wanders, so judge it by the project's own branch.
  project_repo=$(git -C "$project" rev-parse --show-toplevel 2>/dev/null) && check_repo "$project_repo"
fi

[ -z "$blocked" ] && exit 0

cat >&2 <<EOF
BLOCKED: this command writes files and $blocked is on a protected branch.
Per project rule, open a feature branch BEFORE editing:
  git -C '${blocked%% *}' checkout -b feature/<kebab-outcome>   (prefix: feature|fix|docs|refactor)
Then retry. Reads are never blocked; only file-writing commands are.
EOF
exit 2

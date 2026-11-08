#!/bin/sh
# Validates that the commit message header follows Conventional Commits:
#   <type>[(scope)][!]: <description>
# Usage: check-commit-msg.sh <commit-msg-file>

set -eu

msg_file="${1:-}"
if [ -z "$msg_file" ] || [ ! -f "$msg_file" ]; then
	echo "usage: $0 <commit-msg-file>" >&2
	exit 2
fi

header=$(sed -n '/^[[:space:]]*#/d;/^[[:space:]]*$/d;p;q' "$msg_file")

# Let git-generated and rebase-helper headers through.
case "$header" in
	"Merge "* | 'Revert "'* | "fixup!"* | "squash!"* | "amend!"*)
		exit 0
		;;
esac

pattern='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^)]+\))?!?: .+'
if printf '%s\n' "$header" | grep -Eq "$pattern"; then
	exit 0
fi

cat >&2 <<MSG
Invalid commit message header:
  $header

Expected Conventional Commits format:
  <type>[(scope)][!]: <description>

Allowed types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
MSG
exit 1

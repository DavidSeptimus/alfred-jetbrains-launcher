#!/usr/bin/env bash
# Validate a single commit message against Conventional Commits.
#
#   commit-msg-lint.sh <path-to-message-file>   # used by the commit-msg hook
#   ... | commit-msg-lint.sh -                   # read the message from stdin (CI)
#
# Exits 0 when the header line conforms, 1 otherwise. Shared by .githooks/commit-msg
# and the commit-lint CI workflow so the rule lives in exactly one place.
set -euo pipefail

if [ "${1:-}" = "-" ] || [ -z "${1:-}" ]; then
  msg="$(cat)"
else
  msg="$(cat "$1")"
fi

# The header is the first non-empty, non-comment line.
header="$(printf '%s\n' "$msg" | grep -vE '^[[:space:]]*#' | sed '/^[[:space:]]*$/d' | head -1)"

# Allow git's own generated subjects (merges, reverts, autosquash markers).
case "$header" in
  "Merge "*|"Revert "*|"fixup! "*|"squash! "*|"amend! "*) exit 0 ;;
esac

types='feat|fix|perf|refactor|docs|style|test|build|ci|chore|revert'
# <type>(<optional scope>)<optional !>: <subject>
pattern="^(${types})(\([a-zA-Z0-9 ._/-]+\))?!?: .+"

if printf '%s' "$header" | grep -qE "$pattern"; then
  exit 0
fi

cat >&2 <<EOF
✖ Not a Conventional Commit:

    ${header:-(empty)}

Expected:  <type>[optional scope][!]: <subject>
Types:     feat, fix, perf, refactor, docs, style, test, build, ci, chore, revert
Examples:  feat: add a custom open command
           fix(search): honour a renamed keyword
           refactor!: drop the legacy jbup keyword   (! marks a breaking change)

See https://www.conventionalcommits.org
EOF
exit 1

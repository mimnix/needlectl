#!/usr/bin/env bash
#
# Composes the Markdown body of a GitHub release.
#
# Usage:
#   scripts/release/notes.sh <tag> [> RELEASE_NOTES.md]
#
# The body is the curated docs/releases/<tag>.md plus installation instructions
# and the matching section of CHANGELOG.md. It fails when either file is missing
# or when the changelog section is empty, so a release can never be published
# without notes.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

tag="${1:?usage: notes.sh <tag>}"

if [[ ! "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "notes.sh: '$tag' is not a vX.Y.Z tag" >&2
  exit 1
fi
version="${tag#v}"

notes="docs/releases/${tag}.md"
changelog="CHANGELOG.md"

if [ ! -f "$notes" ]; then
  echo "notes.sh: missing release notes file: $notes" >&2
  exit 1
fi
if [ ! -f "$changelog" ]; then
  echo "notes.sh: missing $changelog" >&2
  exit 1
fi

section="$(
  awk -v heading="## [${version}]" '
    index($0, heading) == 1 { found = 1; next }
    found && /^## \[/ { exit }
    found { print }
  ' "$changelog" | sed '/./,$!d'
)"

if [ -z "$(printf '%s' "$section" | tr -d '[:space:]')" ]; then
  echo "notes.sh: CHANGELOG.md has no section for ${version}" >&2
  exit 1
fi

cat "$notes"
cat <<EOF

## Installation

Download the archive for your platform from the assets below, then verify it
with \`sha256sum -c SHA256SUMS\` on Linux/macOS or \`Get-FileHash\` on Windows.

Or install it with Go:

\`\`\`bash
go install github.com/mimnix/needlectl@${tag}
\`\`\`

## Changelog

${section}
EOF

#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/cleanup.sh <relative-path> [...]

Removes test artifacts relative to the wt repo root. Paths must stay inside
the repository to avoid accidental deletions elsewhere.
EOF
}

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

for relpath in "$@"; do
  if [[ "$relpath" = /* ]]; then
    echo "cleanup: absolute paths not allowed ($relpath)" >&2
    exit 1
  fi
  target="${repo_root}/${relpath}"
  if [[ "$target" != "${repo_root}/"* ]]; then
    echo "cleanup: refusing to remove outside repo ($relpath)" >&2
    exit 1
  fi
  rm -rf "$target"
done

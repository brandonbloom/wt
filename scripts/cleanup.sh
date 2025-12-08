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
transcripts_root="/tmp/wt-transcripts"

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

  if [[ "$relpath" == tmprepo || "$relpath" == tmprepo/* ]]; then
    if [[ "$relpath" == *".."* ]]; then
      echo "cleanup: '..' not allowed in path ($relpath)" >&2
      exit 1
    fi
    suffix="${relpath#tmprepo}"
    tmp_target="${transcripts_root}/tmprepo${suffix}"
    case "$tmp_target" in
      "${transcripts_root}"|${transcripts_root}/*)
        rm -rf "$tmp_target"
        ;;
      *)
        echo "cleanup: refusing to remove outside transcript root ($relpath)" >&2
        exit 1
        ;;
    esac
  fi
done

#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: transcripts/entrypoint.sh [options] -- <command> [args...]

Sets up a disposable wt test project, runs the given command inside it,
and cleans up afterward. Intended for transcript integration tests.

Options:
  --skip-init          Leave the repository unconverted (for wt init tests).
  --activate-wrapper   Simulate the wt shell wrapper being active.
  --worktree DIR       cd into DIR (relative to the temp repo) before running.
  --keep               Preserve the temp repo for debugging (prints its path).
EOF
}

run_init=1
activate_wrapper=0
worktree=""
keep_repo=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-init)
      run_init=0
      shift
      ;;
    --activate-wrapper)
      activate_wrapper=1
      shift
      ;;
    --worktree)
      worktree="$2"
      shift 2
      ;;
    --keep)
      keep_repo=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
    *)
      break
      ;;
  esac
done

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmprepo="${repo_root}/tmprepo"

cleanup() {
  if [[ $keep_repo -eq 1 ]]; then
    echo "temp repo kept at ${tmprepo}" >&2
    return
  fi
  rm -rf "${tmprepo}"
}
trap cleanup EXIT

rm -rf "${tmprepo}"
mkdir -p "${tmprepo}"
cd "${tmprepo}"

export GIT_AUTHOR_NAME=wt-test
export GIT_AUTHOR_EMAIL=wt@example.com
export GIT_COMMITTER_NAME=wt-test
export GIT_COMMITTER_EMAIL=wt@example.com
export GIT_AUTHOR_DATE='2000-01-01T00:00:00Z'
export GIT_COMMITTER_DATE='2000-01-01T00:00:00Z'

git init -b main >/dev/null
echo hi >README.md
git add README.md
git commit -m init >/dev/null

if [[ $run_init -eq 1 ]]; then
  "${repo_root}/bin/wt" init >/dev/null
  cd "${tmprepo}"
fi

mkdir -p bin
cat >bin/gh <<'EOF'
#!/bin/sh
set -eu

if [ "$#" -lt 1 ]; then
  echo "gh stub: missing subcommand" >&2
  exit 1
fi

sub="$1"
shift

case "$sub" in
  auth)
    if [ "${1:-}" = "status" ]; then
      exit 0
    fi
    ;;
  repo)
    if [ "${1:-}" = "view" ]; then
      echo main
      exit 0
    fi
    ;;
esac

echo "gh stub cannot handle: $sub $*" >&2
exit 1
EOF
chmod +x bin/gh
export PATH="${tmprepo}/bin:${PATH}"

if [[ $activate_wrapper -eq 1 ]]; then
  export WT_WRAPPER_ACTIVE=1
  export WT_INSTRUCTION_FILE="${tmprepo}/.wt-instruction"
fi

if [[ -n "$worktree" ]]; then
  cd "$worktree"
fi

"$@"

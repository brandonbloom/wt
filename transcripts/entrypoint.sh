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

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
transcripts_root="/tmp/wt-transcripts"
lock_dir="/tmp/wt-transcripts.lock"

mkdir -p "${transcripts_root}"
ln -snf "${repo_root}/bin" "${transcripts_root}/bin"
while ! mkdir "${lock_dir}" 2>/dev/null; do
  sleep 0.1
done

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

tmprepo_rel="tmprepo"
tmprepo="${transcripts_root}/${tmprepo_rel}"

cleanup() {
  if [[ $keep_repo -eq 1 ]]; then
    echo "temp repo kept at ${tmprepo}" >&2
  else
    "${repo_root}/scripts/cleanup.sh" "${tmprepo_rel}"
  fi
  rmdir "${lock_dir}"
}
trap cleanup EXIT

"${repo_root}/scripts/cleanup.sh" "${tmprepo_rel}"
mkdir -p "${tmprepo}"
cd "${tmprepo}"

export GIT_AUTHOR_NAME=wt-test
export GIT_AUTHOR_EMAIL=wt@example.com
export GIT_COMMITTER_NAME=wt-test
export GIT_COMMITTER_EMAIL=wt@example.com
export GIT_AUTHOR_DATE='2000-01-01T00:00:00Z'
export GIT_COMMITTER_DATE='2000-01-01T00:00:00Z'
export NO_COLOR=1
export CLICOLOR=0
export CLICOLOR_FORCE=0

git init -b main >/dev/null
echo hi >README.md
git add README.md
git commit -m init >/dev/null
gh_state_file="${tmprepo}/.gh-prs"

if [[ $run_init -eq 1 ]]; then
  "${repo_root}/bin/wt" init >/dev/null
  cd "${tmprepo}"
fi

cat >"${gh_state_file}" <<'EOF'
demo-branch|42|OPEN|false|2000-01-02T00:00:00Z|https://example.com/pr/42
merged-branch|99|MERGED|false|2000-01-02T00:00:00Z|https://example.com/pr/99
EOF

# Install a hermetic gh stub so transcript runs never hit real GitHub.
mkdir -p bin
cat >bin/gh <<'EOF'
#!/bin/sh
set -eu

STATE_FILE="${WT_GH_STATE_FILE:-${PWD}/.gh-prs}"

pr_list() {
  branch=""
  limit=0
  while [ $# -gt 0 ]; do
    case "$1" in
      --head)
        branch="$2"
        shift 2
        ;;
      --limit)
        limit="$2"
        shift 2
        ;;
      --state|--json)
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  echo '['
  first=1
  count=0
  if [ -f "$STATE_FILE" ]; then
    while IFS='|' read -r pr_branch pr_number pr_state pr_draft pr_updated pr_url; do
      [ -z "$pr_branch" ] && continue
      if [ -n "$branch" ] && [ "$branch" != "$pr_branch" ]; then
        continue
      fi
      count=$((count + 1))
      if [ "$limit" -gt 0 ] && [ "$count" -gt "$limit" ]; then
        break
      fi
      if [ "$first" -eq 0 ]; then
        printf ','
      fi
      first=0
      printf '{"number":%s,"state":"%s","isDraft":%s,"updatedAt":"%s","url":"%s"}' \
        "$pr_number" "$pr_state" "$pr_draft" "$pr_updated" "$pr_url"
    done <"$STATE_FILE"
  fi
  echo ']'
}

pr_close() {
  if [ "$#" -lt 1 ]; then
    echo "gh stub: pr close requires a number" >&2
    exit 1
  fi
  number="$1"
  shift
  found=0
  tmp="$(mktemp "${STATE_FILE}.XXXXXX")"
  if [ -f "$STATE_FILE" ]; then
    while IFS='|' read -r pr_branch pr_number pr_state pr_draft pr_updated pr_url; do
      [ -z "$pr_branch" ] && continue
      if [ "$pr_number" = "$number" ] && [ "$found" -eq 0 ]; then
        pr_state="CLOSED"
        found=1
      fi
      echo "${pr_branch}|${pr_number}|${pr_state}|${pr_draft}|${pr_updated}|${pr_url}" >>"$tmp"
    done <"$STATE_FILE"
  fi
  if [ "$found" -eq 0 ]; then
    rm -f "$tmp"
    echo "gh stub: pull request #${number} not found" >&2
    exit 1
  fi
  mv "$tmp" "$STATE_FILE"
  echo "Closed PR #${number}"
}

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
  pr)
    if [ "${1:-}" = "list" ]; then
      shift
      if [ -n "${WT_TEST_GH_DELAY:-}" ]; then
        sleep "${WT_TEST_GH_DELAY}"
      fi
      pr_list "$@"
      exit 0
    fi
    if [ "${1:-}" = "close" ]; then
      shift
      pr_close "$@"
      exit 0
    fi
    ;;
esac

echo "gh stub cannot handle: $sub $*" >&2
exit 1
EOF
chmod +x bin/gh
export PATH="${tmprepo}/bin:${PATH}"
export WT_GH_STATE_FILE="${gh_state_file}"

if [[ $activate_wrapper -eq 1 ]]; then
  export WT_WRAPPER_ACTIVE=1
  export WT_INSTRUCTION_FILE="${tmprepo}/.wt-instruction"
fi

if [[ -n "$worktree" ]]; then
  cd "$worktree"
fi

"$@"

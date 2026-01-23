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
if [[ "${WT_SKIP_DEFAULT_ORIGIN:-0}" != "1" ]]; then
  git remote add origin git@github.com:brandonbloom/wt.git >/dev/null
fi
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
ci_state_file="${tmprepo}/.gh-ci"
cat >"${ci_state_file}" <<'EOF'
commit|*|build|completed|success|https://example.com/run/success|2000-01-02T00:00:00Z|2000-01-02T00:05:00Z
pr|42|Pull Request Checks|completed|failure|https://example.com/run/pr-42|2000-01-02T23:59:00Z|2000-01-02T23:59:59Z
EOF

# Install a hermetic gh stub so transcript runs never hit real GitHub.
mkdir -p bin
cat >bin/gh <<'EOF'
#!/bin/sh
set -eu

STATE_FILE="${WT_GH_STATE_FILE:-${PWD}/.gh-prs}"
CI_FILE="${WT_GH_CI_FILE:-${PWD}/.gh-ci}"

url_decode() {
  printf '%b' "${1//%/\\x}"
}

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

emit_check_runs() {
  key="$1"
  fallback=""
  case "$key" in
    commit\|*)
      fallback="commit|*"
      ;;
    pr\|*)
      fallback="pr|*"
      ;;
  esac
  if [ -n "${WT_GH_DEBUG:-}" ]; then
    echo "ci_key=$key" >>"${CI_FILE}.log"
  fi

  if [ -f "$CI_FILE" ]; then
    while IFS='|' read -r entry_type entry_id name status conclusion url started completed; do
      [ -z "$entry_type" ] && continue
      entry="${entry_type}|${entry_id}"
      if [ "$entry" = "$key" ] || { [ -n "$fallback" ] && [ "$entry" = "$fallback" ]; }; then
        printf '{"total_count":1,"check_runs":[{"name":"%s","status":"%s","conclusion":"%s","html_url":"%s","details_url":"%s","started_at":"%s","completed_at":"%s"}]}\n' \
          "$name" "$status" "$conclusion" "$url" "$url" "$started" "$completed"
        return 0
      fi
    done <"$CI_FILE"
  fi
  echo '{"total_count":0,"check_runs":[]}'
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
  api)
    if [ "$#" -lt 1 ]; then
      echo "gh stub: api requires a path" >&2
      exit 1
    fi
    endpoint="$1"
    shift
    base="${endpoint%%\?*}"
    case "$base" in
      graphql)
        tmp="$(mktemp "${STATE_FILE}.graphql.XXXXXX")"
        while [ $# -gt 0 ]; do
          case "$1" in
            -f|-F)
              kv="${2:-}"
              shift 2
              key="${kv%%=*}"
              val="${kv#*=}"
              case "$key" in
                b[0-9]*)
                  idx="${key#b}"
                  echo "${idx}|${val}" >>"$tmp"
                  ;;
              esac
              ;;
            *)
              shift
              ;;
          esac
        done

        echo '{"data":{"repository":{'
        first_alias=1
        if [ -f "$tmp" ]; then
          sort -t'|' -k1,1n "$tmp" | while IFS='|' read -r idx branch; do
            [ -z "$idx" ] && continue
            alias="pr${idx}"
            if [ "$first_alias" -eq 0 ]; then
              printf ','
            fi
            first_alias=0
            printf '"%s":{"nodes":[' "$alias"

            first_node=1
            count=0
            if [ -f "$STATE_FILE" ]; then
              while IFS='|' read -r pr_branch pr_number pr_state pr_draft pr_updated pr_url; do
                [ -z "$pr_branch" ] && continue
                if [ "$pr_branch" != "$branch" ]; then
                  continue
                fi
                count=$((count + 1))
                if [ "$count" -gt 5 ]; then
                  break
                fi
                if [ "$first_node" -eq 0 ]; then
                  printf ','
                fi
                first_node=0
                printf '{"number":%s,"state":"%s","isDraft":%s,"updatedAt":"%s","url":"%s","headRefName":"%s"}' \
                  "$pr_number" "$pr_state" "$pr_draft" "$pr_updated" "$pr_url" "$pr_branch"
              done <"$STATE_FILE"
            fi

            printf ']}'
          done
        fi
        echo '}}}'
        rm -f "$tmp"
        exit 0
        ;;
      repos/*/*/commits/*/check-runs)
        ref_with_tail="${base#repos/}"
        ref_with_tail="${ref_with_tail#*/}"
        ref_with_tail="${ref_with_tail#*/}"
        ref_with_tail="${ref_with_tail#commits/}"
        ref="${ref_with_tail%/check-runs}"
        decoded_ref="$(url_decode "$ref")"
        if [ -n "${WT_GH_DEBUG:-}" ]; then
          echo "decoded_ref=$decoded_ref" >>"${CI_FILE}.log"
        fi
        case "$decoded_ref" in
          refs/pull/*/merge)
            pr_num="${decoded_ref#refs/pull/}"
            pr_num="${pr_num%/merge}"
            if [ -n "${WT_GH_DEBUG:-}" ]; then
              echo "pr_num=$pr_num" >>"${CI_FILE}.log"
            fi
            emit_check_runs "pr|${pr_num}"
            exit 0
            ;;
          *)
            emit_check_runs "commit|${decoded_ref}"
            exit 0
            ;;
        esac
        ;;
      repos/*/*/actions/runs*)
        echo '{"total_count":0,"workflow_runs":[]}'
        exit 0
        ;;
    esac
    ;;
  run)
    if [ "${1:-}" = "list" ]; then
      shift
      echo '[]'
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
export WT_GH_CI_FILE="${ci_state_file}"

if [[ $activate_wrapper -eq 1 ]]; then
  export WT_WRAPPER_ACTIVE=1
  export WT_INSTRUCTION_FILE="${tmprepo}/.wt-instruction"
fi

if [[ -n "$worktree" ]]; then
  cd "$worktree"
fi

"$@"

#!/usr/bin/env bash
# Prepare a public DataDog/datadog-agent branch from an internal ddoghq/datadog-agent branch.
#
# Rebuilds the branch's own commits on public main in a new worktree, then checks that nothing
# internal goes along: internal-only commits and files, ddoghq references, private SHAs.
# It never pushes: pushing to the public repository discloses the change, so a human does it.
#
# Runs from a clone of either repository. Prefer a public clone: commits are then signed, and later
# pushed, with your public GitHub identity. Fetch the private branch into it first:
#   git fetch https://github.com/ddoghq/datadog-agent.git BRANCH:refs/private/BRANCH
#   export-to-public.sh refs/private/BRANCH
#
# Usage:
#   export-to-public.sh [options] BRANCH  Cherry-pick the commits of BRANCH that are not in internal
#                                         main (merges excluded) onto public main, then run the checks.
#   export-to-public.sh --check [--workdir DIR] [--allow-path GLOB]...
#                                         Re-run the checks in DIR, e.g. after resolving a conflict
#                                         (git cherry-pick --continue) or rewording a commit.
#
# Options:
#   --workdir DIR     worktree to create, or to check (default: a new temporary directory)
#   --name NAME       name of the public branch (default: BRANCH, without refs/heads/ or refs/private/)
#   --allow-path GLB  skip the internal-path and content checks for files matching GLB, once a human
#                     has reviewed them (repeatable)
#
# Environment:
#   SOURCE_URL       public repository (default: https://github.com/DataDog/datadog-agent.git)
#   DESTINATION_URL  internal repository (default: the clone's origin URL when it points to
#                    ddoghq/datadog-agent, else https://github.com/ddoghq/datadog-agent.git)
#
# Fetched refs are stored under refs/export-to-public/ so that the user's remotes are never touched.

main() {
  set -euo pipefail
  SCRIPT_PATH=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")

  local mode=prepare workdir='' name='' branch=''
  ALLOWED=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --check) mode=check ;;
      --workdir) workdir=$2; shift ;;
      --name) name=$2; shift ;;
      --allow-path) ALLOWED+=("$2"); shift ;;
      -h|--help) sed -n '2,32p' "$SCRIPT_PATH" | sed 's/^# \{0,1\}//'; return 0 ;;
      -*) echo "Unknown argument: $1" >&2; return 2 ;;
      *) [[ -z "$branch" ]] || { echo "Only one branch can be exported." >&2; return 2; }; branch=$1 ;;
    esac
    shift
  done

  if [[ "$mode" == check ]]; then
    [[ -n "$workdir" ]] && cd "$workdir"
    check
    return
  fi
  [[ -n "$branch" ]] || { echo "Usage: $(basename "$SCRIPT_PATH") [options] BRANCH (see --help)" >&2; return 2; }
  git rev-parse --verify -q --end-of-options "$branch^{commit}" >/dev/null || { echo "Unknown branch: $branch" >&2; return 2; }
  if [[ -z "$name" ]]; then
    name=${branch#refs/heads/}
    name=${name#refs/private/}
  fi

  SOURCE_URL=${SOURCE_URL:-https://github.com/DataDog/datadog-agent.git}
  if [[ -z "${DESTINATION_URL:-}" ]]; then
    DESTINATION_URL=$(git remote get-url origin 2>/dev/null | grep 'ddoghq/datadog-agent' || true)
  fi
  DESTINATION_URL=${DESTINATION_URL:-https://github.com/ddoghq/datadog-agent.git}
  readonly ORIGIN_REF=refs/export-to-public/origin UPSTREAM_REF=refs/export-to-public/upstream

  git fetch --quiet --no-tags "$DESTINATION_URL" "+refs/heads/main:$ORIGIN_REF"
  git fetch --quiet --no-tags "$SOURCE_URL" "+refs/heads/main:$UPSTREAM_REF"

  local upstream commits
  upstream=$(git rev-parse "$UPSTREAM_REF")
  # The branch's own work: not in internal main (internal CI commits, sync merges) nor already public.
  commits=$(git rev-list --reverse --no-merges "$branch" "^$ORIGIN_REF" "^$UPSTREAM_REF" --)
  if [[ -z "$commits" ]]; then
    echo "$branch has no commit that is not already in internal or public main." >&2
    return 1
  fi
  echo "Commits to export:" >&2
  # shellcheck disable=SC2086 # one SHA per word
  git --no-pager log --no-walk=unsorted --format='  %h %s' $commits >&2

  workdir=${workdir:-$(mktemp -d "${TMPDIR:-/tmp}/export-to-public.XXXXXX")}
  rmdir "$workdir" 2>/dev/null || true
  git worktree add --quiet -b "export-to-public/$name" "$workdir" "$upstream"
  cd "$workdir"
  echo "Working in worktree $workdir (remove it with: git worktree remove $workdir && git branch -D export-to-public/$name)" >&2
  save_state "$upstream" "$(git rev-parse "$ORIGIN_REF")" "$name"

  # No -x: "(cherry picked from commit <sha>)" would publish internal commit IDs.
  # --empty=drop (git 2.45+) skips commits whose change already reached public main under another SHA.
  # shellcheck disable=SC2086
  if ! git -c core.hooksPath=/dev/null cherry-pick --empty=drop $commits; then
    cat >&2 <<EOF

Cherry-pick stopped on a conflict in $workdir. The change probably touches a file that internal
commits modified (CI identity, CODEOWNERS): resolve it towards the public version, then:
  git add <files> && git cherry-pick --continue
  $SCRIPT_PATH --check --workdir $workdir

To give up: git cherry-pick --abort && cd - && git worktree remove $workdir && git branch -D export-to-public/$name
EOF
    return 1
  fi
  check
}

# Files that only make sense internally. A public PR may legitimately touch some of them
# (CODEOWNERS for instance): --allow-path lets a reviewed change through.
readonly INTERNAL_PATHS=(
  '.github/chainguard/*'
  '.github/CODEOWNERS'
  '.github/workflows/sync-public-main.yml'
  'tools/ci/sync-public-main.sh'
  'tools/ci/tests/test_sync_public_main.py'
  'doc/how-to/internal-repository-*'
)
# Text that points to the private repository or to internal accounts (14012: internal GitLab project ID).
readonly INTERNAL_TEXT='ddoghq|[A-Za-z0-9-]_ddog([^A-Za-z0-9]|$)|datadog-agent-internal|cherry picked from commit|(^|[^0-9])14012([^0-9]|$)'

check() {
  local problems=0 upstream origin file pattern hits excludes=()
  if git rev-parse --verify -q CHERRY_PICK_HEAD >/dev/null; then
    echo 'A cherry-pick is still in progress: finish it with git cherry-pick --continue first.' >&2
    return 1
  fi
  [[ -z "$(git status --porcelain)" ]] || { echo 'The working tree must be clean.' >&2; return 1; }
  load_state
  upstream=$EXPORT_UPSTREAM
  origin=$EXPORT_ORIGIN
  ALLOWED+=(${EXPORT_ALLOWED[@]+"${EXPORT_ALLOWED[@]}"})
  save_state "$upstream" "$origin" "$EXPORT_NAME"

  if ! git merge-base --is-ancestor "$upstream" HEAD; then
    echo "HEAD is not based on public main ($upstream)." >&2
    return 1
  fi
  if git diff --quiet "$upstream" HEAD; then
    echo 'Nothing to export: HEAD has the same content as public main.' >&2
    return 1
  fi
  if [[ -n "$(git rev-list --merges "$upstream..HEAD")" ]]; then
    report 'The branch contains merge commits: export only linear commits on public main.'
  fi
  local commit
  for commit in $(git rev-list "$upstream..HEAD"); do
    if git merge-base --is-ancestor "$commit" "$origin"; then
      report "Commit $(git rev-parse --short "$commit") is an internal main commit: its SHA would be published."
    fi
  done

  # Check every exported commit, not only the final diff: something added by one commit and
  # removed by a later one is gone from the result but would still be published in history.
  for pattern in ${ALLOWED[@]+"${ALLOWED[@]}"}; do excludes+=(":(exclude,glob)$pattern"); done
  while IFS= read -r file; do
    for pattern in "${INTERNAL_PATHS[@]}"; do
      # shellcheck disable=SC2053 # glob match on purpose
      if [[ "$file" == $pattern ]]; then
        report "Internal-only file changed: $file (review it, then pass --allow-path '$file' if it must be public)"
      fi
    done
  done < <(git log --no-merges --no-renames --format= --name-only "$upstream..HEAD" -- . ${excludes[@]+"${excludes[@]}"} | sort -u)

  hits=$(git log --no-merges --no-renames -p -U0 --no-color --format='commit %h' "$upstream..HEAD" -- . ${excludes[@]+"${excludes[@]}"} \
    | awk -v re="$INTERNAL_TEXT" '
      /^commit / { commit = $2; next }
      /^\+\+\+ / { file = substr($0, 7); next }
      /^\+/ && tolower($0) ~ re { print "  " commit " " file ": " substr($0, 2) }')
  if [[ -n "$hits" ]]; then
    report "Lines added by these commits reference internal repositories or accounts:" "$hits"
  fi

  hits=$(git log --format='%h%n%B' "$upstream..HEAD" | grep -iE "$INTERNAL_TEXT" | sed 's/^/  /' || true)
  if [[ -n "$hits" ]]; then
    report "Commit messages reference internal repositories or accounts (reword them):" "$hits"
  fi
  hits=$(git log --format='%h author: %an <%ae>, committer: %cn <%ce>' "$upstream..HEAD" | grep -iE "$INTERNAL_TEXT" | sed 's/^/  /' || true)
  if [[ -n "$hits" ]]; then
    report "Commit identities use internal accounts (amend the author or committer):" "$hits"
  fi

  echo >&2
  if [[ "$problems" -gt 0 ]]; then
    echo "$problems problem(s) found: fix them in $(pwd) and run again with --check." >&2
    return 1
  fi
  echo "Ready to export. Review the result:" >&2
  git --no-pager log --format='  %h %s' "$upstream..HEAD" >&2
  git --no-pager diff --stat "$upstream" HEAD >&2
  cat >&2 <<EOF

Pushing discloses the change. For a security fix, agree on the timing with #agent-release-sync first.
To publish, from $(pwd):
  git push https://github.com/DataDog/datadog-agent.git HEAD:refs/heads/$EXPORT_NAME
  gh pr create --repo DataDog/datadog-agent --head $EXPORT_NAME --draft
Then clean up: cd - && git worktree remove $(pwd) && git branch -D export-to-public/$EXPORT_NAME
EOF
}

report() {
  problems=$((problems + 1))
  printf '%s\n' "$@" >&2
}

save_state() {
  {
    printf 'EXPORT_UPSTREAM=%q\nEXPORT_ORIGIN=%q\nEXPORT_NAME=%q\nEXPORT_ALLOWED=(' "$1" "$2" "$3"
    [[ ${#ALLOWED[@]} -eq 0 ]] || printf ' %q' "${ALLOWED[@]}"
    printf ' )\n'
  } >"$(git rev-parse --git-path export-to-public.state)"
}

load_state() {
  local state
  state=$(git rev-parse --git-path export-to-public.state)
  if [[ ! -f "$state" ]]; then
    echo "No $state: run this from the worktree prepared by $(basename "$SCRIPT_PATH")." >&2
    return 1
  fi
  # shellcheck disable=SC1090
  source "$state"
}

main "$@"

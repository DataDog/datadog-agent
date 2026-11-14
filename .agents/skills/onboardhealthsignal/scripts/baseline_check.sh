#!/usr/bin/env bash
# Tell your failures from the baseline: run the same test command in your worktree and in a clean worktree
# at the SAME SHA, then diff the failing test names. Your uncommitted changes are never touched (no stash).
#
#   bash scripts/baseline_check.sh <worktree> [--targets ./comp/healthplatform] [--log <your-run.log>] [--keep]
#
# Prints "NEW failures (n)" (caused by your change) and "baseline failures (m)" (pre-existing at that SHA).
# Exit 1 when there are NEW failures. Foreground only: each run can take minutes (cold Bazel).
# Env: BASELINE_TEST_CMD overrides the test command (default `dda inv test --targets=<targets>`);
#      BASELINE_SKIP_CODEGEN=1 skips `dda inv schema.codegen` in the clean worktree.
set -uo pipefail
export PATH="/opt/homebrew/opt/go@1.25/bin:$HOME/go/bin:$PATH"

die() { echo "baseline_check: $*" >&2; exit 2; }
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

wt="${1:-}"; [ -n "$wt" ] || die "usage: baseline_check.sh <worktree> [--targets T] [--log F] [--keep]"
shift
targets="./comp/healthplatform"; dirty_log=""; keep=0; given_clean=""
while [ $# -gt 0 ]; do
  case "$1" in
    --targets) targets="${2:-}"; shift 2 ;;
    --log) dirty_log="${2:-}"; shift 2 ;;
    --keep) keep=1; shift ;;
    --clean) given_clean="${2:-}"; shift 2 ;;   # reuse an existing clean worktree at the same SHA (never removed here)
    *) die "unknown option: $1" ;;
  esac
done
git -C "$wt" rev-parse --git-dir >/dev/null 2>&1 || die "not a git worktree: $wt"
cmd="${BASELINE_TEST_CMD:-dda inv test --targets=$targets}"
sha="$(git -C "$wt" rev-parse HEAD)"
tmp="$(mktemp -d)"
clean="$tmp/clean"
[ -n "$given_clean" ] && clean="$given_clean"

cleanup() {
  if [ "$keep" = 1 ]; then echo "kept: $clean (logs in $tmp)" >&2; return; fi
  if [ -z "$given_clean" ]; then
    git -C "$wt" worktree remove --force "$clean" >/dev/null 2>&1 || true
    git -C "$wt" worktree prune >/dev/null 2>&1 || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT

if [ -z "$dirty_log" ]; then
  dirty_log="$tmp/dirty.log"
  echo "baseline_check: running tests in $wt ..." >&2
  (cd "$wt" && bash -c "$cmd") > "$dirty_log" 2>&1 || true
fi

if [ -n "$given_clean" ]; then
  [ -d "$clean" ] || die "--clean worktree not found: $clean"
  [ "$(git -C "$clean" rev-parse HEAD 2>/dev/null)" = "$sha" ] || die "--clean worktree is not at $sha"
  echo "baseline_check: reusing clean worktree $clean at $sha" >&2
else
  echo "baseline_check: clean worktree at $sha ..." >&2
  git -C "$wt" worktree add --detach "$clean" "$sha" >/dev/null 2>&1 || die "cannot create clean worktree at $sha"
  if [ "${BASELINE_SKIP_CODEGEN:-0}" != 1 ]; then
    (cd "$clean" && dda inv schema.codegen) >/dev/null 2>&1 || echo "baseline_check: warning: schema.codegen failed in the clean worktree" >&2
  fi
fi
(cd "$clean" && bash -c "$cmd") > "$tmp/clean.log" 2>&1 || true

echo "compared at ${sha}"
python3 -I "$here/test_failures.py" "$dirty_log" "$tmp/clean.log"
exit $?

#!/usr/bin/env bash
# Measure what a new issue package adds to the cluster agent binary (the bundle ships there too).
#
#   bash scripts/deps_delta.sh <worktree> <pkg> [--tags "<tags>"] [--binary ./cmd/cluster-agent[@tag,tag]]...
#
# --binary is repeatable (the first one replaces the default): check other binaries the bundle ships in, each
# optionally with its own build tags after '@' (for example --binary ./cmd/agent@kubelet,docker). Without '@' a
# binary uses --tags (default: the cluster agent tags). Each binary gets its own verdict; exit 1 if ANY has new deps.
# Compares `go list -deps` (non-stdlib) of the cluster agent binary in a CLEAN worktree at the same SHA with
# `go list -deps` of the new package in your worktree, under the cluster agent build tags. Packages only the
# new package brings are "new to the cluster agent": exit 1 when there are any. Imports behind a build tag
# the cluster agent does not set (for example kubelet, docker) correctly do not count; that is the point of
# gating heavy imports. --tags REPLACES the default tag set. Foreground; your changes are never touched.
# Env: BASELINE_SKIP_CODEGEN=1 skips `dda inv schema.codegen` in the clean worktree.
set -uo pipefail
export PATH="/opt/homebrew/opt/go@1.25/bin:$HOME/go/bin:$PATH"

die() { echo "deps_delta: $*" >&2; exit 2; }

wt="${1:-}"; pkg="${2:-}"
[ -n "$wt" ] && [ -n "$pkg" ] || die "usage: deps_delta.sh <worktree> <pkg> [--tags T] [--binary B]"
shift 2
tags="kubeapiserver clusterchecks orchestrator"; binaries=(); given_clean=""
while [ $# -gt 0 ]; do
  case "$1" in
    --tags) tags="${2:-}"; shift 2 ;;
    --binary) binaries+=("${2:-}"); shift 2 ;;
    --clean) given_clean="${2:-}"; shift 2 ;;   # reuse an existing clean worktree at the same SHA (never removed here)
    *) die "unknown option: $1" ;;
  esac
done
[ "${#binaries[@]}" = 0 ] && binaries=("./cmd/cluster-agent")
git -C "$wt" rev-parse --git-dir >/dev/null 2>&1 || die "not a git worktree: $wt"
sha="$(git -C "$wt" rev-parse HEAD)"
tmp="$(mktemp -d)"; clean="$tmp/clean"
[ -n "$given_clean" ] && clean="$given_clean"
cleanup() {
  if [ -z "$given_clean" ]; then
    git -C "$wt" worktree remove --force "$clean" >/dev/null 2>&1 || true
    git -C "$wt" worktree prune >/dev/null 2>&1 || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT

if [ -n "$given_clean" ]; then
  [ -d "$clean" ] || die "--clean worktree not found: $clean"
  [ "$(git -C "$clean" rev-parse HEAD 2>/dev/null)" = "$sha" ] || die "--clean worktree is not at $sha"
else
  git -C "$wt" worktree add --detach "$clean" "$sha" >/dev/null 2>&1 || die "cannot create clean worktree at $sha"
  if [ "${BASELINE_SKIP_CODEGEN:-0}" != 1 ]; then
    (cd "$clean" && dda inv schema.codegen) >/dev/null 2>&1 || echo "deps_delta: warning: schema.codegen failed in the clean worktree" >&2
  fi
fi

fmt='{{if not .Standard}}{{.ImportPath}}{{end}}'
overall=0
for spec in "${binaries[@]}"; do
  binary="${spec%%@*}"
  if [ "$spec" != "$binary" ]; then csv="${spec#*@}"; else csv="$(echo "$tags" | tr ' ' ',')"; fi
  case "$binary" in */cluster-agent) label="the cluster agent" ;; *) label="$(basename "$binary")" ;; esac
  (cd "$clean" && go list -deps -tags "$csv" -f "$fmt" "$binary" 2>/dev/null) | grep -v '^$' | sort -u > "$tmp/base.txt"
  [ -s "$tmp/base.txt" ] || die "go list found no dependencies for $binary at $sha (wrong --binary or --tags?)"
  self="$(cd "$wt" && go list -tags "$csv" -f '{{.ImportPath}}' "./comp/healthplatform/issues/$pkg" 2>/dev/null)"
  [ -n "$self" ] || die "package ./comp/healthplatform/issues/$pkg not found or excluded by tags '$csv'"
  (cd "$wt" && go list -deps -tags "$csv" -f "$fmt" "./comp/healthplatform/issues/$pkg" 2>/dev/null) | grep -v '^$' | sort -u | grep -vx "$self" > "$tmp/pkg.txt"
  new="$(comm -13 "$tmp/base.txt" "$tmp/pkg.txt")"
  echo "$label tags: $csv (binary $binary at ${sha:0:12})"
  if [ -z "$new" ]; then
    echo "adds nothing to $label binary"
    continue
  fi
  overall=1
  echo "new to $label ($(echo "$new" | wc -l | tr -d ' ')):"
  echo "$new" | sed 's/^/  /'
  echo "move the import into a file gated by the matching build tag (with a check_noop.go stub), or share state through a leaf package already linked"
done
exit $overall

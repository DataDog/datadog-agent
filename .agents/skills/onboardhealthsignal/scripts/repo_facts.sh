#!/usr/bin/env bash
# One call that replaces the reading phase's first 10 greps: neighbor modules, config schema hits, and the
# feature-presence helpers, all from the worktree you are generating into (never from memory or main).
#
#   bash scripts/repo_facts.sh <worktree> [<keyword-or-dir>]... [--keyword K]... [--max N]
#
# A positional argument is a directory when it exists inside the worktree (relative or absolute path), else a
# keyword; --keyword K always means a keyword. Directories add a "dirs" section and scope an extra code search.
#
# Sections printed (plain text, each starts with "== name =="):
#   modules     every comp/healthplatform/issues/<pkg>: layout (A = module.go, B = issue file without module.go),
#               build tag of check.go, IssueName and IssueType constants
#   schema      for each --keyword: matching lines in the config schema yaml (pkg/config/schema/yaml)
#   gates       feature-presence helpers: Feature constants in pkg/config/env and the most-used
#               IsFeaturePresent(env.X) gates in comp/ and pkg/ (a ready-made presence gate often exists)
#   dirs        for each directory argument: its non-test .go files (starting points for owner code)
#   code        for each keyword: first code hits (file:line) outside tests, as starting points
# Read-only. Pass keywords from the engineer's I1/I2 (a config key, a subsystem word, an error string).
set -uo pipefail
die() { echo "repo_facts: $*" >&2; exit 2; }

wt="${1:-}"; [ -n "$wt" ] && [ -d "$wt" ] || die "usage: repo_facts.sh <worktree> [<keyword-or-dir>]... [--keyword K]... [--max N]"
shift
max=12; keywords=(); posargs=(); dirs=()
while [ $# -gt 0 ]; do
  case "$1" in
    --keyword) keywords+=("${2:-}"); shift 2 ;;
    --max) max="${2:-12}"; shift 2 ;;
    -*) die "unknown option: $1" ;;
    *) posargs+=("$1"); shift ;;
  esac
done
wt="$(cd "$wt" && pwd)"
cd "$wt" || die "cannot cd $wt"
for a in "${posargs[@]+"${posargs[@]}"}"; do
  [ -n "$a" ] || continue
  if [ -d "$a" ]; then
    d="$(cd "$a" && pwd)"
    case "$d/" in "$wt"/*) dirs+=("${d#"$wt"/}") ;; *) die "directory is outside the worktree: $a" ;; esac
  else
    keywords+=("$a")
  fi
done

echo "== modules (comp/healthplatform/issues) =="
if [ -d comp/healthplatform/issues ]; then
  for d in comp/healthplatform/issues/*/; do
    [ -d "$d" ] || continue
    name="$(basename "$d")"
    layout="?"
    if [ -f "$d/module.go" ]; then layout="A (module.go)"
    elif [ -f "$d/issue.go" ] || ls "$d"/*_issue.go >/dev/null 2>&1; then layout="B (no module.go)"; fi
    tag="$(grep -h -m1 '^//go:build' "$d"/check.go 2>/dev/null | sed 's#^//go:build ##')"
    iname="$(grep -h -m1 -E '^[[:space:]]*IssueName[[:space:]]*=' "$d"/*.go 2>/dev/null | sed -E 's/.*=[[:space:]]*//' | head -1)"
    itype="$(grep -h -m1 -E '^[[:space:]]*IssueType[[:space:]]*=' "$d"/*.go 2>/dev/null | sed -E 's/.*=[[:space:]]*//' | head -1)"
    printf '%-26s layout=%-16s tag=%-10s name=%s type=%s\n' "$name" "$layout" "${tag:--}" "${iname:--}" "${itype:--}"
  done
else
  echo "(comp/healthplatform/issues not found: wrong worktree or older layout)"
fi

echo "== schema (config keys) =="
if [ "${#keywords[@]}" = 0 ]; then echo "(no --keyword given)"; fi
for k in "${keywords[@]+"${keywords[@]}"}"; do
  [ -n "$k" ] || continue
  echo "-- $k"
  if [ -d pkg/config/schema/yaml ]; then
    grep -rn -i -F -- "$k" pkg/config/schema/yaml 2>/dev/null | head -"$max" | cut -c1-200
  else
    echo "(pkg/config/schema/yaml not found)"
  fi
done

echo "== gates (feature presence) =="
if ls pkg/config/env/*.go >/dev/null 2>&1; then
  echo "-- Feature constants (pkg/config/env):"
  grep -hE '^[[:space:]]*[A-Za-z0-9_]+[[:space:]]+Feature[[:space:]]*=[[:space:]]*"[^"]*"' pkg/config/env/*.go 2>/dev/null | grep -v _test | sed 's/^[[:space:]]*//' | head -40
  echo "-- most used IsFeaturePresent gates (comp/, pkg/):"
  grep -rhoE 'IsFeaturePresent\(env\.[A-Za-z0-9_]+\)' comp pkg 2>/dev/null | sort | uniq -c | sort -rn | head -15
  echo "note: IsFeaturePresent panics before feature detection; tests must call env.SetFeatures(t, ...)"
else
  echo "(pkg/config/env not found)"
fi

echo "== dirs =="
if [ "${#dirs[@]}" = 0 ]; then echo "(no directory argument given)"; fi
for d in "${dirs[@]+"${dirs[@]}"}"; do
  echo "-- $d"
  find "$d" -name '*.go' ! -name '*_test.go' 2>/dev/null | sort | head -"$max"
done

echo "== code (first hits outside tests) =="
for k in "${keywords[@]+"${keywords[@]}"}"; do
  [ -n "$k" ] || continue
  echo "-- $k"
  grep -rn -F --include='*.go' -- "$k" comp pkg cmd 2>/dev/null | grep -v '_test\.go' | head -"$max" | cut -c1-200
done
exit 0

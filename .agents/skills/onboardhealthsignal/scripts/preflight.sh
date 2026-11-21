#!/usr/bin/env bash
# Preflight helpers for /onboardhealthsignal (SPEC phase P0). Run with: bash scripts/preflight.sh <cmd>
#
#   toolchain [--json]                      report required/optional tools using this machine's PATH recipe
#   auth                                    gh auth status (exit code is the answer)
#   worktree <clone> <ref> <dest> [--sparse]  add a detached worktree from a (possibly --no-checkout) clone;
#                                           <ref> may be a branch or a SHA; <dest> may be relative (resolved against cwd)
#   rules <worktree> [--json]               print HEAD sha and the three rule files read at that sha
#   mergebase <pr-number> [owner/repo]      print the merge-base sha of a PR (for backtests)
#   catalog-commit <agent-clone> <agent-sha> <dd-source-clone> [ref]
#                                           print the last dd-source commit STRICTLY before the agent
#                                           commit's timestamp (full epoch, not a bare date: a bare
#                                           date can pick a later commit from the same day)
#   cache [--worktree W [--apply]] [--root DIR]
#                                           shared bazel disk + repository cache (default root
#                                           $ONBOARD_BAZEL_CACHE or ~/.cache/onboard-bazel). Prints the flags;
#                                           with --apply writes them to <W>/user.bazelrc (imported by the
#                                           agent repo's .bazelrc, gitignored there; otherwise the file is
#                                           added to the worktree's local .git info/exclude). Never edits ~/.bazelrc
#                                           or any tracked file. A warm cache cuts cold gazelle from ~2.5 min to ~1 min.
#   bazel-shutdown-idle [--current DIR] [--yes | --ask]
#                                           list bazel servers of OTHER worktrees and stop only idle ones.
#                                           Never touches the current worktree's server or any server with a
#                                           running client (bazelisk/bazel/dda/go) in its workspace. Default is
#                                           list-only; --yes stops them; --ask prompts per server (tty only).
#
# Never pushes, never deploys. Fetches use the gh credential helper over HTTPS (ssh-agent signing is
# unreliable on this machine). Set NO_FETCH=1 to skip the fetch (required when pinning a historical SHA
# so later history is never pulled in). Run with `bash`, check `$?` directly.
set -euo pipefail

# Go toolchain PATH recipe, same as the agent-commit / agent-pre-push skills.
PATH_RECIPE='/opt/homebrew/opt/go@1.25/bin:$HOME/go/bin'
export PATH="/opt/homebrew/opt/go@1.25/bin:$HOME/go/bin:$PATH"
[ -n "${ONBOARD_PATH_PREPEND:-}" ] && export PATH="$ONBOARD_PATH_PREPEND:$PATH"   # tests and dry runs

RULE_FILES=(
  "comp/healthplatform/AGENTS.md"
  ".agents/skills/create-pr/SKILL.md"
  "test/new-e2e/tests/agent-health/.claude/skills/write-agent-health-e2e/SKILL.md"
)
SPARSE_DIRS=(comp/healthplatform .agents .github releasenotes releasenotes-dca test/new-e2e/tests/agent-health)

die() { echo "preflight: $*" >&2; exit 1; }

# name:required:hint
TOOLS="git:1:install git;gh:1:brew install gh;go:1:brew install go@1.25;dda:1:see datadog-agent docs;bazel:1:go install bazelisk as bazel;node:0:catalog slice only;npm:0:catalog slice only;docker:0:needed for run-e2e dev container;mise:0:optional;ddr:0:needed for conductor staging deploy (user runs it);op:0:secrets;python3:1:install python3"

cmd_toolchain() {
  local json=0 ok=1 first=1 out=""
  [ "${1:-}" = "--json" ] && json=1
  local IFS=';'
  for entry in $TOOLS; do
    local name req hint found path
    name="${entry%%:*}"; rest="${entry#*:}"; req="${rest%%:*}"; hint="${rest#*:}"
    path="$(command -v "$name" 2>/dev/null || true)"
    if [ -z "$path" ] && [ "$name" = "bazel" ]; then path="$(command -v bazelisk 2>/dev/null || true)"; fi
    if [ -n "$path" ]; then found=true; else found=false; fi
    if [ "$found" = false ] && [ "$req" = 1 ]; then ok=0; fi
    if [ "$json" = 1 ]; then
      [ $first = 1 ] || out="$out,"
      first=0
      out="$out{\"name\":\"$name\",\"found\":$found,\"required\":$([ "$req" = 1 ] && echo true || echo false),\"path\":\"$path\",\"hint\":\"$hint\"}"
    else
      printf '%-8s %-9s %s\n' "$name" "$([ "$found" = true ] && echo found || echo MISSING)" "${path:-$hint}"
    fi
  done
  if [ "$json" = 1 ]; then
    printf '{"ok":%s,"path_recipe":"%s","tools":[%s]}\n' "$([ $ok = 1 ] && echo true || echo false)" "$PATH_RECIPE" "$out"
  else
    echo "required tools: $([ $ok = 1 ] && echo OK || echo 'MISSING (see above)')"
  fi
  return 0
}

# Prints an explicit status line and returns gh's own exit code, so callers never need PIPESTATUS
# (empty under zsh). Always run this script with bash and read $? directly; do not pipe it.
cmd_auth() {
  local rc=0
  gh auth status || rc=$?
  if [ "$rc" = 0 ]; then echo "auth: ok"; else echo "auth: FAILED (gh exit $rc)"; fi
  return "$rc"
}

cmd_worktree() {
  local clone="${1:-}" ref="${2:-}" dest="${3:-}" sparse="${4:-}"
  [ -n "$clone" ] && [ -n "$ref" ] && [ -n "$dest" ] || die "usage: worktree <clone> <ref> <dest> [--sparse]"
  git -C "$clone" rev-parse --git-dir >/dev/null 2>&1 || die "not a git clone: $clone"
  # Resolve dest against the caller's cwd, never against the clone (git -C would do the latter).
  case "$dest" in /*) ;; *) dest="$PWD/$dest" ;; esac
  mkdir -p "$(dirname "$dest")"
  if [ -e "$dest" ] && [ -n "$(ls -A "$dest" 2>/dev/null)" ]; then die "dest exists and is not empty: $dest"; fi
  if [ "${NO_FETCH:-0}" != 1 ]; then
    git -C "$clone" -c credential.helper='!gh auth git-credential' fetch --quiet origin 2>/dev/null \
      || echo "preflight: warning: fetch failed, using local refs" >&2
  fi
  local sha
  sha="$(git -C "$clone" rev-parse --verify "$ref^{commit}")" || die "cannot resolve ref: $ref"
  if [ "$sparse" = "--sparse" ]; then
    git -C "$clone" worktree add --detach --no-checkout "$dest" "$sha" >/dev/null
    git -C "$dest" sparse-checkout set --cone "${SPARSE_DIRS[@]}"
    git -C "$dest" read-tree -m -u HEAD
  else
    git -C "$clone" worktree add --detach "$dest" "$sha" >/dev/null
  fi
  echo "worktree: $dest @ $sha"
}

cmd_rules() {
  local wt="${1:-}" json=0 missing=0
  [ -n "$wt" ] || die "usage: rules <worktree> [--json]"
  [ "${2:-}" = "--json" ] && json=1
  local sha entries="" first=1
  sha="$(git -C "$wt" rev-parse HEAD)"
  for f in "${RULE_FILES[@]}"; do
    local exists=false hash=""
    if [ -f "$wt/$f" ]; then exists=true; hash="$(shasum -a 256 "$wt/$f" | cut -d' ' -f1)"; else missing=1; fi
    if [ "$json" = 1 ]; then
      [ $first = 1 ] || entries="$entries,"
      first=0
      entries="$entries{\"path\":\"$f\",\"exists\":$exists,\"sha256\":\"$hash\"}"
    else
      printf '%-90s %s %s\n' "$f" "$exists" "$hash"
    fi
  done
  [ "$json" = 1 ] && printf '{"sha":"%s","files":[%s]}\n' "$sha" "$entries" || echo "sha: $sha"
  return $missing
}

cmd_mergebase() {
  local pr="${1:-}" repo="${2:-DataDog/datadog-agent}"
  [ -n "$pr" ] || die "usage: mergebase <pr-number> [owner/repo]"
  local base head
  base="$(gh api "repos/$repo/pulls/$pr" --jq .base.sha)"
  head="$(gh api "repos/$repo/pulls/$pr" --jq .head.sha)"
  gh api "repos/$repo/compare/$base...$head" --jq .merge_base_commit.sha
}

cmd_catalog_commit() {
  local agent="${1:-}" sha="${2:-}" dd="${3:-}" ref="${4:-origin/main}"
  [ -n "$agent" ] && [ -n "$sha" ] && [ -n "$dd" ] || die "usage: catalog-commit <agent-clone> <agent-sha> <dd-source-clone> [ref]"
  local ct iso cutoff found found_iso
  ct="$(git -C "$agent" log -1 --format=%ct "$sha")" || die "cannot read agent commit $sha"
  iso="$(git -C "$agent" log -1 --format=%cI "$sha")"
  cutoff=$((ct - 1))
  found="$(git -C "$dd" rev-list -1 --first-parent --before="$cutoff +0000" "$ref")" || die "cannot walk $ref in $dd"
  [ -n "$found" ] || die "no dd-source commit strictly before $iso on $ref"
  found_iso="$(git -C "$dd" log -1 --format=%cI "$found")"
  echo "preflight: agent commit ${sha:0:12} at $iso; catalog commit ${found:0:12} at $found_iso (strictly earlier)" >&2
  echo "$found"
}

cmd_cache() {
  local wt="" apply=0 root="${ONBOARD_BAZEL_CACHE:-$HOME/.cache/onboard-bazel}"
  while [ $# -gt 0 ]; do
    case "$1" in
      --worktree) wt="${2:-}"; shift 2 ;;
      --apply) apply=1; shift ;;
      --root) root="${2:-}"; shift 2 ;;
      *) die "usage: cache [--worktree W [--apply]] [--root DIR]" ;;
    esac
  done
  mkdir -p "$root/disk" "$root/repo"
  local l1="common --disk_cache=$root/disk" l2="common --repository_cache=$root/repo"
  echo "shared bazel cache root: $root"
  echo "$l1"
  echo "$l2"
  if [ "$apply" = 1 ]; then
    [ -n "$wt" ] && [ -d "$wt" ] || die "--apply needs --worktree <dir>"
    local rc="$wt/user.bazelrc"
    touch "$rc"
    grep -qxF "$l1" "$rc" || echo "$l1" >> "$rc"
    grep -qxF "$l2" "$rc" || echo "$l2" >> "$rc"
    if ! git -C "$wt" check-ignore -q user.bazelrc 2>/dev/null; then
      local ex; ex="$(git -C "$wt" rev-parse --git-path info/exclude 2>/dev/null || true)"
      if [ -n "$ex" ]; then
        case "$ex" in /*) ;; *) ex="$wt/$ex" ;; esac
        mkdir -p "$(dirname "$ex")"; grep -qxF 'user.bazelrc' "$ex" 2>/dev/null || echo 'user.bazelrc' >> "$ex"
      fi
    fi
    echo "applied: $rc (local only; never commit it)"
  else
    echo "not applied (use --worktree <dir> --apply to write <dir>/user.bazelrc)"
  fi
}

# Emits "pid|workspace|busy" lines. BAZEL_SERVERS_FILE (same format) replaces discovery (tests, dry runs).
bazel_servers() {
  if [ -n "${BAZEL_SERVERS_FILE:-}" ]; then cat "$BAZEL_SERVERS_FILE"; return 0; fi
  local cwds="" p c
  for p in $(pgrep -f 'bazelisk|/bazel( |$)|dda inv|gotestsum|go test|go build' 2>/dev/null || true); do
    c="$(lsof -a -d cwd -p "$p" -Fn 2>/dev/null | sed -n 's/^n//p' | head -1)"
    [ -n "$c" ] && cwds="$cwds
$c"
  done
  for p in $(pgrep -f 'bazel\(' 2>/dev/null || true); do
    local ws busy=0
    ws="$(lsof -a -d cwd -p "$p" -Fn 2>/dev/null | sed -n 's/^n//p' | head -1)"
    [ -n "$ws" ] || continue
    if printf '%s\n' "$cwds" | grep -q "^$ws\(/\|$\)"; then busy=1; fi
    echo "$p|$ws|$busy"
  done
}

cmd_bazel_shutdown_idle() {
  local cur="" mode=list
  while [ $# -gt 0 ]; do
    case "$1" in
      --current) cur="${2:-}"; shift 2 ;;
      --yes) mode=yes; shift ;;
      --ask) mode=ask; shift ;;
      *) die "usage: bazel-shutdown-idle [--current DIR] [--yes | --ask]" ;;
    esac
  done
  [ -n "$cur" ] || cur="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
  cur="$(cd "$cur" 2>/dev/null && pwd -P || echo "$cur")"
  local line pid ws busy n_idle=0 n_keep=0
  while IFS='|' read -r pid ws busy; do
    [ -n "$pid" ] || continue
    ws="$(cd "$ws" 2>/dev/null && pwd -P || echo "$ws")"   # normalize (/var vs /private/var on macOS)
    local why=""
    if [ "$ws" = "$cur" ] || [ "${ws#"$cur"/}" != "$ws" ]; then why="current worktree"
    elif [ "$busy" = 1 ]; then why="in use (client running in workspace)"; fi
    if [ -n "$why" ]; then
      echo "keep  pid=$pid ws=$ws ($why)"; n_keep=$((n_keep+1)); continue
    fi
    case "$mode" in
      list) echo "idle  pid=$pid ws=$ws (would stop; rerun with --yes or --ask)" ;;
      yes) echo "stop  pid=$pid ws=$ws"; (cd "$ws" && bazelisk shutdown >/dev/null 2>&1 || bazel shutdown >/dev/null 2>&1) || echo "  warning: shutdown failed for $ws" >&2 ;;
      ask)
        if [ -t 0 ]; then
          printf 'stop idle bazel server pid=%s ws=%s? [y/N] ' "$pid" "$ws"; read -r ans
          case "$ans" in y|Y) (cd "$ws" && bazelisk shutdown >/dev/null 2>&1 || bazel shutdown >/dev/null 2>&1) ;; *) echo "skipped" ;; esac
        else echo "idle  pid=$pid ws=$ws (no tty: not stopping)"; fi ;;
    esac
    n_idle=$((n_idle+1))
  done < <(bazel_servers)
  echo "summary: idle=$n_idle kept=$n_keep mode=$mode"
}

sub="${1:-}"; shift || true
case "$sub" in
  toolchain) cmd_toolchain "$@" ;;
  auth) cmd_auth ;;
  worktree) cmd_worktree "$@" ;;
  rules) cmd_rules "$@" ;;
  mergebase) cmd_mergebase "$@" ;;
  catalog-commit) cmd_catalog_commit "$@" ;;
  cache) cmd_cache "$@" ;;
  bazel-shutdown-idle) cmd_bazel_shutdown_idle "$@" ;;
  *) die "usage: preflight.sh {toolchain|auth|worktree|rules|mergebase|catalog-commit|cache|bazel-shutdown-idle} ..." ;;
esac

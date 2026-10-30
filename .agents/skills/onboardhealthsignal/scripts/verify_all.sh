#!/usr/bin/env bash
# One-shot verification for a generated Agent Health signal: runs every gate in parallel where it is safe,
# prints ONE JSON summary, exits nonzero if any gate failed. Replaces ~15 sequential tool calls.
#
#   bash scripts/verify_all.sh --worktree <wt> --path A|B --pkg <package> [--owner-dirs "dirA dirB"]
#        [--render-json <render_templates.py output>] [--reg-test TestName]
#        [--mutation-cmd '<cmd>' ...] [--mutants-file <file>] [--catalog-dir <dd-source wt>] [--node-modules <dir>]
#        [--out-dir <dir>] [--skip gate,gate] [--baseline on-fail|always|never] [--gazelle-wait SECONDS]
#        [--deps-binary ./cmd/<bin>[@tags]]... [--keep] [--dry-run]
# deps_delta checks the cluster agent by default; each --deps-binary adds a binary (optionally with its own build tags).
#
# Registration test name: --reg-test, else integration_test_name from --render-json (save the renderer's printed
# JSON to a file; <out-dir>/render.json is picked up automatically), else a package-derived guess (a guess can
# match nothing: such a run is reported INCONCLUSIVE, never as a surviving mutant). The registration test runs
# unmutated as its own gate (registration, log registration.log) with -v, Path A only.
# Check-body mutants (Path A, or extra Path B ones): --mutants-file F (default <out-dir>/mutants.txt if present),
# one mutant per line "name<TAB>command"; the command applies the mutation, runs its test, and restores the files
# (same contract as --mutation-cmd). Blank lines and '#' lines are ignored.
# --keep leaves the clean worktree (<out-dir>/clean) in place; by default it is removed at the end, also on error.
#
# RUN IT IN THE BACKGROUND (it takes minutes; a foreground tool call can time out) and read the summary:
#   bash scripts/verify_all.sh ... > "$OUT/summary.json" 2>"$OUT/stderr.log" &   # then poll $OUT/progress.log
# Progress lines go to <out-dir>/progress.log; per-gate logs are <out-dir>/<gate>.log; the summary is also
# written to <out-dir>/summary.json. Default out-dir: $ONBOARD_RESULTS_DIR, else a sibling of the worktree
# (never inside the repo, so it cannot dirty git status).
#
# Order (the critical path is gazelle + the slowest gate, not the sum):
#   t0   start in parallel: prep_clean (clean worktree at the same SHA, once), catalog gate (own tree)
#   1    gazelle (background-safe: the script waits on its GAZELLE_EXIT= line, however long it takes)
#   2    in parallel: pkg_tests, modified_tests, lint, deps_delta (Path A and B)
#   3    mutation (serial, because it edits the tree the tests read): Path A removes the bundle.go blank
#        import; every --mutation-cmd / mutants-file row runs too (each must apply its mutation, run its test,
#        and restore the files; the script verifies git diff is byte-identical afterwards). Each mutant is
#        KILLED (a test failed), SURVIVED, COMPILE_ERROR (build failure: not a kill), INCONCLUSIVE (the named
#        test never ran), or RESTORE_FAILED. The gate is derived from that table: pass only if all are KILLED.
#   4    baseline_check, only when a test gate failed (default --baseline on-fail)
# A mutation gate PASSES when the mutated test run FAILS (the mutation was caught); the observed exit code is
# reported, never assumed. Test commands probe `dda inv test --help`: if --test-args exists it is used, else
# the older `--test-run-name='^Name$' -v` form (older SHAs lack --test-args/--bazel-args).
# Gates: gazelle pkg_tests modified_tests lint registration deps_delta mutation baseline catalog. Skipped gates are
# reported as "skip", never as "pass". eslint is NEVER run by this script; the catalog gate uses a no-op
# eslint shim only so turbo's tsc checks can run: it is not a lint substitute and the PR body must say so.
# Never pushes, never deploys.
set -uo pipefail
export PATH="/opt/homebrew/opt/go@1.25/bin:$HOME/go/bin:$PATH"
[ -n "${ONBOARD_PATH_PREPEND:-}" ] && export PATH="$ONBOARD_PATH_PREPEND:$PATH"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

die() { echo "verify_all: $*" >&2; exit 2; }
usage() { sed -n '2,/^set -uo/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'; }

wt=""; path=""; pkg=""; owner_dirs=""; reg_test=""; catalog_dir=""; node_modules=""; out=""
skip=","; baseline_mode="on-fail"; gazelle_wait=1800; dry=0
mutation_cmds=(); mutation_names=(); deps_binaries=(); render_json=""; mutants_file=""; keep=0
while [ $# -gt 0 ]; do
  case "$1" in
    --worktree) wt="${2:-}"; shift 2 ;;
    --path) path="${2:-}"; shift 2 ;;
    --pkg) pkg="${2:-}"; shift 2 ;;
    --owner-dirs) owner_dirs="${2:-}"; shift 2 ;;
    --reg-test) reg_test="${2:-}"; shift 2 ;;
    --mutation-cmd) mutation_cmds+=("${2:-}"); mutation_names+=("mutant-$(( ${#mutation_cmds[@]} ))"); shift 2 ;;
    --render-json) render_json="${2:-}"; shift 2 ;;
    --mutants-file) mutants_file="${2:-}"; shift 2 ;;
    --keep) keep=1; shift ;;
    --deps-binary) deps_binaries+=("${2:-}"); shift 2 ;;
    --catalog-dir) catalog_dir="${2:-}"; shift 2 ;;
    --node-modules) node_modules="${2:-}"; shift 2 ;;
    --out-dir) out="${2:-}"; shift 2 ;;
    --skip) skip=",${2:-},"; shift 2 ;;
    --baseline) baseline_mode="${2:-}"; shift 2 ;;
    --gazelle-wait) gazelle_wait="${2:-}"; shift 2 ;;
    --dry-run) dry=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done
[ -n "$wt" ] && [ -n "$path" ] && [ -n "$pkg" ] || die "usage: --worktree W --path A|B --pkg P (see --help)"
case "$path" in A|B) ;; *) die "--path must be A or B" ;; esac
case "$baseline_mode" in on-fail|always|never) ;; *) die "--baseline must be on-fail|always|never" ;; esac
[ -d "$wt" ] || die "worktree not found: $wt"
wt="$(cd "$wt" && pwd)"
if [ -z "$out" ]; then
  out="${ONBOARD_RESULTS_DIR:-$(dirname "$wt")/onboard-verify-$(basename "$wt")}"
fi
mkdir -p "$out" || die "cannot create $out"
out="$(cd "$out" && pwd)"
case "$out/" in "$wt"/*) die "--out-dir must not be inside the worktree (it would dirty git status): $out" ;; esac
: > "$out/gates.list"; : > "$out/progress.log"
progress() { echo "$(date +%H:%M:%S) $*" >> "$out/progress.log"; }

skipped() { case "$skip" in *",$1,"*) return 0 ;; esac; return 1; }

# --- targets -----------------------------------------------------------------------------------------
pkg_target="./comp/healthplatform/issues/$pkg"
test_targets="$pkg_target"; gazelle_targets="./comp/healthplatform"; lint_targets="$pkg_target"
for d in $(echo "$owner_dirs" | tr ',' ' '); do
  d="${d#./}"; d="${d%/}"
  test_targets="$test_targets,./$d"; gazelle_targets="$gazelle_targets ./$d"; lint_targets="$lint_targets,./$d"
done
lint_targets="$lint_targets,./comp/healthplatform"
reg_source="flag"
if [ -z "$reg_test" ]; then
  [ -z "$render_json" ] && [ -f "$out/render.json" ] && render_json="$out/render.json"
  if [ -n "$render_json" ]; then
    [ -f "$render_json" ] || die "--render-json not found: $render_json"
    reg_test="$(python3 -I -c 'import json,sys; print(json.load(open(sys.argv[1])).get("integration_test_name",""))' "$render_json" 2>/dev/null)"
    [ -n "$reg_test" ] || die "no integration_test_name in $render_json"
    reg_source="render-json"
  fi
fi
if [ -z "$reg_test" ]; then
  reg_source="package-derived-guess"
  echo "verify_all: warning: no --reg-test/--render-json; guessing the registration test name from the package" >&2
  first="$(printf '%s' "${pkg:0:1}" | tr '[:lower:]' '[:upper:]')"
  reg_test="Test${first}${pkg:1}SurvivesFullPipeline"
fi

# mutants file: "name<TAB>command" per line (command applies the mutation, runs the test, restores the files)
[ -z "$mutants_file" ] && [ -f "$out/mutants.txt" ] && mutants_file="$out/mutants.txt"
if [ -n "$mutants_file" ]; then
  [ -f "$mutants_file" ] || die "--mutants-file not found: $mutants_file"
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ""|"#"*) continue ;; esac
    case "$line" in *$'\t'*) mname="${line%%$'\t'*}"; mcmd="${line#*$'\t'}" ;; *) mname="$line"; mcmd="$line" ;; esac
    mutation_names+=("$mname"); mutation_cmds+=("$mcmd")
  done < "$mutants_file"
fi

# --- dda flag probe ----------------------------------------------------------------------------------
if [ -n "${DDA_HELP_FILE:-}" ]; then dda_help="$(cat "$DDA_HELP_FILE")"; else dda_help="$(cd "$wt" && dda inv test --help 2>&1 || true)"; fi
if printf '%s' "$dda_help" | grep -q -- '--test-args'; then dda_style="test-args"; else dda_style="run-name"; fi
named_test_cmd() { # $1 test name, $2 targets
  if [ "$dda_style" = "test-args" ]; then
    echo "dda inv test --targets=$2 --test-args='-test.run=^$1\$ -test.v' --bazel-args='--test_output=all'"
  else
    echo "dda inv test --targets=$2 --test-run-name='^$1\$' -v"
  fi
}

# --- gate runner -------------------------------------------------------------------------------------
declare_gate() { echo "$1" >> "$out/gates.list"; echo "$2" > "$out/$1.cmd"; }
mark() { # name status [rc]
  echo "$2" > "$out/$1.status"; [ -n "${3:-}" ] && echo "$3" > "$out/$1.rc"; return 0
}
run_gate() { # name cmd...  -- runs in the worktree, in the background, records rc and seconds
  local name="$1"; shift; local cmd="$*"
  declare_gate "$name" "$cmd"
  if skipped "$name"; then mark "$name" skip; return 0; fi
  if [ "$dry" = 1 ]; then mark "$name" plan; return 0; fi
  progress "start $name"
  ( t0=$(date +%s); (cd "$wt" && bash -c "$cmd") > "$out/$name.log" 2>&1; rc=$?
    echo "$rc" > "$out/$name.rc"; echo $(( $(date +%s) - t0 )) > "$out/$name.sec"
    if [ "$rc" = 0 ]; then echo pass > "$out/$name.status"; else echo fail > "$out/$name.status"; fi
    echo "$(date +%H:%M:%S) done $name rc=$rc" >> "$out/progress.log" ) &
  echo $! > "$out/$name.pid"
}
wait_gate() { local p; p="$(cat "$out/$1.pid" 2>/dev/null || true)"; [ -n "$p" ] && wait "$p" 2>/dev/null; return 0; }
gate_failed() { [ "$(cat "$out/$1.status" 2>/dev/null)" = fail ]; }

sha="$(git -C "$wt" rev-parse HEAD 2>/dev/null || echo unknown)"

# --- t0: clean worktree (shared by deps_delta and baseline) and catalog gate -------------------------
clean="$out/clean"
remove_clean() { # the full clean worktree is large: never leave it behind unless --keep
  [ "$keep" = 1 ] && return 0
  [ -e "$clean" ] || { git -C "$wt" worktree prune >/dev/null 2>&1; return 0; }
  git -C "$wt" worktree remove --force "$clean" >/dev/null 2>&1 || true
  [ -e "$clean" ] && rm -rf "$clean"
  git -C "$wt" worktree prune >/dev/null 2>&1 || true
  return 0
}
trap remove_clean EXIT
trap 'remove_clean; exit 130' INT TERM
need_clean=0
if ! skipped deps_delta || { [ "$baseline_mode" != never ] && ! skipped baseline; }; then need_clean=1; fi
if [ "$need_clean" = 1 ] && [ "$dry" = 0 ]; then
  progress "start prep_clean"
  ( git -C "$wt" worktree add --detach "$clean" "$sha" >/dev/null 2>&1 \
      && { [ "${BASELINE_SKIP_CODEGEN:-0}" = 1 ] || (cd "$clean" && dda inv schema.codegen >/dev/null 2>&1) || true; } \
      && touch "$out/clean.ready" || touch "$out/clean.failed"
    echo "$(date +%H:%M:%S) done prep_clean" >> "$out/progress.log" ) &
  echo $! > "$out/prep_clean.pid"
fi

catalog_script="$out/catalog_gate.sh"
write_catalog_script() {
  cat > "$catalog_script" <<'EOS'
#!/usr/bin/env bash
# Catalog gates on real tooling (see references/catalog.md). Args: <catalog-dd-source-worktree> <out-dir> [<node_modules to symlink>]
set -u
cat_dir="$1"; out="$2"; nm="${3:-}"
EW="$cat_dir/domains/evp-workers"; APP="$EW/apps/agthealth-worker"
[ -d "$APP" ] || { echo "catalog: $APP not found (sparse-checkout domains/evp-workers first)"; exit 1; }
if [ ! -e "$EW/node_modules" ] && [ -n "$nm" ]; then ln -s "$nm" "$EW/node_modules"; fi
[ -e "$EW/node_modules" ] || { echo "catalog: no node_modules (pass --node-modules <main tree node_modules>; do NOT npm install)"; exit 1; }
# No-op eslint shim: ONLY so turbo's tsc checks can run on older commits. NOT a lint substitute.
mkdir -p "$out/ewshim"; printf '#!/bin/sh\necho "eslint shim: skipped (not a lint substitute)"\nexit 0\n' > "$out/ewshim/eslint"; chmod +x "$out/ewshim/eslint"
export PATH="$out/ewshim:$EW/node_modules/.bin:$PATH"
export TURBO_CACHE_DIR="$EW/.turbo/cache"
rc=0
revert_agents() { for f in $(git -C "$cat_dir" diff --name-only 2>/dev/null | grep 'AGENTS.md$'); do git -C "$cat_dir" checkout -- "$f" 2>/dev/null; done; }
trap revert_agents EXIT
if [ -L "$EW/node_modules" ]; then
  real="$(cd "$EW/node_modules" && pwd -P)"
  [ -f "$real/@event-workers/jest-transformer/dist/index.js" ] || { echo "catalog: symlinked node_modules points at a tree whose workspace libs are not built (build them there: turbo run build --filter=@event-workers/testkit)"; exit 1; }
else
  (cd "$EW" && turbo run build --filter=@event-workers/testkit --concurrency=4) || exit 1
fi
(cd "$APP" && node scripts/gen-catalog-templates.mjs && node scripts/gen-catalog-templates.mjs) || { echo "catalog: gen-templates failed"; exit 1; }
(cd "$EW" && node_modules/.bin/prettier --check apps/agthealth-worker/src/catalog/generated/*.ts apps/agthealth-worker/src/catalog/index.ts) || { echo "catalog: prettier check failed"; rc=1; }
(cd "$APP" && node --no-warnings --experimental-vm-modules ../../node_modules/jest/bin/jest.js --no-cache src/catalog) || { echo "catalog: jest failed"; rc=1; }
echo "catalog: eslint was NOT run (shim only); say so in the PR body."
exit $rc
EOS
}
if [ -n "$catalog_dir" ]; then
  write_catalog_script
  run_gate catalog "bash '$catalog_script' '$catalog_dir' '$out' '$node_modules'"
else
  declare_gate catalog "(no --catalog-dir)"; mark catalog skip
fi

# --- 1. gazelle (background-safe wait on its exit line) ----------------------------------------------
declare_gate gazelle "bazel run //:gazelle -- $gazelle_targets"
if skipped gazelle; then mark gazelle skip
elif [ "$dry" = 1 ]; then mark gazelle plan
else
  progress "start gazelle"
  ( t0=$(date +%s); (cd "$wt" && bazel run //:gazelle -- $gazelle_targets) > "$out/gazelle.log" 2>&1
    echo "GAZELLE_EXIT=$?" >> "$out/gazelle.log"; echo $(( $(date +%s) - t0 )) > "$out/gazelle.sec" ) &
  gpid=$!
  waited=0
  while ! grep -q '^GAZELLE_EXIT=' "$out/gazelle.log" 2>/dev/null; do
    sleep 1; waited=$((waited + 1))
    if [ "$waited" -ge "$gazelle_wait" ]; then break; fi
  done
  wait "$gpid" 2>/dev/null
  grc="$(sed -n 's/^GAZELLE_EXIT=//p' "$out/gazelle.log" | tail -1)"
  if [ "${grc:-}" = 0 ]; then mark gazelle pass 0; else mark gazelle fail "${grc:-timeout}"; fi
  progress "done gazelle rc=${grc:-timeout}"
fi

# --- 2. parallel gates -------------------------------------------------------------------------------
if gate_failed gazelle; then
  for g in pkg_tests modified_tests lint registration; do declare_gate "$g" "(not run: gazelle failed)"; mark "$g" skip; done
else
  run_gate pkg_tests "dda inv test --targets=$test_targets"
  run_gate modified_tests "dda inv test --only-modified-packages"
  run_gate lint "dda inv linter.go --module=. --targets=$lint_targets"
fi
deps_extra=""
if [ "${#deps_binaries[@]}" -gt 0 ]; then deps_extra=" --binary ./cmd/cluster-agent"; for b in "${deps_binaries[@]}"; do deps_extra="$deps_extra --binary '$b'"; done; fi
dd_cmd="n=0; while [ ! -f '$out/clean.ready' ] && [ ! -f '$out/clean.failed' ] && [ \$n -lt 900 ]; do sleep 2; n=\$((n+2)); done; [ -f '$out/clean.ready' ] || { echo 'prep_clean failed'; exit 2; }; bash '$here/deps_delta.sh' '$wt' '$pkg' --clean '$clean'$deps_extra"
run_gate deps_delta "$dd_cmd"
if [ "$path" = A ]; then
  rt_cmd="$(named_test_cmd "$reg_test" "./comp/healthplatform")"
  # a -test.run that matches nothing exits 0: require the explicit '--- PASS: <name>' line (needs -v)
  run_gate registration "$rt_cmd > '$out/registration.raw' 2>&1; rc=\$?; cat '$out/registration.raw'; [ \$rc = 0 ] || exit \$rc; grep -Eq -- '--- PASS: $reg_test( |\$)' '$out/registration.raw' || { echo 'registration: $reg_test never ran (no --- PASS line; wrong test name?)'; exit 3; }"
  echo "$rt_cmd" > "$out/registration.cmd"
else
  declare_gate registration "(Path B: no bundle.go registration test)"; mark registration skip
fi
for g in pkg_tests modified_tests lint registration; do wait_gate "$g"; done

# --- 3. mutation (serial: it edits files the tests read) --------------------------------------------
# Every mutant becomes one row in mutation.tsv (name, status, observed exit code, note); the gate status is
# DERIVED from that table (pass only when there is at least one row and every row is KILLED).
declare_gate mutation "(see mutation.log and mutation.tsv)"
mut_tsv="$out/mutation.tsv"; : > "$mut_tsv"
add_mutant() { printf '%s\t%s\t%s\t%s\n' "$1" "$2" "${3:--}" "${4:--}" >> "$mut_tsv"; }
# a build failure is not a kill: the mutated test never ran
COMPILE_RE='\[build failed\]|build failed|imported and not used|declared and not used|(^|[[:space:]])undefined: |syntax error|compilation failed|Compilation of .* failed|GoCompilePkg|too many errors'
classify() { # $1 mutant log $2 observed exit code -> KILLED | COMPILE_ERROR | SURVIVED
  if [ "$2" = 0 ]; then echo SURVIVED
  elif ! grep -Eq '^[[:space:]]*--- FAIL:' "$1" && grep -Eq "$COMPILE_RE" "$1"; then echo COMPILE_ERROR
  else echo KILLED; fi
}
if skipped mutation; then mark mutation skip
elif [ "$dry" = 1 ]; then mark mutation plan
elif gate_failed pkg_tests; then mark mutation skip; echo "not run: pkg_tests failed" > "$out/mutation.log"
elif [ "$path" = A ] && gate_failed registration; then mark mutation skip; echo "not run: registration test failed unmutated (see registration.log)" > "$out/mutation.log"
else
  progress "start mutation"
  t0=$(date +%s); : > "$out/mutation.log"
  tree_hash() { (cd "$wt" && { git diff; git ls-files -o --exclude-standard | sort; } | shasum -a 256 | cut -d' ' -f1); }
  if [ "$path" = A ]; then
    bf="$wt/comp/healthplatform/bundle.go"
    if [ ! -f "$bf" ] || ! grep -q "issues/$pkg\"" "$bf"; then
      echo "mutation: no blank import for $pkg in comp/healthplatform/bundle.go (Path A needs one)" >> "$out/mutation.log"
      add_mutant bundle-import MISSING - "no blank import in bundle.go"
    else
      before="$(tree_hash)"; cp "$bf" "$out/bundle.go.orig"
      perl -ni -e "print unless m{issues/$pkg\"}" "$bf"
      mcmd="$(named_test_cmd "$reg_test" "./comp/healthplatform")"
      (cd "$wt" && bash -c "$mcmd") > "$out/mutant_bundle.log" 2>&1; obs=$?
      cp "$out/bundle.go.orig" "$bf"
      after="$(tree_hash)"
      cat "$out/mutant_bundle.log" >> "$out/mutation.log"
      echo "MUTATION observed exit code: $obs (non-zero means the mutation was caught)" >> "$out/mutation.log"
      if [ "$before" != "$after" ]; then echo "mutation: tree not restored" >> "$out/mutation.log"; add_mutant bundle-import RESTORE_FAILED "$obs" "tree not restored"
      elif [ "$obs" = 0 ] && ! grep -Eq -- "--- (PASS|FAIL): $reg_test( |\$)" "$out/mutant_bundle.log"; then
        echo "mutation INCONCLUSIVE: test $reg_test never ran (name matched nothing; pass --reg-test or --render-json)" >> "$out/mutation.log"
        add_mutant bundle-import INCONCLUSIVE "$obs" "test $reg_test never ran"
      elif [ "$obs" != 0 ] && ! grep -Eq -- "--- FAIL: $reg_test( |\$)" "$out/mutant_bundle.log" && ! grep -Eq "$COMPILE_RE" "$out/mutant_bundle.log"; then
        echo "mutation INCONCLUSIVE: exit $obs but no '--- FAIL: $reg_test' line (test never ran or failed elsewhere)" >> "$out/mutation.log"
        add_mutant bundle-import INCONCLUSIVE "$obs" "no --- FAIL: $reg_test line"
      else
        st="$(classify "$out/mutant_bundle.log" "$obs")"
        [ "$st" = COMPILE_ERROR ] && echo "mutation COMPILE_ERROR: bundle-import (build failure is not a kill)" >> "$out/mutation.log"
        [ "$st" = SURVIVED ] && echo "mutation SURVIVED: registration test still passes without the bundle.go import" >> "$out/mutation.log"
        add_mutant bundle-import "$st" "$obs" "-"
      fi
    fi
  fi
  if [ "$path" = B ] && [ "${#mutation_cmds[@]}" = 0 ]; then
    echo "mutation: Path B needs --mutation-cmd (apply mutation, run test, restore); none given" >> "$out/mutation.log"
    add_mutant none NONE - "no --mutation-cmd given"
  fi
  i=0
  for mc in "${mutation_cmds[@]+"${mutation_cmds[@]}"}"; do
    mname="${mutation_names[$i]}"; i=$((i + 1))
    before="$(tree_hash)"
    (cd "$wt" && bash -c "$mc") > "$out/mutant_$i.log" 2>&1; obs=$?
    after="$(tree_hash)"
    cat "$out/mutant_$i.log" >> "$out/mutation.log"
    echo "MUTATION '$mc' observed exit code: $obs (non-zero means caught)" >> "$out/mutation.log"
    if [ "$before" != "$after" ]; then echo "mutation: command did not restore the tree" >> "$out/mutation.log"; add_mutant "$mname" RESTORE_FAILED "$obs" "command did not restore the tree"
    else
      st="$(classify "$out/mutant_$i.log" "$obs")"
      [ "$st" = SURVIVED ] && echo "mutation SURVIVED: '$mc'" >> "$out/mutation.log"
      [ "$st" = COMPILE_ERROR ] && echo "mutation COMPILE_ERROR: '$mc' (build failure is not a kill)" >> "$out/mutation.log"
      add_mutant "$mname" "$st" "$obs" "-"
    fi
  done
  { echo "MUTANT TABLE"; awk -F'\t' 'BEGIN{printf "%-28s %-14s %-5s %s\n","name","status","exit","note"} {printf "%-28s %-14s %-5s %s\n",$1,$2,$3,$4}' "$mut_tsv"; } | tee -a "$out/mutation.log" >&2
  echo $(( $(date +%s) - t0 )) > "$out/mutation.sec"
  if awk -F'\t' '$2!="KILLED"{bad=1} END{exit (NR==0||bad)}' "$mut_tsv"; then mark mutation pass 0; else mark mutation fail 1; fi
  progress "done mutation: $(cut -f2 "$mut_tsv" | sort | uniq -c | tr -s ' ' | tr '\n' ';')"
fi

# --- 4. baseline (only when a test gate failed, unless --baseline always) ---------------------------
declare_gate baseline "bash '$here/baseline_check.sh' '$wt' --targets '$test_targets' --clean '$clean' --log '$out/pkg_tests.log'"
if skipped baseline || [ "$baseline_mode" = never ]; then mark baseline skip
elif [ "$dry" = 1 ]; then mark baseline plan
elif [ "$baseline_mode" = always ] || gate_failed pkg_tests || gate_failed modified_tests; then
  run_gate baseline "n=0; while [ ! -f '$out/clean.ready' ] && [ ! -f '$out/clean.failed' ] && [ \$n -lt 900 ]; do sleep 2; n=\$((n+2)); done; [ -f '$out/clean.ready' ] || exit 2; bash '$here/baseline_check.sh' '$wt' --targets '$test_targets' --clean '$clean' --log '$out/pkg_tests.log'"
else mark baseline skip; echo "not needed: no test gate failed" > "$out/baseline.log"
fi

# --- join -------------------------------------------------------------------------------------------
for g in deps_delta baseline catalog; do wait_gate "$g"; done
[ -f "$out/prep_clean.pid" ] && wait "$(cat "$out/prep_clean.pid")" 2>/dev/null
remove_clean
trap - EXIT INT TERM

OUT="$out" SHA="$sha" PATHKIND="$path" PKG="$pkg" STYLE="$dda_style" REGTEST="$reg_test" REGSRC="$reg_source" DRY="$dry" \
  python3 -I - <<'PYEOF'
import json, os
out = os.environ["OUT"]
def rd(name, ext, default=""):
    try:
        return open(os.path.join(out, f"{name}.{ext}")).read().strip()
    except OSError:
        return default
gates = []
for name in [l.strip() for l in open(os.path.join(out, "gates.list")) if l.strip()]:
    status = rd(name, "status", "skip")
    gates.append({"name": name, "status": status, "rc": rd(name, "rc") or None,
                  "seconds": int(rd(name, "sec", "0") or 0) or None,
                  "log": os.path.join(out, name + ".log") if os.path.exists(os.path.join(out, name + ".log")) else None,
                  "cmd": rd(name, "cmd")})
for g in gates:
    if g["name"] == "mutation" and g["status"] in ("pass", "fail"):
        rows = []
        try:
            for l in open(os.path.join(out, "mutation.tsv")):
                f = l.rstrip("\n").split("\t")
                if len(f) >= 2 and f[0]:
                    rows.append({"name": f[0], "status": f[1], "rc": f[2] if len(f) > 2 and f[2] != "-" else None,
                                 "note": f[3] if len(f) > 3 and f[3] != "-" else None})
        except OSError:
            pass
        g["mutants"] = rows
        # the gate status is derived from the mutant table, never set independently
        g["status"] = "pass" if rows and all(r["status"] == "KILLED" for r in rows) else "fail"
failed = [g["name"] for g in gates if g["status"] == "fail"]
summary = {"ok": not failed and os.environ["DRY"] != "1", "dry_run": os.environ["DRY"] == "1", "sha": os.environ["SHA"],
           "path": os.environ["PATHKIND"], "pkg": os.environ["PKG"], "dda_test_style": os.environ["STYLE"],
           "registration_test": os.environ["REGTEST"], "registration_test_source": os.environ["REGSRC"], "failed": failed,
           "skipped": [g["name"] for g in gates if g["status"] == "skip"], "out_dir": out, "gates": gates,
           "notes": ["eslint was not run (catalog gate uses a no-op shim only for tsc); say so in the PR body"]}
text = json.dumps(summary, indent=2)
open(os.path.join(out, "summary.json"), "w").write(text + "\n")
print(text)
raise SystemExit(1 if failed else 0)
PYEOF

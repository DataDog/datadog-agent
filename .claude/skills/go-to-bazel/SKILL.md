---
name: go-to-bazel
description: Migrate gazelle:exclude lines from BUILD.bazel so that Go packages are managed by Bazel. Use when asked to migrate pkg/util, pkg/trace, pkg/tagger, or other Go packages to Bazel.
argument-hint: "[--prefix pkg/util/] [--dry-run] [--stop-on-failure]"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

Remove `# gazelle:exclude` lines from the root `BUILD.bazel` so that gazelle generates
`BUILD.bazel` files for those Go packages, bringing them under Bazel management.

## Background

The root `BUILD.bazel` contains many lines of the form:
```
# gazelle:exclude pkg/util/some/package
```

Each line tells gazelle to skip that directory. The goal is to remove these lines one by one,
letting gazelle generate `BUILD.bazel` files, then verifying the build still passes.

The automation tool lives at `tools/bazel_gazelle_migrate.py`.

## Running the tool

```bash
# Dry run — see what would be processed
python3 tools/bazel_gazelle_migrate.py --dry-run

# Run all pkg/util/ migrations (default prefix)
python3 tools/bazel_gazelle_migrate.py

# Run for a different package prefix
python3 tools/bazel_gazelle_migrate.py --prefix pkg/trace/
python3 tools/bazel_gazelle_migrate.py --prefix pkg/tagger/

# Stop on first failure (useful for debugging)
python3 tools/bazel_gazelle_migrate.py --stop-on-failure
```

The tool runs for a long time (hours for 75 packages). Run it in the background:
```bash
python3 tools/bazel_gazelle_migrate.py 2>&1 | tee /tmp/gazelle_migrate.log &
# or via Claude Code background task
```

Monitor progress:
```bash
grep -E "^\[|PASS|FAIL|SUMMARY|  \+" /tmp/gazelle_migrate.log
```

## What the tool does per package

For each `# gazelle:exclude <path>` matching the prefix:

1. **Full migration attempt**
   - Remove the exclude line from `BUILD.bazel`
   - Run `bazel run //:gazelle` (creates `BUILD.bazel` files for the whole subtree)
   - Run `bazel test //pkg/trace/... //pkg/tagger/... //pkg/util/...`
   - If tests pass → `git commit -a -m "migrate gazelle: <path>"`

2. **Targeted partial migration** (if full fails)
   - Parse the test/build output to identify which specific sub-packages failed
   - Add `# gazelle:exclude` lines for only those failing packages (not the whole subtree)
   - Re-run gazelle and tests
   - If tests pass → commit (only the genuinely broken sub-packages remain excluded)
   - If no specific failures can be parsed, falls back to excluding all subdirs that contain `.go` files

3. **Full revert** (if both fail)
   - Delete any newly created `BUILD.bazel` files
   - Revert any modifications to existing tracked `BUILD.bazel` files
   - Restore the original exclude line

## Reading the summary

```
SUMMARY: 63 succeeded, 16 failed

Succeeded:
  + pkg/util/maps
  + pkg/util/log  [shallow (sub-dirs still excluded)]

Failed:
  - pkg/util/dmi  (tests failed, no subdirs to try)
```

- Entries marked `[partial (N sub-dir(s) still excluded)]` were partially migrated —
  only the specific sub-directories that caused build failures were excluded. The rest
  of the subtree was fully migrated. The remaining excludes will be processed in a
  future run or manually.
- Failed entries need investigation (see below).

## After a run

Re-run the tool — entries that succeeded via shallow migration added new subdir excludes
which become candidates for the next run. Each pass typically migrates more packages.

## Nature of these changes

These migrations **only add `BUILD.bazel` files** — they never touch Go source code, test
logic, or product behavior. The only thing that can go wrong is that the generated
`BUILD.bazel` has incorrect build instructions (wrong deps, missing srcs, etc.).

This has two important consequences:

1. **A test being skipped/NO STATUS is a success signal.** When bazel analyzes a test
   target and decides to skip it (e.g. platform constraints, `manual` tag, size limits),
   that means the `BUILD.bazel` was correct and complete enough to be analyzed. Skipped ≠
   broken. A run where all tests either PASS or are skipped is fully acceptable.

2. **Real product regressions are impossible.** If CI finds a test failure on a package
   we just migrated, the failure is either:
   - A BUILD.bazel dep issue (FAILED TO BUILD) — fixable by editing the generated file
   - A pre-existing flaky test unrelated to our change
   - A platform-specific build constraint we need to add to the generated BUILD.bazel

   It is **not** possible for our change to alter the behavior of Go code.

## Investigating failures

For each failed package, look at what the actual build error is:

```bash
# Try manually:
# 1. Remove the exclude line from BUILD.bazel
# 2. Run gazelle
bazel run //:gazelle
# 3. Run tests and capture output
bazel test //pkg/trace/... //pkg/tagger/... //pkg/util/... 2>&1 | grep -E "ERROR|FAILED|error:"
# 4. Revert if needed
git checkout BUILD.bazel
# Delete any new BUILD.bazel files
```

Bazel test result meanings in this context:

| Result | Meaning | Action |
|--------|---------|--------|
| `PASSED` | Test ran and passed | ✅ Done |
| `(cached) PASSED` | Passed on a previous run, result reused | ✅ Done |
| `NO STATUS` / skipped | Bazel analyzed it but didn't run it (platform/tag/size) | ✅ Acceptable — BUILD.bazel is correct |
| `FAILED TO BUILD` | BUILD.bazel has wrong/missing deps | Fix the generated BUILD.bazel |
| `FAILED in X.Xs` | Test ran and failed | Investigate — likely pre-existing or platform issue |

Common failure categories:

### Blocked by unmigrated dependency
Output contains:
```
BLOCKED: migration requires package(s) outside this subtree that have no BUILD.bazel yet:
    pkg/util/fargate
Migrate those packages first, then retry.
```
The generated `BUILD.bazel` imports a package that is still gazelle-excluded (no `BUILD.bazel`).
The dependency is *outside* the subtree being migrated, so adding sub-directory excludes cannot
fix it.  The tool reverts automatically.

**Fix**: migrate the blocking package(s) first, then retry the original package.

### FAILED TO BUILD
Gazelle-generated `BUILD.bazel` has incorrect deps (missing or wrong external dep labels).
Fix: edit the generated `BUILD.bazel` to add the missing dep, then run tests manually.

### External module conflicts
The tool detects this automatically and reports:
```
BLOCKED: stale external Bazel module (@@gazelle++go_deps+ repo conflict)
This requires 'bazel sync' or MODULE.bazel updates — not a local package issue.
```
The underlying error looks like:
```
error loading package '@@gazelle++go_deps+com_github_datadog_...//': Unable to find
package for @@[unknown repo 'rules_go' ...]
```
The package is a separate Go module and the external Bazel repo for it is stale or has
dependency issues. Not fixable by adding more `gazelle:exclude` lines.

**Fix**: `bazel sync` or manual `MODULE.bazel` updates.

### Test failures (not build failures)
The package itself has a test that genuinely fails under Bazel. Look at the test log:
```
cat $(bazel info output_path)/darwin_arm64-fastbuild/testlogs/pkg/util/<name>/<name>_test/test.log
```

### Cascade failures
If many unrelated packages fail simultaneously, a previously-migrated package likely
introduced a broken dependency. Look for errors like:
```
ERROR: no such package 'pkg/util/log/slog': BUILD file not found
```
This means a `BUILD.bazel` file (possibly gitignored) references a package that has no
`BUILD.bazel`. Check for gitignored BUILD files:
```bash
find pkg/ -name "BUILD.bazel" | xargs git check-ignore 2>/dev/null
```
Delete any such files and re-run.

## Known pitfalls

### Gitignored BUILD.bazel files
Personal `~/.gitignore` patterns (e.g. `z*/`) can match package directories (e.g. `zap/`).
Gazelle creates `pkg/util/log/zap/BUILD.bazel`, `git status` doesn't show it (gitignored),
the cleanup step misses it, and it poisons every subsequent test run.

**Fix**: `find pkg/ -name "BUILD.bazel" | xargs git check-ignore` to find them, then delete.

The tool now uses `find` (not `git status`) to detect new BUILD.bazel files, which avoids
this problem going forward.

### Git index.lock races
If another git process (e.g. an IDE or a git worktree) holds the index lock, `git commit`
fails. The tool retries up to 3 times with a 3-second delay. If it still fails, the working
tree changes are left uncommitted — just run `git commit -a -m "migrate gazelle: <path>"`.

### Modified existing BUILD.bazel files
Gazelle sometimes modifies pre-existing `BUILD.bazel` files (e.g. updating deps in a sibling
package). The tool reverts these via `git checkout` on failure.

## Workflow for a new package prefix

```bash
# Check what's left
grep "gazelle:exclude pkg/trace/" BUILD.bazel | wc -l

# Run migrations
python3 tools/bazel_gazelle_migrate.py --prefix pkg/trace/

# After CI passes, run again to pick up newly-exposed subdir excludes
python3 tools/bazel_gazelle_migrate.py --prefix pkg/trace/
```

Repeat until no more excludes remain for that prefix, or until the remaining failures
all need manual investigation.

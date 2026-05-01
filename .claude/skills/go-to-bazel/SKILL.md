---
name: go-to-bazel
description: Migrate gazelle:exclude lines from BUILD.bazel so that Go packages are managed by Bazel. Use when asked to migrate Go packages to Bazel.
argument-hint: "[--package pkg/foo/bar] [--todo /tmp/TODO] [--no-commit] [--dry-run]"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

Remove `# gazelle:exclude` entries so that gazelle generates `BUILD.bazel` files for
Go packages, bringing them under Bazel management.
Run gazelle to generate BUILD.bazel files
Run tests to verify everything builds.

## Background

The root `BUILD.bazel` contains lines of the form:
```
# gazelle:exclude pkg/util/some/package
```

Each line tells gazelle to skip that directory. The goal is to remove these lines,
let gazelle generate `BUILD.bazel` files, and verify the build still passes.

Local `BUILD.bazel` files inside the tree may also contain relative
`# gazelle:exclude` directives that restrict which sub-directories gazelle enters.
`# gazelle:ignore` directives to specify that gazelle should ignore this directory, while still looking at sub-directories.

The automation tool lives at `tools/bazel_gazelle_migrate.py`.

## Running the tool

```bash
# Migrate one or more specific packages
python3 tools/bazel_gazelle_migrate.py --package pkg/serializer/mocks
python3 tools/bazel_gazelle_migrate.py --package pkg/foo pkg/bar pkg/baz

# Migrate packages listed in a TODO file (one path per line, optional [N] prefix)
python3 tools/bazel_gazelle_migrate.py --todo /tmp/TODO

# Both sources can be combined; paths from both are processed in order
python3 tools/bazel_gazelle_migrate.py --todo /tmp/TODO --package pkg/extra

# Dry run — see what would be done without making changes
python3 tools/bazel_gazelle_migrate.py --package pkg/foo --dry-run

# Skip auto-commit (review changes before committing manually)
python3 tools/bazel_gazelle_migrate.py --todo /tmp/TODO --no-commit

# Stop after N changed BUILD.bazel files (default: 50)
python3 tools/bazel_gazelle_migrate.py --todo /tmp/TODO --max-files 30

# Custom test targets
python3 tools/bazel_gazelle_migrate.py --package pkg/foo \
  --test-targets //cmd/... //comp/... //pkg/...
```

Run in the background for large TODO lists:
```bash
python3 tools/bazel_gazelle_migrate.py --todo /tmp/TODO 2>&1 | tee /tmp/migrate.log &
```

Monitor progress:
```bash
grep -E "^(>>>|  (PASS|FAIL)|SUMMARY|Changed)" /tmp/migrate.log
bazel test //cmd/... //comp/... //pkg/...
```

## Three migration modes

The tool classifies each target path and selects the appropriate strategy:

### 1. DIRECT — root BUILD.bazel has an exact exclude line

```
>>> DIRECT: pkg/discovery/tracermetadata/language
  removed # gazelle:exclude pkg/discovery/tracermetadata/language from BUILD.bazel
  gazelle created 1 file(s)
  PASS
```

- Removes the exact `# gazelle:exclude <path>` line from the root `BUILD.bazel`.
- Runs `bazel run //:gazelle` then `bazel test`.
- On failure: restores the exclude line, deletes any new BUILD.bazel files.

### 2. CARVEOUT — a parent directory is excluded in the root BUILD.bazel

```
>>> CARVEOUT: pkg/collector/corechecks  targets=['pkg/collector/corechecks/snmp/internal/common', ...]
  removed # gazelle:exclude pkg/collector/corechecks (and children) from BUILD.bazel
  added 36 child excludes to BUILD.bazel
  gazelle created 7 file(s)
  PASS  pkg/collector/corechecks/snmp/internal/common
```

- Removes the parent exclude (and any child entries) from the root `BUILD.bazel`.
- Adds `# gazelle:exclude <child>` lines for all non-target subdirs directly to
  the root `BUILD.bazel` (minimal covering set — one line per subtree root,
  so gazelle only descends into the target directories).
- Runs gazelle + tests.
- On failure: restores the root `BUILD.bazel` to its pre-operation state exactly,
  deletes any gazelle-generated BUILD.bazel files.

**Key invariant**: all `# gazelle:exclude` directives live in the root `BUILD.bazel`.
No local BUILD.bazel files are created for subdirectory excludes.

### 3. LOCAL DIRECT — a local BUILD.bazel has the exclude

```
>>> LOCAL DIRECT: pkg/serializer/mocks  (from pkg/serializer/BUILD.bazel: exclude mocks)
  removed # gazelle:exclude mocks from pkg/serializer/BUILD.bazel
  gazelle created 1 file(s)
  PASS
```

- The tool walks up the ancestor directories looking for a local `BUILD.bazel` that
  contains a `# gazelle:exclude <rel>` directive covering the target.
- Removes just that line, runs gazelle + tests.
- On failure: restores the line to the local `BUILD.bazel`.

## Failure modes

### Blocked by unmigrated dependency

```
FAIL: blocked by: comp/core/autodiscovery/integration, pkg/aggregator/sender
```

The generated BUILD.bazel imports a package that has no BUILD.bazel yet. This is
outside the subtree being migrated, so the tool cannot fix it by adjusting excludes.

**Fix**: find and migrate the blocking packages first, then retry.

To locate where a blocker is excluded:
```bash
# Check root BUILD.bazel
grep "gazelle:exclude.*pkg/aggregator/sender" BUILD.bazel

# Check local BUILD.bazel files (if not in root)
grep -r "gazelle:exclude" --include="BUILD.bazel" | grep "sender"
```

Note: a blocker can be excluded via the root BUILD.bazel OR via a local
`BUILD.bazel` with a relative exclude. Check both.

### External module conflict

```
FAIL: external module conflict
```

The package is a separate Go module whose external Bazel repo is stale.
Requires `bazel sync` or `MODULE.bazel` updates — not fixable by adding excludes.

### Tests failed

The package builds but its tests genuinely fail under Bazel. Usually a pre-existing
issue or platform-specific constraint. Investigate the test log:
```bash
cat $(bazel info output_path)/darwin_arm64-fastbuild/testlogs/<path>/<name>_test/test.log
```

### No matching exclude found

```
FAIL: no matching exclude found
```

The target path is not in the root BUILD.bazel excludes and no ancestor is either.
Possible causes:
- Already migrated (BUILD.bazel already exists).
- Excluded via a local BUILD.bazel but the tool couldn't find it (check with grep).
- Listed in the TODO but never needed a migration.

## Reading results

```
SUMMARY: 17 succeeded, 9 failed
Succeeded:
  + pkg/discovery/tracermetadata/language
  + pkg/serializer/mocks
Failed:
  - pkg/collector/corechecks/snmp/internal/common  (blocked by: pkg/aggregator/sender)
  - pkg/trace/event  (external module conflict)

Changed BUILD.bazel files: 44
```

The tool stops automatically when the changed file count reaches `--max-files` (default 50)
to keep PR sizes manageable. Run again after committing to continue.

## After a run

Check what changed:
```bash
git status --porcelain | grep BUILD.bazel
git diff BUILD.bazel        # root exclude removals
git diff --stat HEAD        # all new local BUILD.bazel files
```

Then commit if not using auto-commit:
```bash
git add -A && git commit -m "migrate gazelle: <description>"
```

Re-run the tool with a fresh TODO to pick up newly-exposed packages:
```bash
python3 tools/bazel_gazelle_migrate.py --todo /tmp/TODO
```

## Nature of these changes

Migrations only add `BUILD.bazel` files — they never touch Go source code, test
logic, or product behavior.

- **Skipped/NO STATUS tests are a success signal.** Gazelle's BUILD.bazel was
  correct enough to be analyzed. Skipped ≠ broken.
- **Real product regressions are impossible.** Any CI failure on a newly-migrated
  package is a BUILD.bazel dep issue (fixable) or a pre-existing flaky test, never
  a behavior change.

## Known pitfalls

### Over-revert bug (fixed)
The tool snapshots which BUILD.bazel files are already modified before each
operation and only reverts files newly modified in the current operation. This
prevents a failed migration from undoing changes made by prior successful migrations
in the same session.

### Gitignored BUILD.bazel files
Personal `~/.gitignore` patterns can silently hide new BUILD.bazel files from
`git status`. The tool uses `rglob` (not git) to detect new files, avoiding this.
To check for hidden files: `find pkg/ -name "BUILD.bazel" | xargs git check-ignore`

### Git index.lock races
If another git process holds the lock, `git commit` retries up to 3 times with a
3-second delay. If it still fails, changes are left uncommitted — commit manually.

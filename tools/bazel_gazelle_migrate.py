#!/usr/bin/env python3
"""
Automates removing `# gazelle:exclude <path>` lines from BUILD.bazel one at a time,
running gazelle + bazel tests, and committing or reverting each change.

When a full migration fails, falls back to a "shallow" migration: adds
gazelle:exclude lines for all sub-directories so only the top-level
BUILD.bazel is created, then retries. If that also fails, fully reverts.

Usage:
    python3 tools/bazel_gazelle_migrate.py [options]

Options:
    --prefix PREFIX     Only process excludes matching this prefix (default: pkg/util/)
    --dry-run           Print what would happen without executing anything
    --stop-on-failure   Halt on first test failure (default: continue to next path)
    --test-targets      Space-separated bazel test targets
                        (default: //pkg/trace/... //pkg/tagger/... //pkg/util/...)
"""

import argparse
import re
import subprocess
import sys
from pathlib import Path

# REPO_ROOT = Path(__file__).resolve().parent.parent
REPO_ROOT = Path(".")
BUILD_BAZEL = REPO_ROOT / "BUILD.bazel"
DEFAULT_TEST_TARGETS = ["//pkg/trace/...", "//pkg/tagger/...", "//pkg/util/..."]


def log(msg: str) -> None:
    print(msg, flush=True)


def run(cmd: list[str], *, check: bool = True, capture: bool = False) -> subprocess.CompletedProcess:
    kwargs: dict = dict(cwd=REPO_ROOT)
    if capture:
        kwargs["capture_output"] = True
        kwargs["text"] = True
    return subprocess.run(cmd, check=check, **kwargs)


def get_exclude_lines(prefix: str) -> list[tuple[int, str]]:
    """Return list of (line_number_0based, path) for matching gazelle:exclude lines."""
    pattern = re.compile(r"^# gazelle:exclude\s+(.+)$")
    results = []
    lines = BUILD_BAZEL.read_text().splitlines()
    for i, line in enumerate(lines):
        m = pattern.match(line.strip())
        if m and m.group(1).startswith(prefix):
            results.append((i, m.group(1).strip()))
    return results


def current_excludes() -> set[str]:
    """Return the set of all currently excluded paths."""
    pattern = re.compile(r"^# gazelle:exclude\s+(.+)$")
    result = set()
    for line in BUILD_BAZEL.read_text().splitlines():
        m = pattern.match(line.strip())
        if m:
            result.add(m.group(1).strip())
    return result


def remove_exclude_line(path: str) -> None:
    """Remove the gazelle:exclude line for the given path from BUILD.bazel."""
    text = BUILD_BAZEL.read_text()
    target_line = f"# gazelle:exclude {path}"
    lines = text.splitlines(keepends=True)
    new_lines = [l for l in lines if l.rstrip() != target_line]
    if len(new_lines) == len(lines):
        raise ValueError(f"Line not found in BUILD.bazel: {target_line!r}")
    BUILD_BAZEL.write_text("".join(new_lines))


def add_exclude_line(path: str) -> None:
    """Insert a gazelle:exclude line for the given path, keeping the block sorted."""
    if path in current_excludes():
        return  # already present
    lines = BUILD_BAZEL.read_text().splitlines(keepends=True)
    target_line = f"# gazelle:exclude {path}\n"
    pattern = re.compile(r"^# gazelle:exclude\s+(.+)$")

    insert_at = None
    last_exclude = None
    for i, line in enumerate(lines):
        m = pattern.match(line.strip())
        if m:
            last_exclude = i
            if m.group(1).strip() > path and insert_at is None:
                insert_at = i

    if insert_at is not None:
        lines.insert(insert_at, target_line)
    elif last_exclude is not None:
        lines.insert(last_exclude + 1, target_line)
    else:
        lines.append(target_line)

    BUILD_BAZEL.write_text("".join(lines))


def restore_exclude_line(path: str) -> None:
    """Re-insert the gazelle:exclude line for the given path, keeping the list sorted."""
    add_exclude_line(path)


def remove_added_exclude_lines(paths: list[str]) -> None:
    """Remove a set of previously-added exclude lines from BUILD.bazel."""
    text = BUILD_BAZEL.read_text()
    target_lines = {f"# gazelle:exclude {p}" for p in paths}
    lines = text.splitlines(keepends=True)
    BUILD_BAZEL.write_text("".join(l for l in lines if l.rstrip() not in target_lines))


def get_subdirs(path: str) -> list[str]:
    """Return relative paths of sub-directories under path that contain .go files.

    We only need a gazelle:exclude for a sub-directory when gazelle would actually
    generate a BUILD.bazel there — and gazelle only does that when .go files are
    present. Excluding dirs without .go files is unnecessary noise.
    """
    abs_path = REPO_ROOT / path
    if not abs_path.is_dir():
        return []
    result = []
    for entry in sorted(abs_path.rglob("*")):
        if entry.is_dir() and any(entry.glob("*.go")):
            result.append(str(entry.relative_to(REPO_ROOT)))
    return result


def find_all_build_files() -> set[Path]:
    """Return the set of all BUILD.bazel files currently on disk under REPO_ROOT."""
    return set(REPO_ROOT.rglob("BUILD.bazel"))


def get_new_build_files(before: set[Path]) -> list[Path]:
    """Return BUILD.bazel files that exist now but were not in `before`.

    Using find-based diffing rather than git-status so that gitignored files
    (e.g. those matched by personal ~/.gitignore patterns) are not missed.
    """
    after = find_all_build_files()
    return sorted(after - before)


def revert_gazelle_changes(new_files: list[Path]) -> None:
    """Delete newly created BUILD.bazel files and revert any modifications gazelle
    made to existing tracked files (e.g. updating deps in a sibling BUILD.bazel)."""
    for f in new_files:
        if f.exists():
            f.unlink()
            log(f"  deleted {f.relative_to(REPO_ROOT)}")
    # Revert modifications to any tracked BUILD.bazel files
    result = run(["git", "diff", "--name-only"], capture=True)
    modified = [p.strip() for p in result.stdout.splitlines() if p.strip().endswith("BUILD.bazel")]
    if modified:
        run(["git", "checkout", "--"] + modified)
        for p in modified:
            log(f"  reverted {p}")


# Keep a simple alias used by older call sites
def delete_files(files: list[Path]) -> None:
    revert_gazelle_changes(files)


def run_gazelle() -> bool:
    """Run gazelle; return True on success."""
    log("  running: bazel run //:gazelle")
    result = run(["bazel", "run", "//:gazelle"], check=False)
    if result.returncode != 0:
        log(f"  FAIL: gazelle exited with code {result.returncode}")
        return False
    return True


def run_tests(targets: list[str]) -> tuple[bool, str]:
    """Run bazel tests; return (passed, combined_output)."""
    log(f"  running: bazel test {' '.join(targets)}")
    result = subprocess.run(
        ["bazel", "test"] + targets,
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
    )
    output = result.stdout + result.stderr
    # Echo output so the log file stays useful
    print(output, end="", flush=True)
    if result.returncode != 0:
        log(f"  FAIL: tests exited with code {result.returncode}")
        return False, output
    log("  PASS: tests succeeded")
    return True, output


def failing_packages_from_output(output: str, under_path: str | None = None) -> list[str]:
    """Parse bazel test/build output and return failing package paths.

    If *under_path* is given, only packages that are children of (or equal to)
    that path are returned — i.e. packages whose failures we might be able to
    address by adding sub-directory excludes.

    Pass ``None`` (the default) to return *all* failing packages regardless of
    location; this is useful for detecting failures caused by packages outside
    the subtree currently being migrated.
    """
    under_prefix = (under_path.rstrip("/") + "/") if under_path else None
    found: set[str] = set()

    # Patterns that name a specific failing package path.
    # All repo-relative package paths start with a lowercase letter (never '/'),
    # so we use [a-z] as the first-character anchor instead of hard-coding 'pkg/'
    # — this lets us catch failures in comp/, cmd/, tasks/, etc. as well.
    #
    # The BUILD-file path pattern uses a negative lookbehind (?<![/\w]) to
    # prevent it from anchoring mid-string inside an absolute OS path like
    # /Users/tony/ws/repo/pkg/foo/BUILD.bazel (which would otherwise match
    # from 'sers/...' because .*? is non-greedy).
    #
    #   ERROR: pkg/util/foo/BUILD.bazel:3:11: ...
    #   ERROR: no such package 'comp/core/log/impl': ...
    #   GoCompilePkg comp/core/log/fx/foo.a failed
    #   //comp/core/log/fx:fx_test  FAILED TO BUILD
    patterns = [
        re.compile(r"ERROR:.*?(?<![/\w])([a-z][^\s'\":]+/BUILD\.bazel)"),  # path to a BUILD file
        re.compile(r"no such package '([a-z][^']+)'"),  # missing package
        re.compile(r"GoCompilePkg\s+([a-z][^\s]+)\.a\s+failed"),  # compile failure
        re.compile(r"//([^:]+):[^\s]+\s+FAILED"),  # bazel target
    ]

    for line in output.splitlines():
        for pat in patterns:
            m = pat.search(line)
            if m:
                raw = m.group(1)
                # Normalise: strip trailing /BUILD.bazel
                pkg_path = raw.removesuffix("/BUILD.bazel").removesuffix("BUILD.bazel").rstrip("/")
                if under_prefix is None or pkg_path.startswith(under_prefix) or pkg_path == under_path:
                    found.add(pkg_path)

    return sorted(found)


def add(path: str, new_files: list[Path]) -> None:
    for f in new_files:
        run(["git", "add", str(f)])


def commit(path: str, new_files: list[Path]) -> None:
    msg = f"migrate gazelle: {path}\n\nCo-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
    # Retry once on index.lock races (can occur when a worktree in another
    # directory holds the lock briefly).
    for attempt in range(3):
        result = subprocess.run(
            ["git", "commit", "-a", "-m", msg],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
        )
        if result.returncode == 0:
            break
        if "index.lock" in result.stderr and attempt < 2:
            import time

            log("  git index.lock conflict, retrying in 3s...")
            time.sleep(3)
        else:
            raise subprocess.CalledProcessError(result.returncode, ["git", "commit"], result.stdout, result.stderr)
    log(f"  committed: migrate gazelle: {path}")


def try_migrate(path: str, test_targets: list[str]) -> tuple[bool, str]:
    """
    Attempt to migrate a single gazelle:exclude path.

    Strategy:
      1. Full migration: remove the exclude, run gazelle (creates BUILD files for
         the whole subtree), run tests.
      2. If that fails, shallow migration: add excludes for all sub-directories,
         re-run gazelle (creates only the top-level BUILD.bazel), run tests.
      3. If that also fails, fully revert everything.

    Returns (success, reason_string).
    """
    existing_excludes = current_excludes()
    build_files_before = find_all_build_files()

    # --- Step 1: remove the exclude line ---
    remove_exclude_line(path)
    log("  removed line from BUILD.bazel")

    # --- Step 2: run gazelle ---
    if not run_gazelle():
        new_files = get_new_build_files(build_files_before)
        delete_files(new_files)
        restore_exclude_line(path)
        return False, "gazelle failed"

    new_files = get_new_build_files(build_files_before)
    if new_files:
        log(f"  gazelle created {len(new_files)} file(s):")
        for f in new_files:
            log(f"    {f.relative_to(REPO_ROOT)}")
    else:
        log("  gazelle created no new BUILD.bazel files")

    # --- Step 3: run tests ---
    passed, test_output = run_tests(test_targets)
    if passed:
        add(path, new_files)
        # commit(path, new_files)
        return True, ""

    # --- Full migration failed; retry with targeted excludes ---
    # Parse the output to find which specific sub-packages caused failures,
    # then exclude only those rather than the entire subtree.
    log("  full migration failed; identifying failing sub-packages to exclude")
    delete_files(new_files)

    # Check for stale external Bazel module errors (@@gazelle++go_deps+... repos
    # that reference an unknown @rules_go).  These are not fixable by adding
    # more gazelle:exclude lines — they require `bazel sync` or MODULE.bazel
    # updates.  Detect them early so we don't emit a misleading BLOCKED message.
    if re.search(r"@@\[unknown repo|error loading package '@@", test_output):
        restore_exclude_line(path)
        log("  BLOCKED: stale external Bazel module (@@gazelle++go_deps+ repo conflict)")
        log("  This requires 'bazel sync' or MODULE.bazel updates — not a local package issue.")
        return False, "external module conflict (stale gazelle dep repo)"

    # Before attempting partial migration, check whether the failure is caused
    # by packages *outside* the subtree we're migrating.  Those packages have
    # no BUILD.bazel yet (still gazelle-excluded) and adding sub-dir excludes
    # inside our package cannot fix that.  Fail fast with a clear message.
    all_failing = failing_packages_from_output(test_output)
    unrelated = [
        p for p in all_failing
        if p != path and not p.startswith(path.rstrip("/") + "/")
    ]
    if unrelated:
        restore_exclude_line(path)
        log("  BLOCKED: migration requires package(s) outside this subtree that have no BUILD.bazel yet:")
        for p in unrelated:
            log(f"    {p}")
        log("  Migrate those packages first, then retry.")
        return False, f"blocked by unmigrated dependency: {', '.join(unrelated)}"

    targeted = failing_packages_from_output(test_output, path)
    all_subdirs_with_go = get_subdirs(path)  # only dirs that have .go files

    # Fall back to all subdirs if we couldn't identify specific failures
    # (e.g. the failure is at the top-level package itself or a parse miss).
    if not targeted:
        targeted = all_subdirs_with_go
        log("  could not identify specific failing packages; excluding all subdirs with .go files")
    else:
        log(f"  identified {len(targeted)} failing sub-package(s):")
        for t in targeted:
            log(f"    {t}")

    newly_added_excludes = [d for d in targeted if d not in existing_excludes]

    if not newly_added_excludes:
        log("  no new sub-directories to exclude; reverting")
        restore_exclude_line(path)
        return False, "tests failed, nothing to exclude"

    for d in newly_added_excludes:
        add_exclude_line(d)

    # Re-run gazelle with the targeted excludes in place
    build_files_before_retry = find_all_build_files()
    if not run_gazelle():
        remove_added_exclude_lines(newly_added_excludes)
        restore_exclude_line(path)
        return False, "gazelle failed on targeted retry"

    new_files = get_new_build_files(build_files_before_retry)
    if new_files:
        log(f"  gazelle created {len(new_files)} file(s) (targeted):")
        for f in new_files:
            log(f"    {f.relative_to(REPO_ROOT)}")
    else:
        log("  gazelle created no new BUILD.bazel files (targeted)")

    passed, _ = run_tests(test_targets)
    if passed:
        excluded_note = ", ".join(newly_added_excludes)
        commit(path, new_files)
        return True, f"partial ({len(newly_added_excludes)} sub-dir(s) still excluded)"

    # --- Targeted retry also failed; full revert ---
    log("  targeted retry also failed; reverting everything")
    delete_files(new_files)
    remove_added_exclude_lines(newly_added_excludes)
    restore_exclude_line(path)
    return False, "tests failed even after targeted sub-dir excludes"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--prefix", default="pkg/util/", help="Path prefix to filter excludes (default: pkg/util/)")
    parser.add_argument("--dry-run", action="store_true", help="Print actions without executing them")
    parser.add_argument("--stop-on-failure", action="store_true", help="Stop on first test failure")
    parser.add_argument(
        "--test-targets",
        nargs="+",
        default=DEFAULT_TEST_TARGETS,
        metavar="TARGET",
        help="Bazel test targets to run after each migration",
    )
    args = parser.parse_args()

    excludes = get_exclude_lines(args.prefix)
    if not excludes:
        log(f"No gazelle:exclude lines found with prefix {args.prefix!r}")
        return 0

    log(f"Found {len(excludes)} exclude(s) matching prefix {args.prefix!r}")
    for _, path in excludes:
        log(f"  {path}")
    log("")

    succeeded = []
    failed = []

    for idx, (_, path) in enumerate(excludes):
        # Re-read the list each iteration: the path may already be gone if it
        # was a sub-directory that was added as an exclude during a previous
        # shallow migration.
        if path not in current_excludes():
            log(f"[{idx + 1}/{len(excludes)}] Skipping {path} (already removed as part of a parent migration)")
            log("")
            continue

        log(f"[{idx + 1}/{len(excludes)}] Processing: {path}")

        if args.dry_run:
            log("  [dry-run] would attempt full migration, then targeted partial migration")
            log("")
            continue

        success, reason = try_migrate(path, args.test_targets)
        if success:
            succeeded.append((path, reason))
        else:
            failed.append((path, reason))
            if args.stop_on_failure:
                break

        log("")

    # Summary
    log("=" * 60)
    log(f"SUMMARY: {len(succeeded)} succeeded, {len(failed)} failed")
    if succeeded:
        log("\nSucceeded:")
        for p, note in succeeded:
            suffix = f"  [{note}]" if note else ""
            log(f"  + {p}{suffix}")
    if failed:
        log("\nFailed:")
        for p, reason in failed:
            log(f"  - {p}  ({reason})")

    return 0 if not failed else 1


if __name__ == "__main__":
    sys.exit(main())

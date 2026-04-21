#!/usr/bin/env python3
"""
Automates migrating Go packages to Bazel management.

Reads target paths from a TODO file (one per line, optional [N] prefix).
For each target:

  DIRECT: path has its own # gazelle:exclude line in root BUILD.bazel.
    → Remove it, run gazelle + tests. Revert on failure.

  CARVEOUT: a parent path is excluded in root BUILD.bazel.
    → Remove the parent exclude. Create <parent>/BUILD.bazel with
      # gazelle:ignore + # gazelle:exclude <rel> for non-target subdirs
      (minimal covering set so gazelle descends only into targets).
      Run gazelle + tests. Revert all changes on failure.

Only REMOVES lines from root BUILD.bazel — never adds new ones.
Local BUILD.bazel files handle subdirectory excludes for carveouts.
Stops when total changed BUILD.bazel file count reaches --max-files.

Usage:
    python3 tools/bazel_gazelle_migrate.py --todo /tmp/TODO [options]
    python3 tools/bazel_gazelle_migrate.py --package pkg/foo/bar [pkg/baz ...] [options]
    python3 tools/bazel_gazelle_migrate.py --todo /tmp/TODO --package pkg/extra [options]

Options:
    --todo FILE         Path to TODO file listing target package paths
    --package PKG+      One or more package paths to migrate directly
    --max-files N       Stop when N BUILD.bazel files have changed (default: 50)
    --dry-run           Print what would happen without executing
    --no-commit         Don't commit after each success
    --test-targets T+   Bazel test targets (default: //cmd/... //comp/... //pkg/...)
"""

import argparse
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(".")
BUILD_BAZEL = REPO_ROOT / "BUILD.bazel"
DEFAULT_TEST_TARGETS = ["//cmd/...", "//comp/...", "//pkg/..."]
DEFAULT_MAX_FILES = 50


def log(msg: str) -> None:
    print(msg, flush=True)


def run(cmd: list[str], *, check: bool = False, capture: bool = False) -> subprocess.CompletedProcess:
    kwargs: dict = {"cwd": REPO_ROOT}
    if capture:
        kwargs["capture_output"] = True
        kwargs["text"] = True
    return subprocess.run(cmd, check=check, **kwargs)


# ── root BUILD.bazel management ───────────────────────────────────────────────

def current_excludes() -> set[str]:
    """Return the set of all currently excluded paths from root BUILD.bazel."""
    pattern = re.compile(r"^# gazelle:exclude\s+(.+)$")
    result = set()
    for line in BUILD_BAZEL.read_text().splitlines():
        m = pattern.match(line.strip())
        if m:
            result.add(m.group(1).strip())
    return result


def remove_all_excludes_under(prefix: str) -> None:
    """Remove all # gazelle:exclude lines for prefix or any path under it."""
    text = BUILD_BAZEL.read_text()
    lines = []
    for line in text.splitlines(keepends=True):
        s = line.strip()
        if s.startswith("# gazelle:exclude "):
            excluded = s.removeprefix("# gazelle:exclude ").strip()
            if excluded == prefix or excluded.startswith(prefix + "/"):
                continue
        lines.append(line)
    BUILD_BAZEL.write_text("".join(lines))


def restore_exclude(path: str) -> None:
    """Re-add a # gazelle:exclude line in sorted order."""
    if path in current_excludes():
        return
    lines = BUILD_BAZEL.read_text().splitlines(keepends=True)
    target = f"# gazelle:exclude {path}\n"
    pat = re.compile(r"^# gazelle:exclude\s+(.+)$")
    insert_at = last = None
    for i, line in enumerate(lines):
        m = pat.match(line.strip())
        if m:
            last = i
            if m.group(1).strip() > path and insert_at is None:
                insert_at = i
    if insert_at is not None:
        lines.insert(insert_at, target)
    elif last is not None:
        lines.insert(last + 1, target)
    else:
        lines.append(target)
    BUILD_BAZEL.write_text("".join(lines))


# ── local BUILD.bazel exclude management ─────────────────────────────────────

_EXCLUDE_PAT = re.compile(r"^# gazelle:exclude\s+(.+)$")


def find_local_exclude(path: str) -> tuple[str, str] | None:
    """Search ancestor directories for a local BUILD.bazel that excludes path.

    Returns (ancestor_dir, relative_exclude) where ancestor_dir is the
    repo-relative directory containing the BUILD.bazel and relative_exclude is
    the value after '# gazelle:exclude' in that file.  Returns None if not found.
    """
    parts = path.split("/")
    for depth in range(len(parts) - 1, 0, -1):
        ancestor = "/".join(parts[:depth])
        local_build = REPO_ROOT / ancestor / "BUILD.bazel"
        if not local_build.exists():
            continue
        rel_target = "/".join(parts[depth:])
        for line in local_build.read_text().splitlines():
            m = _EXCLUDE_PAT.match(line.strip())
            if m:
                excl = m.group(1).strip()
                if excl == rel_target or rel_target.startswith(excl + "/"):
                    return ancestor, excl
    return None


def remove_local_exclude(build_dir: str, rel_excl: str) -> None:
    """Remove a # gazelle:exclude line from a local BUILD.bazel."""
    build_file = REPO_ROOT / build_dir / "BUILD.bazel"
    target = f"# gazelle:exclude {rel_excl}"
    lines = build_file.read_text().splitlines(keepends=True)
    new_lines = [l for l in lines if l.rstrip() != target]
    build_file.write_text("".join(new_lines))


def restore_local_exclude(build_dir: str, rel_excl: str) -> None:
    """Re-add a # gazelle:exclude line to a local BUILD.bazel in sorted order."""
    build_file = REPO_ROOT / build_dir / "BUILD.bazel"
    text = build_file.read_text()
    # Already present?
    if f"# gazelle:exclude {rel_excl}" in text:
        return
    lines = text.splitlines(keepends=True)
    target = f"# gazelle:exclude {rel_excl}\n"
    insert_at = last = None
    for i, line in enumerate(lines):
        m = _EXCLUDE_PAT.match(line.strip())
        if m:
            last = i
            if m.group(1).strip() > rel_excl and insert_at is None:
                insert_at = i
    if insert_at is not None:
        lines.insert(insert_at, target)
    elif last is not None:
        lines.insert(last + 1, target)
    else:
        lines.append(target)
    build_file.write_text("".join(lines))


# ── file tracking ─────────────────────────────────────────────────────────────

def find_all_build_files() -> set[Path]:
    """Return the set of all BUILD.bazel files currently on disk."""
    return set(REPO_ROOT.rglob("BUILD.bazel"))


def get_new_build_files(before: set[Path]) -> list[Path]:
    """BUILD.bazel files that appeared since the `before` snapshot."""
    after = find_all_build_files()
    return sorted(after - before)


def get_modified_tracked_build_files() -> set[str]:
    """Repo-relative paths of tracked BUILD.bazel files with uncommitted changes."""
    result = run(["git", "diff", "--name-only"], capture=True)
    return {p.strip() for p in result.stdout.splitlines() if p.strip().endswith("BUILD.bazel")}


def changed_build_file_count() -> int:
    """Count BUILD.bazel files that differ from HEAD (new or modified)."""
    result = run(["git", "status", "--porcelain"], capture=True)
    count = 0
    for line in result.stdout.splitlines():
        parts = line.strip().split(None, 1)
        if len(parts) == 2 and parts[1].endswith("BUILD.bazel"):
            count += 1
    return count


def revert_gazelle_changes(
    new_files: list[Path],
    pre_existing_mods: set[str],
) -> None:
    """Delete newly created BUILD.bazel files and revert modifications to tracked
    BUILD.bazel files introduced by this operation only.

    pre_existing_mods: set of repo-relative BUILD.bazel paths that were already
    modified before this operation started — those are NOT reverted here (they
    belong to prior successful migrations and must be preserved).

    Root BUILD.bazel is never touched here; callers manage it explicitly via
    restore_exclude().
    """
    for f in new_files:
        if f.exists():
            f.unlink()
            log(f"  deleted {f.relative_to(REPO_ROOT)}")

    result = run(["git", "diff", "--name-only"], capture=True)
    to_revert = [
        p.strip() for p in result.stdout.splitlines()
        if (p.strip().endswith("BUILD.bazel")
            and p.strip() != "BUILD.bazel"          # root managed by caller
            and p.strip() not in pre_existing_mods)  # skip prior successful ops
    ]
    if to_revert:
        run(["git", "checkout", "--"] + to_revert)
        for p in to_revert:
            log(f"  reverted {p}")


# ── minimal covering set ──────────────────────────────────────────────────────

def _collect_excludes(prefix: str, abs_dir: Path, targets_rel: set[str], result: list[str]) -> None:
    """Recursively collect the minimal set of relative paths to exclude.

    Descends into directories that are targets or ancestors of targets;
    appends everything else as a single exclude entry (one line covers the
    whole subtree).
    """
    try:
        entries = sorted(abs_dir.iterdir())
    except PermissionError:
        return
    for entry in entries:
        if not entry.is_dir():
            continue
        rel = f"{prefix}/{entry.name}" if prefix else entry.name
        needed = any(t == rel or t.startswith(rel + "/") for t in targets_rel)
        if needed:
            _collect_excludes(rel, entry, targets_rel, result)
        else:
            result.append(rel)


def get_minimal_excludes(parent_abs: Path, targets_rel: set[str]) -> list[str]:
    """Return the minimal sorted list of relative-to-parent dirs to exclude."""
    result: list[str] = []
    _collect_excludes("", parent_abs, targets_rel, result)
    return sorted(result)


# ── gazelle + tests ───────────────────────────────────────────────────────────

def run_gazelle() -> bool:
    """Run gazelle; return True on success."""
    log("  running: bazel run //:gazelle")
    result = run(["bazel", "run", "//:gazelle"])
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
    print(output, end="", flush=True)
    if result.returncode != 0:
        log(f"  FAIL: tests exited with code {result.returncode}")
        return False, output
    log("  PASS: tests succeeded")
    return True, output


_REPO_TOP_DIRS = {"pkg", "comp", "cmd", "tasks", "test", "tools", "bazel", "internal"}


def failing_packages_from_output(output: str) -> list[str]:
    """Parse bazel output; return repo-relative failing package paths.

    Filters to paths whose first component is a known top-level repo directory
    to avoid false positives from absolute OS paths in error messages.
    """
    patterns = [
        re.compile(r"ERROR:.*?(?<![/\w])([a-z][^\s'\":]+/BUILD\.bazel)"),
        re.compile(r"no such package '([a-z][^']+)'"),
        re.compile(r"GoCompilePkg\s+([a-z][^\s]+)\.a\s+failed"),
        re.compile(r"//([^:]+):[^\s]+\s+FAILED"),
    ]
    found: set[str] = set()
    for line in output.splitlines():
        for pat in patterns:
            m = pat.search(line)
            if m:
                raw = m.group(1)
                pkg = raw.removesuffix("/BUILD.bazel").removesuffix("BUILD.bazel").rstrip("/")
                top = pkg.split("/")[0]
                if top in _REPO_TOP_DIRS:
                    found.add(pkg)
    return sorted(found)


# ── per-entry migration ───────────────────────────────────────────────────────

def migrate_direct(path: str, test_targets: list[str]) -> tuple[bool, str]:
    """Remove the exact exclude line from root BUILD.bazel, run gazelle + tests."""
    pre_mods = get_modified_tracked_build_files()
    before = find_all_build_files()

    remove_all_excludes_under(path)
    log(f"  removed # gazelle:exclude {path} from BUILD.bazel")

    if not run_gazelle():
        revert_gazelle_changes(get_new_build_files(before), pre_mods)
        restore_exclude(path)
        return False, "gazelle failed"

    new_files = get_new_build_files(before)
    log(f"  gazelle created {len(new_files)} file(s)")

    passed, output = run_tests(test_targets)
    if passed:
        return True, ""

    revert_gazelle_changes(new_files, pre_mods)
    restore_exclude(path)

    if re.search(r"@@\[unknown repo|error loading package '@@", output):
        return False, "external module conflict"
    all_failing = failing_packages_from_output(output)
    unrelated = [p for p in all_failing if p != path and not p.startswith(path.rstrip("/") + "/")]
    if unrelated:
        return False, f"blocked by: {', '.join(unrelated)}"
    return False, "tests failed"


def migrate_local_direct(path: str, build_dir: str, rel_excl: str, test_targets: list[str]) -> tuple[bool, str]:
    """Remove a # gazelle:exclude from a local BUILD.bazel, run gazelle + tests."""
    pre_mods = get_modified_tracked_build_files()
    before = find_all_build_files()

    remove_local_exclude(build_dir, rel_excl)
    log(f"  removed # gazelle:exclude {rel_excl} from {build_dir}/BUILD.bazel")

    if not run_gazelle():
        revert_gazelle_changes(get_new_build_files(before), pre_mods)
        restore_local_exclude(build_dir, rel_excl)
        return False, "gazelle failed"

    new_files = get_new_build_files(before)
    log(f"  gazelle created {len(new_files)} file(s)")

    passed, output = run_tests(test_targets)
    if passed:
        return True, ""

    revert_gazelle_changes(new_files, pre_mods)
    restore_local_exclude(build_dir, rel_excl)

    if re.search(r"@@\[unknown repo|error loading package '@@", output):
        return False, "external module conflict"
    all_failing = failing_packages_from_output(output)
    unrelated = [p for p in all_failing if p != path and not p.startswith(path.rstrip("/") + "/")]
    if unrelated:
        return False, f"blocked by: {', '.join(unrelated)}"
    return False, "tests failed"


def migrate_carveout(parent: str, targets: list[str], test_targets: list[str]) -> dict[str, tuple[bool, str]]:
    """Remove parent exclude from root BUILD.bazel; create local <parent>/BUILD.bazel
    with # gazelle:ignore and # gazelle:exclude <rel> for all non-target subdirs.

    On failure, all changes are reverted (root exclude restored, local BUILD.bazel
    restored to its pre-operation state or deleted if it was newly created).
    """
    parent_abs = REPO_ROOT / parent
    targets_rel = {t[len(parent) + 1:] for t in targets}

    min_excludes = get_minimal_excludes(parent_abs, targets_rel)

    local_build = parent_abs / "BUILD.bazel"
    local_existed_before = local_build.exists()
    old_local_content = local_build.read_text() if local_existed_before else None

    # Snapshot state before we touch anything
    pre_mods = get_modified_tracked_build_files()
    before = find_all_build_files()

    # Remove parent (and any child entries) from root BUILD.bazel
    remove_all_excludes_under(parent)
    log(f"  removed # gazelle:exclude {parent} (and children) from BUILD.bazel")

    # Write the local BUILD.bazel with gazelle:ignore + minimal excludes
    local_content = "# gazelle:ignore\n"
    for rel in min_excludes:
        local_content += f"# gazelle:exclude {rel}\n"
    local_build.write_text(local_content)
    log(f"  wrote {local_build.relative_to(REPO_ROOT)} ({len(min_excludes)} sub-dir excludes)")

    def _undo() -> None:
        """Revert all changes made by this carveout attempt."""
        revert_gazelle_changes(get_new_build_files(before), pre_mods)
        # Restore the local BUILD.bazel explicitly, because revert_gazelle_changes
        # skips files that were already modified before this operation (pre_mods).
        if local_existed_before:
            local_build.write_text(old_local_content)
            log(f"  restored {local_build.relative_to(REPO_ROOT)}")
        elif local_build.exists():
            local_build.unlink()
            log(f"  deleted {local_build.relative_to(REPO_ROOT)}")
        restore_exclude(parent)

    if not run_gazelle():
        _undo()
        return {t: (False, "gazelle failed") for t in targets}

    new_files = get_new_build_files(before)
    log(f"  gazelle created {len(new_files)} file(s)")

    passed, output = run_tests(test_targets)
    if passed:
        results = {}
        for t in targets:
            t_build = REPO_ROOT / t / "BUILD.bazel"
            results[t] = (True, "") if t_build.exists() else (False, "no BUILD.bazel generated")
        return results

    _undo()

    if re.search(r"@@\[unknown repo|error loading package '@@", output):
        return {t: (False, "external module conflict") for t in targets}
    all_failing = failing_packages_from_output(output)
    unrelated = [
        p for p in all_failing
        if not any(p == t or p.startswith(t + "/") for t in targets)
        and not p.startswith(parent + "/")
    ]
    if unrelated:
        return {t: (False, f"blocked by: {', '.join(unrelated)}") for t in targets}
    return {t: (False, "tests failed") for t in targets}


# ── TODO file parsing ─────────────────────────────────────────────────────────

def parse_todo(todo_file: str) -> list[str]:
    """Parse TODO file; each non-empty line's last whitespace-delimited token is the path.

    Handles both plain paths and lines with an optional [N] prefix like:
        [  0]  pkg/some/path
        pkg/other/path
    """
    paths = []
    for line in open(todo_file):
        line = line.strip()
        if line:
            paths.append(line.split()[-1])
    return paths


# ── commit ────────────────────────────────────────────────────────────────────

def do_commit(description: str, new_files: list[Path]) -> None:
    """Stage new BUILD.bazel files and commit all BUILD.bazel changes."""
    if new_files:
        run(["git", "add"] + [str(f) for f in new_files], check=True)
    msg = f"migrate gazelle: {description}\n\nCo-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
    for attempt in range(3):
        result = subprocess.run(
            ["git", "commit", "-a", "-m", msg],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
        )
        if result.returncode == 0:
            log(f"  committed: {description}")
            return
        if "index.lock" in result.stderr and attempt < 2:
            import time
            log("  git index.lock conflict, retrying in 3s…")
            time.sleep(3)
        else:
            raise subprocess.CalledProcessError(
                result.returncode, ["git", "commit"], result.stdout, result.stderr
            )


# ── main ──────────────────────────────────────────────────────────────────────

def main() -> int:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("--todo", metavar="FILE",
                        help="Path to TODO file listing target package paths")
    parser.add_argument("--package", metavar="PKG", nargs="+",
                        help="One or more package paths to migrate directly")
    parser.add_argument("--max-files", type=int, default=DEFAULT_MAX_FILES, metavar="N",
                        help=f"Stop when N BUILD.bazel files have changed (default: {DEFAULT_MAX_FILES})")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print what would happen without executing")
    parser.add_argument("--no-commit", action="store_true",
                        help="Don't commit after each successful migration")
    parser.add_argument("--test-targets", nargs="+", default=DEFAULT_TEST_TARGETS, metavar="TARGET",
                        help="Bazel test targets to run after each migration")
    args = parser.parse_args()

    if not args.todo and not args.package:
        parser.error("at least one of --todo or --package is required")

    paths = []
    if args.todo:
        paths += parse_todo(args.todo)
    if args.package:
        paths += args.package

    excludes = current_excludes()

    # Classify: direct (has own exclude line) vs under a parent exclude
    parent_map: dict[str, list[str]] = {}
    for p in paths:
        if p not in excludes:
            parts = p.split("/")
            par = next(
                ("/".join(parts[:i]) for i in range(len(parts) - 1, 0, -1)
                 if "/".join(parts[:i]) in excludes),
                None,
            )
            if par:
                parent_map.setdefault(par, []).append(p)

    n_direct = sum(1 for p in paths if p in excludes)
    log(f"TODO: {len(paths)} entries  Direct: {n_direct}  Parent groups: {len(parent_map)}")
    log(f"File limit: {args.max_files}  Current changed files: {changed_build_file_count()}")
    log("")

    results: dict[str, tuple[bool, str]] = {}
    processed_parents: set[str] = set()

    def at_limit() -> bool:
        count = changed_build_file_count()
        if count >= args.max_files:
            log(f"\n*** Reached {count} changed BUILD.bazel files — stopping. ***")
            return True
        return False

    for path in paths:
        if at_limit():
            break

        if path in excludes:
            # ── DIRECT (root BUILD.bazel) ────────────────────────────────────
            log(f">>> DIRECT: {path}")
            if args.dry_run:
                log("  [dry-run] would remove exclude and run gazelle + tests")
                log("")
                continue

            before = find_all_build_files()
            ok, reason = migrate_direct(path, args.test_targets)
            results[path] = (ok, reason)
            log("  " + ("PASS" if ok else f"FAIL: {reason}"))
            if ok:
                if not args.no_commit:
                    do_commit(path, get_new_build_files(before))
                excludes = current_excludes()
            log("")

        else:
            # Check for a local BUILD.bazel exclude first
            local = find_local_exclude(path)
            if local is not None:
                build_dir, rel_excl = local
                # ── LOCAL DIRECT (local BUILD.bazel) ─────────────────────────
                log(f">>> LOCAL DIRECT: {path}  (from {build_dir}/BUILD.bazel: exclude {rel_excl})")
                if args.dry_run:
                    log(f"  [dry-run] would remove # gazelle:exclude {rel_excl} from {build_dir}/BUILD.bazel")
                    log("")
                    continue

                before = find_all_build_files()
                ok, reason = migrate_local_direct(path, build_dir, rel_excl, args.test_targets)
                results[path] = (ok, reason)
                log("  " + ("PASS" if ok else f"FAIL: {reason}"))
                if ok and not args.no_commit:
                    do_commit(path, get_new_build_files(before))
                log("")
                continue

            # ── CARVEOUT (root BUILD.bazel parent) ──────────────────────────
            parts = path.split("/")
            par = next(
                ("/".join(parts[:i]) for i in range(len(parts) - 1, 0, -1)
                 if "/".join(parts[:i]) in excludes),
                None,
            )
            if par is None:
                if path not in results:
                    results[path] = (False, "no matching exclude found")
                continue
            if par in processed_parents:
                continue

            targets = parent_map[par]
            log(f">>> CARVEOUT: {par}  targets={targets}")
            if args.dry_run:
                log(f"  [dry-run] would remove {par} exclude, create local BUILD.bazel, run gazelle + tests")
                log("")
                processed_parents.add(par)
                continue

            before = find_all_build_files()
            group = migrate_carveout(par, targets, args.test_targets)
            for t, (ok, reason) in group.items():
                results[t] = (ok, reason)
                log(f"  {'PASS' if ok else 'FAIL: ' + reason}  {t}")

            if any(ok for ok, _ in group.values()):
                if not args.no_commit:
                    do_commit(par, get_new_build_files(before))
                excludes = current_excludes()
            processed_parents.add(par)
            log("")

    # ── Summary ───────────────────────────────────────────────────────────────
    log("=" * 60)
    ok_list = [p for p, (ok, _) in results.items() if ok]
    fail_list = [(p, r) for p, (ok, r) in results.items() if not ok]
    log(f"SUMMARY: {len(ok_list)} succeeded, {len(fail_list)} failed")
    if ok_list:
        log("Succeeded:")
        for p in ok_list:
            log(f"  + {p}")
    if fail_list:
        log("Failed:")
        for p, r in fail_list:
            log(f"  - {p}  ({r})")
    log(f"\nChanged BUILD.bazel files: {changed_build_file_count()}")

    return 0 if not fail_list else 1


if __name__ == "__main__":
    sys.exit(main())

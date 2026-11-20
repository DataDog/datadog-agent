#!/usr/bin/env python3
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.
"""Map Go source paths to the Agent components they belong to, and list how to wire each component in a test.

Usage, from anywhere in the repository:
    map_components.py                # the Go files changed against the merge base with origin/main
    map_components.py <path>...      # Go files or directories, e.g. the ones a feature touches
    map_components.py --all ...      # list every consumer instead of the first ones

For every component or bundle reached, it prints its team, the fx modules and mocks available to wire it in a test,
and the packages that import its interface or its fx modules (its consumers and the packages wiring it, the next
hops of the pipeline). A pkg/ path is mapped to the
components importing it. The output is Markdown, meant to be read by the write-test skill.
"""

import functools
import os
import posixpath
import re
import subprocess
import sys
from collections import defaultdict

MODULE = "github.com/DataDog/datadog-agent"
# subdirectories that hold a way to build the component in a test, from the most to the least real
WIRING_DIRS = ["fx", "impl", "fx-mock", "mock", "fx-noop", "noop-impl"]
# subdirectories of a component that are not components themselves
COMPONENT_SUBDIRS = set(WIRING_DIRS) | {"def"}
MAX_CONSUMERS = 8


def git(*args):
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def changed_go_files():
    """Return the merge base and the Go files changed since it, deleted ones and the sources of renames included: they are entry points too."""
    try:
        base = git("merge-base", "HEAD", "origin/main").strip()
    except subprocess.CalledProcessError:
        sys.exit("origin/main not found: fetch it (git fetch origin main), or pass the paths to map explicitly.")
    tracked = []
    # --name-status keeps both paths of a rename or copy: the old package and its importers are entry points too
    fields = git("diff", "--name-status", "-z", base).split("\0")
    i = 0
    while i < len(fields) and fields[i]:
        count = 2 if fields[i][0] in "RC" else 1
        tracked += fields[i + 1 : i + 1 + count]
        i += 1 + count
    untracked = [p for p in git("ls-files", "-z", "--others", "--exclude-standard").split("\0") if p]
    return base, sorted({p for p in tracked + untracked if p.endswith(".go")})


@functools.cache
def tree_files(rev):
    """Return the set of files and the set of directories of the tree at rev."""
    files = {p for p in git("ls-tree", "-r", "-z", "--name-only", rev).split("\0") if p}
    dirs = {posixpath.dirname(f) for f in files}
    for d in list(dirs):
        while d:
            d = posixpath.dirname(d)
            dirs.add(d)
    return files, dirs


def exists(path, rev=None):
    return os.path.exists(path) if rev is None else path in tree_files(rev)[0]


def isdir(path, rev=None):
    return os.path.isdir(path) if rev is None else path in tree_files(rev)[1]


def read(path, rev=None):
    if rev is None:
        with open(path, encoding="utf-8") as f:
            return f.read()
    return git("show", f"{rev}:{path}")


def listdir(directory, rev=None):
    if rev is None:
        return os.listdir(directory)
    prefix = directory + "/"
    return sorted({f[len(prefix) :].split("/")[0] for f in tree_files(rev)[0] if f.startswith(prefix)})


def expand(paths):
    """Return the Go files of the given files and directories, warning about the arguments that hold none."""
    files = []
    for path in paths:
        if os.path.isdir(path):
            listed = [
                p
                for p in git("ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", path).split("\0")
                if p
            ]
            found = [p for p in listed if p.endswith(".go") and os.path.exists(p)]
        elif os.path.isfile(path) and path.endswith(".go"):
            found = [path]
        else:
            found = []
        if not found:
            print(f"warning: ignoring `{path}`: not a Go file or a directory holding Go files", file=sys.stderr)
        files += found
    return files


def group_by_package(files):
    packages = defaultdict(list)
    for path in files:
        packages[posixpath.dirname(path)].append(path)
    return packages


@functools.cache
def package_root(directory, rev=None):
    """Return (root, kind) of the component or bundle containing the package directory, or (None, None).

    rev reads the tree at that revision instead of the working tree.
    """
    while directory.startswith("comp/"):
        # def/, fx/, impl/... belong to the component above them, even when they hold a component.go
        if posixpath.basename(directory) not in COMPONENT_SUBDIRS:
            if exists(posixpath.join(directory, "def", "component.go"), rev):
                return directory, "component"
            if exists(posixpath.join(directory, "component.go"), rev):
                return directory, "component (legacy layout)"
            if exists(posixpath.join(directory, "bundle.go"), rev):
                return directory, "bundle"
        directory = posixpath.dirname(directory)
    return None, None


def team(root, rev=None):
    for name in ("def/component.go", "component.go", "bundle.go"):
        candidate = posixpath.join(root, name)
        if exists(candidate, rev):
            match = re.search(r"^// team: (.+)$", read(candidate, rev), re.MULTILINE)
            if match:
                return match.group(1).strip()
    return "?"


def declares_module(directory, rev=None):
    """Return whether a non-test Go file of the directory declares a Module() function."""
    for name in listdir(directory, rev):
        if name.endswith(".go") and not name.endswith("_test.go"):
            if re.search(r"^func Module\(", read(posixpath.join(directory, name), rev), re.MULTILINE):
                return True
    return False


def wiring(root, kind, rev=None):
    """List what can build the component in a test."""
    found = [f"`{d}/`" for d in WIRING_DIRS if isdir(posixpath.join(root, d), rev)]
    if kind == "bundle":
        found.insert(0, "`Bundle()`")
    elif kind == "component (legacy layout)":
        # the root package holds the fx module and, often, the mock
        if declares_module(root, rev):
            found.insert(0, "root `Module()`")
        found += [f"`{name}`" for name in sorted(listdir(root, rev)) if name.endswith("_mock.go")]
    return found


def public_packages(root, rev=None):
    """Return the packages other code imports to use or wire the component: its interface, fx modules and impls.

    Importers of def/ use the component; importers of fx/ (or fx-mock/, fx-noop/) assemble it, which is how a bundle
    or a binary that wires the pipeline shows up. Some production code imports impl/ directly, bypassing fx
    (telemetryimpl.GetCompatComponent()), and is a consumer too. The root package is public when it holds Go code, as a
    bundle's Bundle() or a legacy component's Module() does: binaries import it directly.
    """
    packages = [posixpath.join(root, d) for d in ["def"] + WIRING_DIRS]
    packages = [p for p in packages if isdir(p, rev)]
    if any(n.endswith(".go") and not n.endswith("_test.go") for n in listdir(root, rev)):
        packages.append(root)
    return packages or [root]


def importers(package_dir, rev=None):
    """Return the directories of the non-test Go files importing package_dir, in the working tree or at rev."""
    return importers_of((package_dir,), rev)


@functools.cache
def importers_of(package_dirs, rev=None):
    """Return the directories of the non-test Go files importing any of package_dirs, with one git grep."""
    patterns = [arg for d in package_dirs for arg in ("-e", f'"{MODULE}/{d}"')]
    # --untracked: a new component's importers may not be committed yet, like the component itself
    where = ["--untracked"] if rev is None else []
    revs = [] if rev is None else [rev]
    try:
        output = git("grep", "-l", "-z", *where, "--fixed-strings", *patterns, *revs, "--", "*.go", ":!*_test.go")
    except subprocess.CalledProcessError:  # no match
        return frozenset()
    # at a revision, git grep prefixes each path with "<rev>:"
    paths = [p.split(":", 1)[1] if rev is not None else p for p in output.split("\0") if p]
    return frozenset(posixpath.dirname(p) for p in paths)


def main():
    # paths are matched against repository-relative prefixes such as comp/: resolve the arguments from where they were
    # typed, then work from the repository root
    args = sys.argv[1:]
    show_all = "--all" in args
    args = [a for a in args if a != "--all"]
    root = git("rev-parse", "--show-toplevel").strip()
    # repository paths are slash-separated, as git prints them, whatever the OS
    paths = [os.path.relpath(os.path.abspath(p), root).replace(os.sep, "/") for p in args]
    os.chdir(root)
    base = "origin/main"
    if paths:
        files = expand(paths)
    else:
        base, files = changed_go_files()
        base = base[:12]
    if not files:
        print("No Go file to map.")
        return

    changed = defaultdict(set)  # root -> changed files
    kinds = {}
    gone = set()  # roots deleted since the merge base: described as they were there
    via_pkg = defaultdict(set)  # root -> pkg/ packages it imports
    unmapped = {}  # package -> number of files
    outside = {}  # pkg/ package -> its importers outside comp/, such as the binaries' commands
    for package_dir, package_files in group_by_package(files).items():
        root, kind = package_root(package_dir)
        if not paths and not all(os.path.exists(p) for p in package_files):
            # a deleted file belongs to the component that held it at the merge base, which may be deleted too: in
            # the working tree, its directory would be attributed to a surviving parent bundle
            old_root, old_kind = package_root(package_dir, base)
            # a root still holding a component, in another layout, is described as it is now
            if old_root and package_root(old_root)[0] != old_root:
                root, kind = old_root, old_kind
                gone.add(root)
        if root:
            changed[root].update(package_files)
            kinds[root] = kind
            continue
        package_importers = importers(package_dir)
        reached = {package_root(d) for d in package_importers} - {(None, None)}
        others = sorted(d for d in package_importers if package_root(d)[0] is None and d != package_dir)
        if others:
            outside[package_dir] = others
        if not reached:
            unmapped[package_dir] = len(package_files)
        for root, kind in reached:
            via_pkg[root].add(package_dir)
            kinds[root] = kind

    print("| Component | Kind | Team | Reached by | Test wiring | Consumers |")
    print("| --- | --- | --- | --- | --- | --- |")
    for root in sorted(set(changed) | set(via_pkg)):
        rev = base if root in gone else None
        reached_by = sorted(
            f"`{posixpath.relpath(p, root)}`" + ("" if os.path.exists(p) else " (deleted)")
            for p in changed.get(root, ())
        )
        reached_by += sorted(f"imports `{p}`" for p in via_pkg.get(root, ()))
        consumers = sorted(
            {package_root(d, rev)[0] or d for d in importers_of(tuple(public_packages(root, rev)), rev)} - {root}
        )
        listed = consumers if show_all else consumers[:MAX_CONSUMERS]
        shown = ", ".join(f"`{c}`" for c in listed)
        if len(consumers) > len(listed):
            shown += f", +{len(consumers) - len(listed)} more (--all lists them)"
        kind = kinds[root] + (f", deleted: as at `{base}`" if root in gone else "")
        print(
            f"| `{root}` | {kind} | {team(root, rev)} | {', '.join(reached_by)} "
            f"| {', '.join(wiring(root, kinds[root], rev)) or '-'} | {shown or '-'} |"
        )
    if outside:
        # a cmd/ package is where a binary exposes the behavior: an entry point the component table can't show
        print("\nImported outside comp/, by the binaries' commands and other packages:")
        for package_dir in sorted(outside):
            listed = outside[package_dir] if show_all else outside[package_dir][:MAX_CONSUMERS]
            more = len(outside[package_dir]) - len(listed)
            shown = ", ".join(f"`{d}`" for d in listed) + (f", +{more} more (--all lists them)" if more else "")
            print(f"- `{package_dir}`: {shown}")
    if unmapped:
        print(
            "\nNo component imports these packages directly: pass their importers (`git grep -l '\"<import path>\"'`)"
        )
        print("to this script to follow the pipeline up to a component, or test them as plain packages:")
        for package_dir in sorted(unmapped):
            print(f"- `{package_dir}` ({unmapped[package_dir]} file(s))")
    # a package is gone once no Go source is left: its directory can survive, holding fixtures or documentation
    deleted = sorted(p for p in unmapped if not (os.path.isdir(p) and any(n.endswith(".go") for n in os.listdir(p))))
    if deleted:
        # their importers changed or are gone too, so they can only be followed at the merge base
        print(f"\nDeleted entirely, so follow them at the merge base `{base}`:", ", ".join(f"`{p}`" for p in deleted))
        print(f"`git show {base}:<path>` reads a file, `git grep -l '\"<import path>\"' {base}` its importers.")


if __name__ == "__main__":
    main()

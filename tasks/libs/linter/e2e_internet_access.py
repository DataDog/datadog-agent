"""Checks that every E2E Internet access opt-in is listed in the allowlist registry."""

from __future__ import annotations

import functools
from collections import Counter
from collections.abc import Collection, Iterable
from dataclasses import dataclass
from pathlib import Path, PurePath

import yaml

from tasks.libs.common.utils import join_command

REGISTRY_PATH = "test/new-e2e/internet-access.yaml"
SCANNED_DIRS = ("test/new-e2e/", "test/e2e-framework/")
SCANNED_PATHSPECS = tuple(f"{directory}*.go" for directory in SCANNED_DIRS)
TREE_SITTER_DEPS = ("tree-sitter==0.26.0", "tree-sitter-go==0.25.0")
POLICY_DOC = "test/new-e2e/codereview_guideline.md"
STATUSES = ("justified", "debt", "untriaged")
_FIELDS = ("file", "function", "status", "reason")

_IDENT = "WithInternetAccess"
_IDENT_NODES = {"identifier", "field_identifier"}
_FUNC_NODES = {"function_declaration", "method_declaration"}


@dataclass(frozen=True)
class Usage:
    file: str
    function: str | None
    line: int


@dataclass(frozen=True)
class Entry:
    file: str
    function: str
    status: str
    reason: str


@functools.cache
def _parser():
    import tree_sitter_go
    from tree_sitter import Language, Parser

    return Parser(Language(tree_sitter_go.language()))


def find_usages(source: str) -> list[tuple[int, str | None]]:
    """Return (line, enclosing top-level function or None) for each WithInternetAccess reference in Go `source`."""
    usages = []
    stack = [_parser().parse(source.encode()).root_node]
    while stack:
        node = stack.pop()
        if node.type in _IDENT_NODES and node.text.decode() == _IDENT:
            usages.append((node.start_point[0] + 1, _enclosing_function(node)))
        stack.extend(reversed(node.children))
    return usages


def _enclosing_function(node) -> str | None:
    while node is not None and node.type not in _FUNC_NODES:
        node = node.parent
    if node is None:
        return None
    name = node.child_by_field_name("name").text.decode()
    receiver = node.child_by_field_name("receiver")
    if receiver is None:
        return name
    receiver_type = receiver.named_children[0].child_by_field_name("type")
    while receiver_type.type != "type_identifier":
        receiver_type = receiver_type.child_by_field_name("type") or receiver_type.named_children[0]
    return f"{receiver_type.text.decode()}.{name}"


def scan(root: Path, files: Iterable[str]) -> list[Usage]:
    """Return every WithInternetAccess usage in `files`, given relative to `root`."""
    usages = []
    for file in files:
        source = (root / file).read_text(encoding="utf-8")
        if _IDENT in source:
            usages.extend(Usage(file, function, line) for line, function in find_usages(source))
    return usages


def normalize_paths(paths: Iterable[str], root: Path) -> list[str]:
    """Return `paths` relative to `root` in POSIX form.

    Relative paths resolve from the working directory. Directories, and missing paths without a ".go" suffix, end with "/";
    `root` and its ancestors become "", and other paths outside `root` are dropped.
    """
    root = root.resolve()
    normalized = []
    for path in paths:
        absolute = Path(path).resolve()
        if root.is_relative_to(absolute):
            normalized.append("")
        elif absolute.is_relative_to(root):
            relative = absolute.relative_to(root).as_posix()
            is_dir = absolute.is_dir() or (not absolute.exists() and absolute.suffix != ".go")
            normalized.append(f"{relative}/" if is_dir else relative)
    return normalized


def select_scope(paths: Iterable[str]) -> list[str] | None:
    """Return the normalized `paths` that can contain scanned Go files, or None when a full scan is needed.

    A full scan is needed when `paths` is empty, includes the registry, or includes a directory containing every scanned directory.
    """
    paths = list(paths)
    if not paths or REGISTRY_PATH in paths:
        return None
    scope = []
    for path in paths:
        if path == "" or path.endswith("/"):
            if all(directory.startswith(path) for directory in SCANNED_DIRS):
                return None
            if path.startswith(SCANNED_DIRS) or any(directory.startswith(path) for directory in SCANNED_DIRS):
                scope.append(path)
        elif path.endswith(".go") and path.startswith(SCANNED_DIRS):
            scope.append(path)
    return scope


def target_pathspecs(scope: list[str] | None) -> tuple[str, ...]:
    """Return the `git ls-files` pathspecs selecting the Go files of `scope` (every scanned file when None)."""
    if scope is None:
        return SCANNED_PATHSPECS
    return tuple(f"{path}*.go" if path.endswith("/") else path for path in scope)


def ls_files_command(root: PurePath, pathspecs: Iterable[str]) -> str:
    """Return the shell command listing the tracked files of `root` matching `pathspecs`, NUL-separated."""
    return join_command(["git", "-C", str(root), "ls-files", "-z", "--", *pathspecs])


def in_scope(file: str, scope: Collection[str] | None) -> bool:
    """Return whether `file` is one of the files of `scope`, or lies under one of its directories."""
    return scope is None or any(file == path or (path.endswith("/") and file.startswith(path)) for path in scope)


def load_registry(text: str) -> tuple[list[Entry], list[str]]:
    """Parse the registry YAML `text` into valid entries, plus one error per invalid entry."""
    data = yaml.safe_load(text)
    raw_entries = data.get("entries") if isinstance(data, dict) else None
    if not isinstance(raw_entries, list):
        return [], [f"{REGISTRY_PATH}: expected a top-level 'entries' list"]
    entries, errors, seen = [], [], set()
    for index, raw in enumerate(raw_entries, start=1):
        where = f"{REGISTRY_PATH}: entry #{index}"
        if not isinstance(raw, dict) or set(raw) != set(_FIELDS):
            errors.append(f"{where}: expected exactly the fields {', '.join(_FIELDS)}")
        elif not all(isinstance(raw[field], str) and raw[field].strip() for field in _FIELDS):
            errors.append(f"{where}: every field must be a non-empty string")
        elif raw["status"] not in STATUSES:
            errors.append(f"{where}: status must be one of {', '.join(STATUSES)}")
        elif (raw["file"], raw["function"]) in seen:
            errors.append(f"{where}: duplicate entry for {raw['file']} {raw['function']}")
        else:
            seen.add((raw["file"], raw["function"]))
            entries.append(Entry(**raw))
    return entries, errors


def check(usages: list[Usage], entries: list[Entry], scope: Collection[str] | None = None) -> list[str]:
    """Return one error per usage outside a function, unregistered usage, or stale entry within `scope`.

    `scope` holds normalized files and directories (see `select_scope`); None checks every entry for staleness.
    """
    errors = []
    found: dict[tuple[str, str], int] = {}
    for usage in usages:
        if usage.function is None:
            errors.append(f"{usage.file}:{usage.line}: {_IDENT} used outside a function; move it into one")
        else:
            found.setdefault((usage.file, usage.function), usage.line)
    registered = {(entry.file, entry.function) for entry in entries}
    for (file, function), line in sorted(found.items()):
        if (file, function) not in registered:
            errors.append(
                f"{file}:{line}: {function} uses {_IDENT} but is not listed in {REGISTRY_PATH}.\n"
                f"Remove the Internet dependency (see {POLICY_DOC}), or request an exception from "
                f"@DataDog/agent-devx by adding:\n"
                f"- file: {file}\n  function: {function}\n  status: justified\n"
                f"  reason: <why this test needs Internet access>"
            )
    for entry in entries:
        if in_scope(entry.file, scope) and (entry.file, entry.function) not in found:
            errors.append(
                f"{REGISTRY_PATH}: stale entry {entry.file} {entry.function} matches no {_IDENT} usage; remove it"
            )
    return errors


def summarize(entries: list[Entry]) -> str:
    """Return a one-line count of registry entries per status."""
    counts = Counter(entry.status for entry in entries)
    return f"{len(entries)} Internet access exceptions: " + ", ".join(f"{counts[s]} {s}" for s in STATUSES)

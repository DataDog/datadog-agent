"""Checks that every E2E Internet access opt-in is listed in the allowlist registry."""

from __future__ import annotations

import functools
from collections import Counter
from collections.abc import Collection, Iterable
from dataclasses import dataclass, field
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parents[3]
REGISTRY_PATH = Path("test/new-e2e/internet-access.yaml")
SCANNED_DIRS = (Path("test/new-e2e"), Path("test/e2e-framework"))
POLICY_DOC = Path("test/new-e2e/codereview_guideline.md")
TREE_SITTER_DEPS = ("tree-sitter==0.26.0", "tree-sitter-go==0.25.0")
STATUSES = ("justified", "debt", "untriaged")

_FIELDS = ("file", "function", "status", "reason")
_IDENT = "WithInternetAccess"
_IDENT_NODES = {"identifier", "field_identifier"}
_FUNC_NODES = {"function_declaration", "method_declaration"}


# === Scope === #


def normalize_paths(paths: Iterable[str], root: Path) -> list[Path]:
    """Return `paths` relative to `root`; relative paths resolve from the working directory.

    `root` and its ancestors become ".", and other paths outside `root` are dropped.
    """
    root = root.resolve()
    normalized = []
    for path in paths:
        absolute = Path(path).resolve()
        if root.is_relative_to(absolute):
            normalized.append(Path("."))
        elif absolute.is_relative_to(root):
            normalized.append(absolute.relative_to(root))
    return normalized


def select_scope(paths: Iterable[Path]) -> list[Path] | None:
    """Return the normalized `paths` that can contain scanned Go files, or None when a full scan is needed.

    A full scan is needed when `paths` is empty, includes the registry, or includes a directory containing every scanned directory.
    """
    paths = list(paths)
    if not paths or REGISTRY_PATH in paths:
        return None
    scope = []
    for path in paths:
        if all(directory.is_relative_to(path) for directory in SCANNED_DIRS):
            return None
        if any(path.is_relative_to(directory) or directory.is_relative_to(path) for directory in SCANNED_DIRS):
            scope.append(path)
    return scope


def _in_scope(file: Path, scope: Collection[Path] | None) -> bool:
    return scope is None or any(file.is_relative_to(path) for path in scope)


# === Parsing === #


@dataclass(frozen=True)
class Usage:
    file: Path
    function: str | None
    line: int


def scan(root: Path, scope: Collection[Path] | None) -> list[Usage]:
    """Return every WithInternetAccess usage in the scanned Go files under `root`, restricted to `scope` unless None."""
    paths = set()
    for base in SCANNED_DIRS if scope is None else scope:
        path = root / base
        if path.is_dir():
            paths.update(child for child in path.rglob("*.go") if child.is_file())
        elif path.suffix == ".go" and path.is_file():
            paths.add(path)
    usages = []
    for path in sorted(paths):
        usages.extend(_find_usages(path, path.relative_to(root)))
    return usages


@functools.cache
def _parser():
    import tree_sitter_go
    from tree_sitter import Language, Parser

    return Parser(Language(tree_sitter_go.language()))


def _find_usages(path: Path, file: Path) -> list[Usage]:
    source = path.read_bytes()
    if _IDENT.encode() not in source:
        return []
    usages = []
    stack = [_parser().parse(source).root_node]
    while stack:
        node = stack.pop()
        if node.type in _IDENT_NODES and node.text.decode() == _IDENT:
            usages.append(Usage(file, _enclosing_function(node), node.start_point[0] + 1))
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


# === Registry === #


@dataclass(frozen=True)
class Entry:
    file: Path
    function: str
    status: str
    reason: str


@dataclass
class Registry:
    entries: list[Entry]
    errors: list[str] = field(default_factory=list)

    @classmethod
    def parse(cls, text: str) -> Registry:
        """Parse the registry YAML `text`, keeping one error per invalid entry."""
        data = yaml.safe_load(text)
        raw_entries = data.get("entries") if isinstance(data, dict) else None
        if not isinstance(raw_entries, list):
            return cls([], [f"{REGISTRY_PATH.as_posix()}: expected a top-level 'entries' list"])
        registry, seen = cls([]), set()
        for index, raw in enumerate(raw_entries, start=1):
            where = f"{REGISTRY_PATH.as_posix()}: entry #{index}"
            if not isinstance(raw, dict) or set(raw) != set(_FIELDS):
                registry.errors.append(f"{where}: expected exactly the fields {', '.join(_FIELDS)}")
            elif not all(isinstance(raw[name], str) and raw[name].strip() for name in _FIELDS):
                registry.errors.append(f"{where}: every field must be a non-empty string")
            elif raw["status"] not in STATUSES:
                registry.errors.append(f"{where}: status must be one of {', '.join(STATUSES)}")
            elif (Path(raw["file"]), raw["function"]) in seen:
                registry.errors.append(f"{where}: duplicate entry for {raw['file']} {raw['function']}")
            else:
                entry = Entry(Path(raw["file"]), raw["function"], raw["status"], raw["reason"])
                seen.add((entry.file, entry.function))
                registry.entries.append(entry)
        return registry

    def check(self, usages: Iterable[Usage], scope: Collection[Path] | None = None) -> list[str]:
        """Return one error per usage outside a function, unregistered usage, or stale entry within `scope`.

        A `scope` of None checks every entry for staleness.
        """
        errors = []
        found: dict[tuple[Path, str], int] = {}
        for usage in usages:
            if usage.function is None:
                errors.append(
                    f"{usage.file.as_posix()}:{usage.line}: {_IDENT} used outside a function; move it into one"
                )
            else:
                found.setdefault((usage.file, usage.function), usage.line)
        registered = {(entry.file, entry.function) for entry in self.entries}
        for (file, function), line in sorted(found.items()):
            if (file, function) not in registered:
                errors.append(
                    f"{file.as_posix()}:{line}: {function} uses {_IDENT} but is not listed in {REGISTRY_PATH.as_posix()}.\n"
                    f"Remove the Internet dependency (see {POLICY_DOC.as_posix()}), or request an exception from "
                    f"@DataDog/agent-devx by adding:\n"
                    f"- file: {file.as_posix()}\n  function: {function}\n  status: justified\n"
                    f"  reason: <why this test needs Internet access>"
                )
        for entry in self.entries:
            if _in_scope(entry.file, scope) and (entry.file, entry.function) not in found:
                errors.append(
                    f"{REGISTRY_PATH.as_posix()}: stale entry {entry.file.as_posix()} {entry.function} "
                    f"matches no {_IDENT} usage; remove it"
                )
        return errors

    def summary(self) -> str:
        """Return a one-line count of entries per status."""
        counts = Counter(entry.status for entry in self.entries)
        return f"{len(self.entries)} Internet access exceptions: " + ", ".join(f"{counts[s]} {s}" for s in STATUSES)

"""Provides repository-owned Buildifier file selection and command batching."""

from __future__ import annotations

import stat
from collections.abc import Iterable, Sequence
from fnmatch import fnmatchcase
from pathlib import Path

from tasks.libs.common.utils import join_command

BUILDIFIER_MAX_COMMAND_LENGTH = 7_500
BUILDIFIER_MAX_FILES = 100

_EXCLUDED_PATH_PATTERNS = (
    "./.*",
    "./third_party/*",
    "./deps/*/overlay/*",
    "./vendor/*",
)

_EXACT_FILENAMES = frozenset(
    {
        "BUILD",
        "BUILD.bazel",
        "MODULE.bazel",
        "REPO.bazel",
        "WORKSPACE",
        "WORKSPACE.bazel",
        "WORKSPACE.bzlmod",
        "WORKSPACE.oss",
    }
)

_FILENAME_PATTERNS = (
    "*.bzl",
    "*.sky",
    "*.star",
    "*.BUILD",
    "*.BUILD.bazel",
    "BUILD.*.bazel",
    "BUILD.*.oss",
    "*.MODULE.bazel",
    "WORKSPACE.*.bazel",
    "WORKSPACE.*.oss",
)


def _normalize_repository_path(path: str) -> str | None:
    normalized = path.replace("\\", "/")
    while normalized.startswith("./"):
        normalized = normalized[2:]

    if not normalized or normalized.startswith("/") or (len(normalized) > 1 and normalized[1] == ":"):
        return None
    if any(part in {"", ".", ".."} for part in normalized.split("/")):
        return None

    return f"./{normalized}"


def _normalize_buildifier_path(path: str) -> str | None:
    normalized = _normalize_repository_path(path)
    if normalized is None or any(fnmatchcase(normalized, pattern) for pattern in _EXCLUDED_PATH_PATTERNS):
        return None

    filename = normalized.rsplit("/", 1)[-1]
    if filename not in _EXACT_FILENAMES and not any(fnmatchcase(filename, pattern) for pattern in _FILENAME_PATTERNS):
        return None

    return normalized


def is_buildifier_path(path: str) -> bool:
    """Return whether a repository-relative path belongs to the Buildifier policy."""
    return _normalize_buildifier_path(path) is not None


def select_buildifier_files(paths: Iterable[str], repo_root: Path) -> list[str]:
    """Select existing regular files covered by the repository's Buildifier policy."""
    selected = set()
    for path in paths:
        normalized = _normalize_buildifier_path(path)
        if normalized is None:
            continue

        disk_path = repo_root.joinpath(*normalized.removeprefix("./").split("/"))
        try:
            mode = disk_path.lstat().st_mode
        except FileNotFoundError:
            continue
        if stat.S_ISREG(mode):
            selected.add(normalized)

    return sorted(selected)


def buildifier_commands(
    executable: str,
    files: Sequence[str],
    *,
    fix: bool,
    diff_command: str | None = None,
    max_files: int = BUILDIFIER_MAX_FILES,
    max_chars: int = BUILDIFIER_MAX_COMMAND_LENGTH,
) -> list[list[str]]:
    """Build bounded Buildifier command lines for the selected files."""
    if max_files <= 0:
        raise ValueError("The Buildifier batch size must be positive.")
    if max_chars <= 0:
        raise ValueError("The Buildifier command length limit must be positive.")

    mode = "fix" if fix else "diff"
    lint = "fix" if fix else "warn"
    prefix = [executable, f"-mode={mode}", f"-lint={lint}"]
    if not fix and diff_command is not None:
        prefix.append(f"-diff_command={diff_command}")
    commands = []
    batch: list[str] = []

    for path in files:
        candidate = [*batch, path]
        if batch and (len(candidate) > max_files or len(join_command([*prefix, *candidate])) > max_chars):
            commands.append([*prefix, *batch])
            batch = [path]
        else:
            batch = candidate

        if len(join_command([*prefix, *batch])) > max_chars:
            raise ValueError(f"The Buildifier command for {path!r} exceeds the {max_chars}-character limit.")

    if batch:
        commands.append([*prefix, *batch])

    return commands

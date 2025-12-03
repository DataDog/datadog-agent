# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
File reading utilities for CI configuration generation.

Provides a common interface (FileReader) with two implementations:
- LocalFileReader: reads from the local filesystem
- GitFileReader: reads from a specific git ref without checkout
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from typing import TYPE_CHECKING, Protocol

import yaml

if TYPE_CHECKING:
    from dda.cli.application import Application


class FileReader(Protocol):
    """
    Protocol for reading files from different sources.

    Implementations:
    - LocalFileReader: reads from the local filesystem
    - GitFileReader: reads from a specific git ref
    """

    @property
    def source_description(self) -> str:
        """Human-readable description of the source (e.g., 'local filesystem' or 'git ref main')."""
        ...

    def read_file(self, file_path: str) -> str | None:
        """
        Read a file's content.

        Args:
            file_path: Path relative to the project root.

        Returns:
            File content as string, or None if file doesn't exist.
        """
        ...

    def file_exists(self, file_path: str) -> bool:
        """Check if a file exists."""
        ...

    def list_files(self, directory: str = "") -> list[str]:
        """
        List files in a directory.

        Args:
            directory: Directory path relative to project root.

        Returns:
            List of file paths relative to the directory.
        """
        ...

    def load_yaml(self, file_path: str) -> dict:
        """
        Load a YAML file.

        Args:
            file_path: Path relative to the project root.

        Returns:
            Parsed YAML content as dictionary.

        Raises:
            FileNotFoundError: If file doesn't exist.
        """
        ...


class LocalFileReader:
    """
    Read files from the local filesystem.
    """

    def __init__(self, project_root: Path):
        """
        Initialize the local file reader.

        Args:
            project_root: Root path of the project.
        """
        self.project_root = project_root

    @property
    def source_description(self) -> str:
        return "local filesystem"

    def read_file(self, file_path: str) -> str | None:
        """Read a file's content from the local filesystem."""
        path = self.project_root / file_path
        if not path.exists():
            return None
        return path.read_text()

    def file_exists(self, file_path: str) -> bool:
        """Check if a file exists on the local filesystem."""
        return (self.project_root / file_path).exists()

    def list_files(self, directory: str = "") -> list[str]:
        """List files in a directory on the local filesystem."""
        dir_path = self.project_root / directory if directory else self.project_root
        if not dir_path.exists() or not dir_path.is_dir():
            return []

        files = []
        for path in dir_path.rglob("*"):
            if path.is_file():
                rel_path = path.relative_to(dir_path)
                files.append(str(rel_path))
        return sorted(files)

    def load_yaml(self, file_path: str) -> dict:
        """Load a YAML file from the local filesystem."""
        content = self.read_file(file_path)
        if content is None:
            raise FileNotFoundError(f"File '{file_path}' not found")
        return yaml.safe_load(content) or {}


class GitFileReader:
    """
    Read files from a specific git ref without checking out.

    This allows generating GitLab CI configurations as they would appear
    at a specific branch, tag, or commit.
    """

    def __init__(self, project_root: Path, ref: str, app: Application | None = None):
        """
        Initialize the git file reader.

        Args:
            project_root: Root path of the git repository.
            ref: Git ref to read from (branch, tag, or commit SHA).
            app: Optional application for logging.
        """
        self.project_root = project_root
        self.ref = ref
        self.app = app
        self._validate_ref()

    def _validate_ref(self) -> None:
        """Validate that the ref exists in the repository."""
        try:
            subprocess.run(
                ["git", "rev-parse", "--verify", self.ref],
                cwd=self.project_root,
                check=True,
                capture_output=True,
            )
        except subprocess.CalledProcessError as e:
            raise ValueError(f"Invalid git ref '{self.ref}': {e.stderr.decode().strip()}") from e

    @property
    def source_description(self) -> str:
        return f"git ref '{self.ref}'"

    def read_file(self, file_path: str) -> str | None:
        """Read a file's content from the specified git ref."""
        try:
            result = subprocess.run(
                ["git", "show", f"{self.ref}:{file_path}"],
                cwd=self.project_root,
                check=True,
                capture_output=True,
            )
            return result.stdout.decode()
        except subprocess.CalledProcessError:
            return None

    def file_exists(self, file_path: str) -> bool:
        """Check if a file exists at the specified git ref."""
        return self.read_file(file_path) is not None

    def list_files(self, directory: str = "") -> list[str]:
        """List files in a directory at the specified git ref."""
        try:
            tree_path = f"{self.ref}:{directory}" if directory else self.ref
            result = subprocess.run(
                ["git", "ls-tree", "-r", "--name-only", tree_path],
                cwd=self.project_root,
                check=True,
                capture_output=True,
            )
            files = result.stdout.decode().strip().split("\n")
            return [f for f in files if f]
        except subprocess.CalledProcessError:
            return []

    def load_yaml(self, file_path: str) -> dict:
        """Load a YAML file from the specified git ref."""
        content = self.read_file(file_path)
        if content is None:
            raise FileNotFoundError(f"File '{file_path}' not found at ref '{self.ref}'")
        return yaml.safe_load(content) or {}


def get_commit_info(project_root: Path, ref: str) -> dict[str, str]:
    """
    Get information about a git commit.

    Args:
        project_root: Root path of the git repository.
        ref: Git ref (branch, tag, or commit SHA).

    Returns:
        Dictionary with commit info (sha, author, date, message).
    """
    try:
        result = subprocess.run(
            ["git", "log", "-1", "--format=%H|%an|%ai|%s", ref],
            cwd=project_root,
            check=True,
            capture_output=True,
        )
        parts = result.stdout.decode().strip().split("|", 3)
        return {
            "sha": parts[0] if len(parts) > 0 else "",
            "author": parts[1] if len(parts) > 1 else "",
            "date": parts[2] if len(parts) > 2 else "",
            "message": parts[3] if len(parts) > 3 else "",
        }
    except subprocess.CalledProcessError:
        return {}


def resolve_ref(project_root: Path, ref: str) -> str:
    """
    Resolve a git ref to its full SHA.

    Args:
        project_root: Root path of the git repository.
        ref: Git ref (branch, tag, or commit SHA).

    Returns:
        Full commit SHA.
    """
    result = subprocess.run(
        ["git", "rev-parse", ref],
        cwd=project_root,
        check=True,
        capture_output=True,
    )
    return result.stdout.decode().strip()

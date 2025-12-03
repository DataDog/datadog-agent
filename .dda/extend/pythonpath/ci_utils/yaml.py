# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Custom YAML handling for GitLab CI configuration files.

This module provides support for GitLab CI-specific YAML tags like !reference.
It also removes YAML anchors/aliases when dumping to produce cleaner output.
"""

from __future__ import annotations

import copy
from pathlib import Path
from typing import Any

import yaml


class GitLabReference:
    """Represents a GitLab CI !reference tag."""

    def __init__(self, value: list):
        self.value = value

    def __repr__(self) -> str:
        return f"!reference {self.value}"


def _gitlab_reference_constructor(loader: yaml.SafeLoader, node: yaml.Node) -> GitLabReference:
    """Constructor for GitLab CI !reference tag."""
    value = loader.construct_sequence(node)
    return GitLabReference(value)


def _gitlab_reference_representer(dumper: yaml.SafeDumper, data: GitLabReference) -> yaml.Node:
    """Representer for GitLab CI !reference tag."""
    return dumper.represent_sequence("!reference", data.value)


class NoAnchorDumper(yaml.SafeDumper):
    """
    Custom YAML dumper that never uses anchors/aliases.

    This produces cleaner output where all values are inlined,
    even if the same object appears multiple times.
    """

    def ignore_aliases(self, data: Any) -> bool:
        """Always ignore aliases - inline all values."""
        return True


# Register the custom tag handlers
yaml.SafeLoader.add_constructor("!reference", _gitlab_reference_constructor)
yaml.SafeDumper.add_representer(GitLabReference, _gitlab_reference_representer)
NoAnchorDumper.add_representer(GitLabReference, _gitlab_reference_representer)


def load_yaml(path: Path) -> dict[str, Any]:
    """
    Load a YAML file with GitLab CI tag support.

    Args:
        path: Path to the YAML file.

    Returns:
        Parsed YAML content as a dictionary.
    """
    with open(path) as f:
        return yaml.safe_load(f) or {}


def dump_yaml(content: dict[str, Any], path: Path, header: str | None = None) -> None:
    """
    Dump content to a YAML file with GitLab CI tag support.

    Anchors/aliases are removed and all values are inlined for cleaner output.

    Args:
        content: Dictionary to dump.
        path: Path to write to.
        header: Optional header comment to add at the top of the file.
    """
    # Deep copy to ensure no shared references that could create anchors
    content = copy.deepcopy(content)

    with open(path, "w") as f:
        if header:
            for line in header.strip().split("\n"):
                f.write(f"# {line}\n")
            f.write("---\n")
        yaml.dump(
            content,
            f,
            Dumper=NoAnchorDumper,
            default_flow_style=False,
            sort_keys=False,
            allow_unicode=True,
        )

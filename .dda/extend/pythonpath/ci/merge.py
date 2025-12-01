# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Utilities for merging GitLab CI configuration files.
"""
from __future__ import annotations

import glob
from pathlib import Path
from typing import Any

from ci.yaml import load_yaml


def deep_merge(base: dict[str, Any], override: dict[str, Any]) -> dict[str, Any]:
    """
    Deep merge two dictionaries, with override taking precedence.

    Args:
        base: Base dictionary.
        override: Dictionary to merge on top of base.

    Returns:
        Merged dictionary.
    """
    result = base.copy()
    for key, value in override.items():
        if key in result and isinstance(result[key], dict) and isinstance(value, dict):
            result[key] = deep_merge(result[key], value)
        elif key in result and isinstance(result[key], list) and isinstance(value, list):
            # For lists, we concatenate (useful for stages, rules, etc.)
            result[key] = result[key] + value
        else:
            result[key] = value
    return result


def _resolve_include_path(include: str | dict, project_root: Path) -> list[Path]:
    """
    Resolve an include directive to a list of file paths.

    Supports:
    - Simple string paths: "path/to/file.yml"
    - Local includes: {"local": "path/to/file.yml"}
    - Glob patterns: "path/to/*.yml" or {"local": "path/to/*.yml"}

    Args:
        include: Include directive (string or dict).
        project_root: Root path of the project.

    Returns:
        List of resolved file paths.
    """
    if isinstance(include, str):
        pattern = include
    elif isinstance(include, dict) and "local" in include:
        pattern = include["local"].lstrip("/")
    else:
        # Skip remote includes, project includes, etc.
        return []

    # Check if it's a glob pattern
    if "*" in pattern or "?" in pattern or "[" in pattern:
        full_pattern = str(project_root / pattern)
        matched_files = glob.glob(full_pattern, recursive=True)
        return sorted(Path(f) for f in matched_files if Path(f).is_file())
    else:
        path = project_root / pattern
        return [path] if path.exists() else []


def resolve_includes(content: dict[str, Any], base_path: Path, project_root: Path) -> dict[str, Any]:
    """
    Recursively resolve all include directives in the YAML content.

    Supports glob patterns like "path/to/*.yml".

    Args:
        content: YAML content dictionary.
        base_path: Path of the current file being processed.
        project_root: Root path of the project.

    Returns:
        Merged content with all includes resolved.
    """
    if "include" not in content:
        return content

    includes = content.pop("include")
    if not isinstance(includes, list):
        includes = [includes]

    merged: dict[str, Any] = {}

    for include in includes:
        include_paths = _resolve_include_path(include, project_root)

        for include_path in include_paths:
            included_content = load_yaml(include_path)

            # Recursively resolve includes in the included file
            included_content = resolve_includes(included_content, include_path.parent, project_root)

            # Merge the included content
            merged = deep_merge(merged, included_content)

    # Merge the original content on top (it takes precedence)
    merged = deep_merge(merged, content)

    return merged


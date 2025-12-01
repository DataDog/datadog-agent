# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Parser for dd.pipelines.yml configuration.
"""
from __future__ import annotations

import fnmatch
import subprocess
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml


@dataclass
class ChangesTrigger:
    """Represents a changes trigger with include/all_except patterns.

    Keywords in dd.pipelines.yml:

    1. `include:` - Only match files matching these patterns (filter mode)
       ```yaml
       - changes:
         - include:
           - standalone-repo/**/*
       ```
       Result: Only standalone-repo files match

    2. `all_except:` - Match all files EXCEPT these patterns
       ```yaml
       - changes:
         - all_except:
           - standalone-repo/**/*
       ```
       Result: Everything except standalone-repo matches

    3. Combined `all_except` + `include` - include overrides all_except
       ```yaml
       - changes:
         - all_except:
           - standalone-repo/**/*
         - include:
           - standalone-repo/shared/**/*  # override: this WILL match
       ```
       Result:
         - pkg/main.go → matches (not in all_except)
         - standalone-repo/shared/util.go → matches (include overrides all_except)
         - standalone-repo/other/main.go → does not match (in all_except, no override)
    """

    include: list[str] = field(default_factory=list)
    all_except: list[str] = field(default_factory=list)

    def matches(self, file_path: str) -> bool:
        """Check if a file path matches this trigger."""
        # Case 1: Only `include` patterns - act as filter (only these files)
        if self.include and not self.all_except:
            for pattern in self.include:
                if _matches_pattern(file_path, pattern):
                    return True
            return False

        # Case 2: Only `all_except` patterns - everything except these files
        if self.all_except and not self.include:
            for pattern in self.all_except:
                if _matches_pattern(file_path, pattern):
                    return False
            return True

        # Case 3: `all_except` with `include` override
        # include patterns override all_except (exceptions to the exception)
        if self.all_except and self.include:
            # Check include first - overrides all_except
            for pattern in self.include:
                if _matches_pattern(file_path, pattern):
                    return True
            # Check all_except
            for pattern in self.all_except:
                if _matches_pattern(file_path, pattern):
                    return False
            return True

        # No patterns - match everything
        return True


@dataclass
class Pipeline:
    """Represents a pipeline configuration."""

    name: str
    entrypoint: str = ""  # Path to GitLab CI config file
    triggers: list[ChangesTrigger] = field(default_factory=list)

    def should_trigger(self, changed_files: list[str]) -> bool:
        """Check if this pipeline should be triggered by the changed files."""
        if not self.triggers:
            # No triggers means always run
            return True

        for trigger in self.triggers:
            for file_path in changed_files:
                if trigger.matches(file_path):
                    return True

        return False


@dataclass
class PipelinesConfig:
    """Configuration loaded from .pipelines/ folder or dd.pipelines.yml."""

    pipelines: list[Pipeline] = field(default_factory=list)

    @classmethod
    def _parse_pipeline_data(cls, data: dict) -> Pipeline:
        """Parse a single pipeline from YAML data."""
        triggers = []
        # YAML parses `on:` as boolean True, so check both "on" and True
        on_value = data.get("on") or data.get(True) or []
        for on_item in on_value:
            if "changes" in on_item:
                changes = on_item["changes"]
                include = []
                all_except = []

                for item in changes:
                    if isinstance(item, str):
                        # Plain string is a pattern to include
                        include.append(item)
                    elif isinstance(item, dict):
                        # `all_except` - match everything except these
                        if "all_except" in item:
                            all_except.extend(item["all_except"])
                        # `include` - only match these / override for all_except
                        if "include" in item:
                            include.extend(item["include"])

                triggers.append(ChangesTrigger(
                    include=include,
                    all_except=all_except,
                ))

        return Pipeline(
            name=data.get("name", ""),
            entrypoint=data.get("entrypoint", ""),
            triggers=triggers,
        )

    @classmethod
    def load_from_folder(cls, folder_path: Path) -> "PipelinesConfig":
        """Load pipeline configurations from a folder of YAML files.

        Each YAML file in the folder represents a single pipeline.
        Files should have .yml or .yaml extension.
        """
        if not folder_path.exists() or not folder_path.is_dir():
            return cls()

        pipelines = []
        for file_path in sorted(folder_path.iterdir()):
            if file_path.suffix not in (".yml", ".yaml"):
                continue

            with open(file_path) as f:
                data = yaml.safe_load(f) or {}

            if not data:
                continue

            # Use filename as name if not specified
            if "name" not in data:
                data["name"] = file_path.stem

            pipeline = cls._parse_pipeline_data(data)
            if pipeline.name:
                pipelines.append(pipeline)

        return cls(pipelines=pipelines)

    @classmethod
    def load(cls, path: Path) -> "PipelinesConfig":
        """Load configuration from dd.pipelines.yml (legacy format)."""
        if not path.exists():
            return cls()

        with open(path) as f:
            data = yaml.safe_load(f) or {}

        pipelines = []
        for p in data.get("pipelines", []):
            pipeline = cls._parse_pipeline_data(p)
            if pipeline.name:
                pipelines.append(pipeline)

        return cls(pipelines=pipelines)

    def get_triggered_pipelines(self, changed_files: list[str]) -> list[Pipeline]:
        """Get list of pipelines that should be triggered by the changed files."""
        if not changed_files:
            return self.pipelines

        return [p for p in self.pipelines if p.should_trigger(changed_files)]


def _matches_pattern(file_path: str, pattern: str) -> bool:
    """
    Check if a file path matches a pattern.

    Supports:
    - Exact paths: "path/to/file.yml"
    - Directory prefixes: "path/to/" matches anything under that directory
    - Glob patterns: "**/*.go" (recursive), "path/*.yml" (single level)

    Note: Single `*` does NOT match `/` (like shell globbing).
    Use `**` for recursive matching.
    """
    # Normalize pattern
    pattern = pattern.rstrip("/")

    # Handle ** glob pattern (recursive)
    if "**" in pattern:
        # "standalone-repo/**/*" should match "standalone-repo/foo/bar.go"
        parts = pattern.split("**")
        if len(parts) == 2:
            prefix = parts[0].rstrip("/")
            suffix = parts[1].lstrip("/")

            # Check prefix matches
            if prefix:
                if not (file_path.startswith(prefix + "/") or file_path == prefix):
                    return False
                remaining = file_path[len(prefix):].lstrip("/")
            else:
                remaining = file_path

            # No suffix or just "*" - match anything
            if not suffix or suffix == "*":
                return True

            # Check suffix matches (using fnmatch for wildcards)
            # The suffix applies to the filename part
            return fnmatch.fnmatch(remaining.split("/")[-1], suffix)

    # Standard glob pattern (single `*` should NOT match `/`)
    if "*" in pattern or "?" in pattern or "[" in pattern:
        # Split both pattern and path by `/` and match segment by segment
        pattern_parts = pattern.split("/")
        path_parts = file_path.split("/")

        # Must have same number of segments for non-recursive patterns
        if len(pattern_parts) != len(path_parts):
            return False

        # Match each segment
        for pp, fp in zip(pattern_parts, path_parts):
            if not fnmatch.fnmatch(fp, pp):
                return False
        return True

    # Directory prefix match
    if file_path.startswith(pattern + "/") or file_path == pattern:
        return True

    # Exact match
    return file_path == pattern


def get_changed_files(project_root: Path, compare_branch: str = "main") -> list[str]:
    """
    Get list of files changed compared to a branch.

    Args:
        project_root: Root path of the project.
        compare_branch: Branch to compare against.

    Returns:
        List of changed file paths (relative to project root).
    """
    try:
        # Get the merge base to compare against
        result = subprocess.run(
            ["git", "merge-base", "HEAD", f"origin/{compare_branch}"],
            cwd=project_root,
            capture_output=True,
            text=True,
            check=True,
        )
        merge_base = result.stdout.strip()

        # Get changed files
        result = subprocess.run(
            ["git", "diff", "--name-only", merge_base, "HEAD"],
            cwd=project_root,
            capture_output=True,
            text=True,
            check=True,
        )
        return [f for f in result.stdout.strip().split("\n") if f]
    except subprocess.CalledProcessError:
        # If git commands fail, return empty list
        return []


def get_pipelines_folder(project_root: Path) -> Path:
    """Get the default pipelines folder path."""
    return project_root / ".pipelines"


def get_default_pipelines_path(project_root: Path) -> Path:
    """Get the default pipelines configuration file path (legacy)."""
    return project_root / "dd.pipelines.yml"


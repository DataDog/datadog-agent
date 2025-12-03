# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Utilities for merging GitLab CI configuration files.
"""

from __future__ import annotations

import copy
import glob
from pathlib import Path
from typing import TYPE_CHECKING, Any

from ci_utils.pipelines import Pipeline
from ci_utils.yaml import load_yaml

if TYPE_CHECKING:
    from dda.cli.application import Application


def merge_pipeline_configs(
    pipelines: list[Pipeline],
    project_root: Path,
    should_resolve_includes: bool,
    should_resolve_extends: bool,
    app: Application,
) -> dict[str, Any]:
    """
    Merge GitLab CI configs from multiple pipelines.

    Each pipeline's entrypoint config is loaded and merged together.
    The merge strategy:
    - stages: concatenated and deduplicated (preserving order)
    - variables: deep merged (later pipelines override individual vars)
    - extends: optionally resolved (templates merged into jobs)
    - jobs: merged (later pipelines override)
    - other keys: merged (later pipelines override)
    """
    merged: dict[str, Any] = {}
    all_stages: list[str] = []
    all_variables: dict[str, Any] = {}

    for pipeline in pipelines:
        if not pipeline.entrypoint:
            app.display_warning(f"Pipeline '{pipeline.name}' has no entrypoint, skipping")
            continue

        entrypoint_path = project_root / Path(pipeline.entrypoint)
        if not entrypoint_path.exists():
            app.display_warning(
                f"Entrypoint '{pipeline.entrypoint}' for pipeline '{pipeline.name}' not found, skipping"
            )
            continue

        app.display_info(f"Loading config for '{pipeline.name}': {pipeline.entrypoint}")
        content = load_yaml(entrypoint_path)

        if not content:
            app.display_warning(f"Config for '{pipeline.name}' is empty, skipping")
            continue

        if should_resolve_includes:
            content = resolve_includes(content, entrypoint_path.parent, project_root)

        # Handle stages specially - collect and deduplicate
        if "stages" in content:
            for stage in content["stages"]:
                if stage not in all_stages:
                    all_stages.append(stage)
            del content["stages"]

        # Handle variables specially - merge them
        if "variables" in content:
            all_variables = deep_merge(all_variables, content["variables"])
            del content["variables"]

        # Merge the rest
        merged = deep_merge(
            merged, content, allow_override=False
        )  # We don't want to override variables with different values in different pipelines.

    # Add variables at the top
    if all_variables:
        merged["variables"] = all_variables

    # Add deduplicated stages
    if all_stages:
        merged["stages"] = all_stages

    # Resolve extends if requested
    if should_resolve_extends:
        app.display_info("Resolving extends directives...")
        merged = resolve_extends(merged)

    return merged


def deep_merge(base: dict[str, Any], override: dict[str, Any], allow_override: bool = True) -> dict[str, Any]:
    """
    Deep merge two dictionaries, with override taking precedence. Raise an error if a key is present in both dictionaries, and allow_override is False.
    We want to avoid silently overriding jobs with different configurations.

    Lists are concatenated (useful for merging stages from multiple pipelines).

    Args:
        base: Base dictionary.
        override: Dictionary to merge on top of base.

    Returns:
        Merged dictionary.
    """
    result = base.copy()
    for key, value in override.items():
        if not allow_override and key in result:
            raise ValueError(f"Key {key} is present in both dictionaries, cannot merge")
        if key in result and isinstance(result[key], dict) and isinstance(value, dict):
            result[key] = deep_merge(result[key], value)
        elif key in result and isinstance(result[key], list) and isinstance(value, list):
            # For lists, we concatenate (useful for stages, rules, etc.)
            result[key] = result[key] + value
        else:
            result[key] = value
    return result


def extends_merge(base: dict[str, Any], override: dict[str, Any]) -> dict[str, Any]:
    """
    Merge dictionaries following GitLab CI extends behavior.

    This is different from deep_merge:
    - Dictionaries are deep merged (e.g., variables)
    - Lists and other values are completely overridden by the job's value

    Args:
        base: Template configuration (lower priority).
        override: Job configuration (higher priority).

    Returns:
        Merged dictionary with job values taking precedence.
    """
    result = copy.deepcopy(base)
    for key, value in override.items():
        if key in result and isinstance(result[key], dict) and isinstance(value, dict):
            # Deep merge dictionaries (e.g., variables, artifacts)
            result[key] = extends_merge(result[key], value)
        else:
            # For lists and other values, job completely overrides template
            result[key] = copy.deepcopy(value)
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


def _resolve_single_extend(
    job_name: str,
    job_config: dict[str, Any],
    all_configs: dict[str, Any],
    resolved_cache: dict[str, dict[str, Any]],
    resolving_stack: set[str],
) -> dict[str, Any]:
    """
    Resolve extends for a single job, handling nested extends.

    Args:
        job_name: Name of the job being resolved.
        job_config: The job's configuration.
        all_configs: All configurations (jobs and templates).
        resolved_cache: Cache of already resolved configurations.
        resolving_stack: Set of job names currently being resolved (for cycle detection).

    Returns:
        The fully resolved job configuration.
    """
    # Check for circular extends
    if job_name in resolving_stack:
        raise ValueError(f"Circular extends detected: {job_name}")

    # Return cached result if available
    if job_name in resolved_cache:
        return copy.deepcopy(resolved_cache[job_name])

    # If no extends, return the config as-is
    if "extends" not in job_config:
        resolved_cache[job_name] = job_config
        return copy.deepcopy(job_config)

    resolving_stack.add(job_name)

    extends = job_config["extends"]
    if isinstance(extends, str):
        extends = [extends]

    # Start with empty config and merge all extended templates
    merged: dict[str, Any] = {}

    for template_name in extends:
        if template_name not in all_configs:
            raise ValueError(f"Template {template_name} not found, cannot merge")

        # Recursively resolve the template's extends first
        template_config = all_configs[template_name]
        resolved_template = _resolve_single_extend(
            template_name,
            template_config,
            all_configs,
            resolved_cache,
            resolving_stack,
        )

        # Merge the resolved template using extends_merge
        # Later templates override earlier ones
        merged = extends_merge(merged, resolved_template)

    # Remove extends from the job config and merge it on top
    # Job's own config has highest priority
    job_without_extends = {k: v for k, v in job_config.items() if k != "extends"}
    merged = extends_merge(merged, job_without_extends)

    resolving_stack.discard(job_name)
    resolved_cache[job_name] = merged

    return copy.deepcopy(merged)


def resolve_extends(content: dict[str, Any]) -> dict[str, Any]:
    """
    Resolve all `extends:` directives in the GitLab CI configuration.

    This merges template content into jobs, similar to what GitLab CI does
    at pipeline creation time. This makes variables and other configurations
    from templates directly available in each job.

    Args:
        content: The full GitLab CI configuration.

    Returns:
        Configuration with all extends resolved and merged.
    """
    result: dict[str, Any] = {}
    resolved_cache: dict[str, dict[str, Any]] = {}

    # First pass: copy non-job keys (variables, stages, default, workflow, etc.)
    # and identify all jobs/templates
    jobs_and_templates: dict[str, dict[str, Any]] = {}

    for key, value in content.items():
        if isinstance(value, dict):
            # This is a job or template
            jobs_and_templates[key] = value
        else:
            # Copy non-job content as-is (variables, stages, etc.)
            result[key] = value

    # Second pass: resolve extends for each job/template
    for name, config in jobs_and_templates.items():
        resolved = _resolve_single_extend(
            name,
            config,
            jobs_and_templates,
            resolved_cache,
            set(),
        )
        result[name] = resolved

    return result

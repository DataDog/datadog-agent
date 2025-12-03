# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Utilities for merging GitLab CI configuration files.
"""

from __future__ import annotations

import copy
from pathlib import Path
from typing import TYPE_CHECKING, Any

from ci_utils.pipelines import Pipeline

if TYPE_CHECKING:
    from dda.cli.application import Application

    from ci_utils.git import FileReader


def merge_pipeline_configs(
    pipelines: list[Pipeline],
    project_root: Path,
    should_resolve_includes: bool,
    should_resolve_extends: bool,
    app: Application,
    file_reader: FileReader | None = None,
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

    Args:
        pipelines: List of pipelines to merge.
        project_root: Root path of the project.
        should_resolve_includes: Whether to resolve include directives.
        should_resolve_extends: Whether to resolve extends directives.
        app: Application for logging.
        file_reader: Optional FileReader to read files (LocalFileReader or GitFileReader).
    """
    merged: dict[str, Any] = {}
    all_stages: list[str] = []
    all_variables: dict[str, Any] = {}

    # Use provided file_reader or create a local one
    if file_reader is None:
        from ci_utils.git import LocalFileReader

        file_reader = LocalFileReader(project_root)

    for pipeline in pipelines:
        if not pipeline.entrypoint:
            app.display_warning(f"Pipeline '{pipeline.name}' has no entrypoint, skipping")
            continue

        # Load content from file reader
        if not file_reader.file_exists(pipeline.entrypoint):
            app.display_warning(
                f"Entrypoint '{pipeline.entrypoint}' for pipeline '{pipeline.name}' not found ({file_reader.source_description}), skipping"
            )
            continue

        app.display_info(
            f"Loading config for '{pipeline.name}': {pipeline.entrypoint} ({file_reader.source_description})"
        )
        content = file_reader.load_yaml(pipeline.entrypoint)

        if not content:
            app.display_warning(f"Config for '{pipeline.name}' is empty, skipping")
            continue

        if should_resolve_includes:
            content = resolve_includes(content, Path(pipeline.entrypoint).parent, project_root, file_reader=file_reader)

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

    # Always resolve references after extends (they depend on the full config)
    app.display_info("Resolving !reference tags...")
    merged = resolve_references(merged)

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


def _resolve_include_path(
    include: str | dict,
    project_root: Path,
    file_reader: FileReader | None = None,
) -> list[str]:
    """
    Resolve an include directive to a list of file paths.

    Supports:
    - Simple string paths: "path/to/file.yml"
    - Local includes: {"local": "path/to/file.yml"}
    - Glob patterns: "path/to/*.yml" or {"local": "path/to/*.yml"}

    Args:
        include: Include directive (string or dict).
        project_root: Root path of the project.
        file_reader: FileReader for reading files.

    Returns:
        List of file paths (as strings, relative to project root).

    Raises:
        FileNotFoundError: If a local include file doesn't exist.
    """
    if isinstance(include, str):
        pattern = include.lstrip("/")
    elif isinstance(include, dict) and "local" in include:
        pattern = include["local"].lstrip("/")
    else:
        # Skip remote includes, project includes, etc.
        return []

    # Use provided file_reader or create a local one
    if file_reader is None:
        from ci_utils.git import LocalFileReader

        file_reader = LocalFileReader(project_root)

    # Check if it's a glob pattern
    if "*" in pattern or "?" in pattern or "[" in pattern:
        # List all files and filter manually
        all_files = file_reader.list_files()
        import fnmatch

        matched = [f for f in all_files if fnmatch.fnmatch(f, pattern)]
        return sorted(matched)
    else:
        if not file_reader.file_exists(pattern):
            raise FileNotFoundError(f"Include file not found ({file_reader.source_description}): {pattern}")
        return [pattern]


def resolve_includes(
    content: dict[str, Any],
    base_path: Path,
    project_root: Path,
    already_included: set[str] | None = None,
    file_reader: FileReader | None = None,
) -> dict[str, Any]:
    """
    Recursively resolve all include directives in the YAML content.

    Supports glob patterns like "path/to/*.yml".
    Tracks already-included files to avoid duplicates.

    Args:
        content: YAML content dictionary.
        base_path: Path of the current file being processed.
        project_root: Root path of the project.
        already_included: Set of file paths already included (for deduplication).
        file_reader: FileReader for reading files (LocalFileReader or GitFileReader).

    Returns:
        Merged content with all includes resolved.
    """
    if already_included is None:
        already_included = set()

    # Use provided file_reader or create a local one
    if file_reader is None:
        from ci_utils.git import LocalFileReader

        file_reader = LocalFileReader(project_root)

    if "include" not in content:
        return content

    includes = content.pop("include")
    if not isinstance(includes, list):
        includes = [includes]

    merged: dict[str, Any] = {}

    for include in includes:
        include_paths = _resolve_include_path(include, project_root, file_reader)

        for include_path in include_paths:
            # Skip already-included files to avoid duplicates
            if include_path in already_included:
                continue
            already_included.add(include_path)

            # Load the included file
            included_content = file_reader.load_yaml(include_path)

            # Recursively resolve includes in the included file
            included_content = resolve_includes(
                included_content,
                Path(include_path).parent,
                project_root,
                already_included,
                file_reader,
            )

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


def _get_reference_value(content: dict[str, Any], path: list[str]) -> Any:
    """
    Get a value from the content by following a path.

    Args:
        content: The full configuration.
        path: List of keys to follow (e.g., [".template", "script"]).

    Returns:
        The value at the path.

    Raises:
        KeyError: If the path doesn't exist.
    """
    current = content
    for key in path:
        if isinstance(current, dict) and key in current:
            current = current[key]
        else:
            raise KeyError(f"Reference path not found: {path}")
    return current


def _resolve_references_recursive(value: Any, content: dict[str, Any]) -> Any:
    """
    Recursively resolve !reference tags in a value.

    Args:
        value: The value to process (can be dict, list, or scalar).
        content: The full configuration (for looking up references).

    Returns:
        The value with all !reference tags resolved.
    """
    from ci_utils.yaml import GitLabReference

    if isinstance(value, GitLabReference):
        # Resolve the reference
        try:
            resolved = _get_reference_value(content, value.value)
            # Recursively resolve any nested references in the resolved value
            return _resolve_references_recursive(copy.deepcopy(resolved), content)
        except KeyError:
            # If reference can't be resolved, keep it as-is (GitLab will handle it)
            return value

    elif isinstance(value, dict):
        return {k: _resolve_references_recursive(v, content) for k, v in value.items()}

    elif isinstance(value, list):
        result = []
        for item in value:
            resolved_item = _resolve_references_recursive(item, content)
            # If the resolved item is a list (from a reference), flatten it
            if isinstance(item, GitLabReference) and isinstance(resolved_item, list):
                result.extend(resolved_item)
            else:
                result.append(resolved_item)
        return result

    else:
        return value


def resolve_references(content: dict[str, Any]) -> dict[str, Any]:
    """
    Resolve all `!reference` tags in the GitLab CI configuration.

    This replaces !reference tags with the actual values they reference,
    similar to what GitLab CI does at pipeline creation time.

    Example:
        Input:
            .template:
              script:
                - echo "hello"
            job:
              script:
                - !reference [.template, script]

        Output:
            .template:
              script:
                - echo "hello"
            job:
              script:
                - echo "hello"

    Args:
        content: The full GitLab CI configuration.

    Returns:
        Configuration with all !reference tags resolved.
    """
    return _resolve_references_recursive(content, content)

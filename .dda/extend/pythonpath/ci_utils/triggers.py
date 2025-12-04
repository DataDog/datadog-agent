# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Utilities for finding and resolving triggered pipelines in GitLab CI configurations.
"""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from dda.cli.application import Application

    from ci_utils.git import FileReader


def get_trigger_filenames(node: str | dict | list | None) -> list[str]:
    """
    Gets all trigger downstream pipelines defined by the `trigger` key in a gitlab-ci configuration.

    Args:
        node: The value of the 'include' key within a 'trigger' definition

    Returns:
        List of local file paths for triggered pipelines
    """
    if isinstance(node, str):
        return [node]
    elif isinstance(node, dict):
        return [node['local']] if 'local' in node else []
    elif isinstance(node, list):
        res = []
        for n in node:
            res.extend(get_trigger_filenames(n))
        return res
    return []


def find_triggered_pipelines(config: dict) -> list[str]:
    """
    Find all triggered pipeline files from a GitLab CI configuration.

    Args:
        config: A resolved GitLab CI configuration dict

    Returns:
        List of file paths for triggered pipelines
    """
    triggered = []

    for job in config.values():
        if not isinstance(job, dict):
            continue

        if 'trigger' in job and 'include' in job['trigger']:
            triggered.extend(get_trigger_filenames(job['trigger']['include']))

    return triggered


def get_all_triggered_configurations(
    main_config: dict,
    file_reader: FileReader,
    project_root: Path,
    app: Application | None = None,
) -> dict[str, dict]:
    """
    Get all configurations including triggered pipelines.

    Traverses the main config, finds all triggered pipelines, and resolves
    each one recursively.

    Args:
        main_config: The main/root GitLab CI configuration
        file_reader: FileReader for reading pipeline files
        project_root: Root path of the project
        app: Application instance for logging (optional)

    Returns:
        Dictionary mapping entry points to their resolved configurations.
        The main config is stored under "main", triggered pipelines under their file path.
    """
    from ci_utils.merge import resolve_extends, resolve_includes, resolve_references

    configurations: dict[str, dict] = {"main": main_config}

    # Queue of files to process
    to_process = find_triggered_pipelines(main_config)
    processed: set[str] = set()

    while to_process:
        trigger_file = to_process.pop(0)

        if trigger_file in processed:
            continue
        processed.add(trigger_file)

        if app:
            app.display_info(f"  Resolving triggered pipeline: {trigger_file}")

        try:
            # Load the triggered pipeline config
            trigger_config = file_reader.load_yaml(trigger_file)

            if not trigger_config:
                if app:
                    app.display_warning(f"    Empty or missing config: {trigger_file}")
                continue

            # Resolve includes
            trigger_config = resolve_includes(trigger_config, project_root, project_root, file_reader=file_reader)

            # Resolve extends
            trigger_config = resolve_extends(trigger_config)

            # Resolve references
            trigger_config = resolve_references(trigger_config)

            configurations[trigger_file] = trigger_config

            # Find nested triggered pipelines
            nested_triggers = find_triggered_pipelines(trigger_config)
            for nested in nested_triggers:
                if nested not in processed:
                    to_process.append(nested)

        except FileNotFoundError:
            if app:
                app.display_warning(f"    Triggered pipeline file not found: {trigger_file}")
        except Exception as e:
            if app:
                app.display_warning(f"    Error resolving {trigger_file}: {e}")

    return configurations

# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

import click
from ci_utils import (
    Pipeline,
    PipelinesConfig,
    dump_yaml,
    get_default_pipelines_path,
    get_pipelines_folder,
    merge_pipeline_configs,
)
from dda.cli.base import dynamic_command, pass_app
from utils.changes import get_changed_files

if TYPE_CHECKING:
    from dda.cli.application import Application


def find_project_root(start: Path) -> Path | None:
    """Find the project root by looking for .pipelines folder"""
    current = start
    while current != current.parent:
        if (current / ".pipelines").exists():
            return current
        current = current.parent
    return None


def load_pipelines_config(project_root: Path) -> PipelinesConfig:
    """Load pipelines config from .pipelines folder or dd.pipelines.yml (legacy)."""
    # Prefer .pipelines folder
    pipelines_folder = get_pipelines_folder(project_root)
    if pipelines_folder.exists() and pipelines_folder.is_dir():
        return PipelinesConfig.load_from_folder(pipelines_folder)

    # Fallback to legacy dd.pipelines.yml
    legacy_path = get_default_pipelines_path(project_root)
    if legacy_path.exists():
        return PipelinesConfig.load(legacy_path)

    return PipelinesConfig()


@dynamic_command(
    short_help="Generate CI pipeline",
)
@click.option(
    "--output",
    "-o",
    "output_file",
    type=str,
    default="generated-pipeline.yml",
    help="Output file for the generated pipeline (default: generated-pipeline.yml)",
)
@click.option(
    "--resolve-includes/--no-resolve-includes",
    "should_resolve_includes",
    default=True,
    help="Whether to resolve local include directives (default: true)",
)
@click.option(
    "--pipeline",
    "-p",
    "pipeline_names",
    type=str,
    multiple=True,
    help="Generate only specific pipeline(s) from dd.pipelines.yml (can be specified multiple times)",
)
@click.option(
    "--filter-by-changes",
    is_flag=True,
    default=False,
    help="Only generate pipelines affected by changed files (uses dd.pipelines.yml)",
)
@click.option(
    "--compare-branch",
    type=str,
    default="main",
    help="Branch to compare against for change detection (default: main)",
)
@click.option(
    "--show-pipelines",
    is_flag=True,
    default=False,
    help="Show which pipelines would be triggered without generating",
)
@click.option(
    "--all",
    "generate_all",
    is_flag=True,
    default=False,
    help="Generate all pipelines merged together",
)
@click.option(
    "--resolve-extends/--no-resolve-extends",
    "should_resolve_extends",
    default=True,
    help="Whether to resolve extends directives, merging templates into jobs (default: true)",
)
@pass_app
def cmd(
    app: Application,
    *,
    output_file: str,
    should_resolve_includes: bool,
    pipeline_names: tuple[str, ...],
    filter_by_changes: bool,
    compare_branch: str,
    show_pipelines: bool,
    generate_all: bool,
    should_resolve_extends: bool,
) -> None:
    """
    Generate CI pipeline(s) from dd.pipelines.yml.

    This command reads pipeline configurations and merges their GitLab CI
    entrypoint files into a single pipeline.

    Examples:

    \b
    # Generate all pipelines merged together
    dda ci generate --all

    \b
    # Generate a specific pipeline
    dda ci generate --pipeline main

    \b
    # Generate multiple specific pipelines merged
    dda ci generate --pipeline main --pipeline standalone-repo

    \b
    # Show which pipelines would be triggered by your changes
    dda ci generate --show-pipelines

    \b
    # Generate only pipelines affected by changes
    dda ci generate --filter-by-changes

    \b
    # Generate without resolving includes
    dda ci generate --no-resolve-includes
    """

    # Find project root
    project_root = find_project_root(Path.cwd())
    if not project_root:
        project_root = Path.cwd()

    # Load pipelines configuration
    pipelines_config = load_pipelines_config(project_root)

    if not pipelines_config.pipelines:
        app.abort("No pipelines found. Create .pipelines/*.yml files to define your pipelines.")

    # Handle --show-pipelines
    if show_pipelines:
        changed_files = get_changed_files(app, project_root)

        if changed_files:
            app.display_info(f"Found {len(changed_files)} changed files:")
            for f in changed_files[:10]:
                app.display_info(f"  • {f}")
            if len(changed_files) > 10:
                app.display_info(f"  ... and {len(changed_files) - 10} more")
            app.display_info("")

        triggered = pipelines_config.get_triggered_pipelines(changed_files)

        app.display_info("Pipelines that would be triggered:")
        for p in triggered:
            entrypoint = f" ({p.entrypoint})" if p.entrypoint else ""
            app.display_success(f"  ✓ {p.name}{entrypoint}")

        not_triggered = [p for p in pipelines_config.pipelines if p not in triggered]
        if not_triggered:
            app.display_info("")
            app.display_info("Pipelines that would NOT be triggered:")
            for p in not_triggered:
                app.display_info(f"  ✗ {p.name}")
        return

    # Determine which pipelines to generate
    pipelines_to_generate: list[Pipeline] = []

    if generate_all:
        pipelines_to_generate = pipelines_config.pipelines
        app.display_info("Generating all pipelines")
    elif pipeline_names:
        for name in pipeline_names:
            pipeline = next((p for p in pipelines_config.pipelines if p.name == name), None)
            if not pipeline:
                available = ", ".join(p.name for p in pipelines_config.pipelines)
                app.abort(f"Pipeline '{name}' not found. Available: {available}")
            pipelines_to_generate.append(pipeline)
        app.display_info(f"Generating pipelines: {', '.join(pipeline_names)}")
    elif filter_by_changes:
        app.display_info(f"Detecting changes compared to {compare_branch}...")
        changed_files = get_changed_files(app, project_root)

        if changed_files:
            app.display_info(f"Found {len(changed_files)} changed files")

        pipelines_to_generate = pipelines_config.get_triggered_pipelines(changed_files)
        triggered_names = [p.name for p in pipelines_to_generate]

        if not pipelines_to_generate:
            app.display_warning("No pipelines triggered by changes")
            return

        app.display_info(f"Triggered pipelines: {', '.join(triggered_names)}")
    else:
        click.echo(click.get_current_context().get_help())
        return

    # Merge configs from all pipelines to generate
    app.display_info("")
    content = merge_pipeline_configs(
        pipelines_to_generate,
        project_root,
        should_resolve_includes,
        should_resolve_extends,
        app,
    )

    if not content:
        app.abort("No content to generate - all pipeline configs are empty or missing")

    output_path = project_root / output_file

    header_lines = [
        "Generated by: dda ci generate",
        f"Pipelines: {', '.join(p.name for p in pipelines_to_generate)}",
        "DO NOT EDIT - This file is auto-generated",
    ]

    dump_yaml(content, output_path, header="\n".join(header_lines))

    app.display_info("")
    app.display_success(f"Generated pipeline written to {output_path}")

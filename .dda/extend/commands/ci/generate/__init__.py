# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

import click
from dda.cli.base import dynamic_command, pass_app

from ci import (
    Pipeline,
    PipelinesConfig,
    apply_job_injections,
    deep_merge,
    dump_yaml,
    get_changed_files,
    get_default_pipelines_path,
    get_pipelines_folder,
    load_yaml,
    resolve_extends,
    resolve_includes,
)

if TYPE_CHECKING:
    from dda.cli.application import Application


def find_project_root(start: Path) -> Path | None:
    """Find the project root by looking for .pipelines folder or dd.pipelines.yml."""
    current = start
    while current != current.parent:
        if (current / ".pipelines").exists() or (current / "dd.pipelines.yml").exists():
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
    - inject: collected from all pipelines and applied to all jobs
    - jobs: merged (later pipelines override)
    - other keys: merged (later pipelines override)
    """
    merged: dict[str, Any] = {}
    all_stages: list[str] = []
    all_variables: dict[str, Any] = {}

    # Collect injections from all pipelines
    all_before_script: list[str] = []
    all_after_script: list[str] = []
    all_needs: list[str | dict] = []
    all_inject_variables: dict[str, Any] = {}
    all_tags: list[str] = []

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

        # Collect injection config from this pipeline
        if pipeline.inject:
            inj = pipeline.inject
            if inj.before_script:
                all_before_script.extend(inj.before_script)
            if inj.after_script:
                all_after_script.extend(inj.after_script)
            if inj.needs:
                all_needs.extend(inj.needs)
            if inj.variables:
                all_inject_variables = deep_merge(all_inject_variables, inj.variables)
            if inj.tags:
                all_tags.extend(inj.tags)

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
        merged = deep_merge(merged, content)

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

    # Apply injections to all jobs
    has_injections = any([all_before_script, all_after_script, all_needs, all_inject_variables, all_tags])
    if has_injections:
        app.display_info("Applying job injections...")
        merged = apply_job_injections(
            merged,
            before_script=all_before_script or None,
            after_script=all_after_script or None,
            needs=all_needs or None,
            variables=all_inject_variables or None,
            tags=all_tags or None,
        )

    return merged


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
        app.display_info(f"Detecting changes compared to {compare_branch}...")
        changed_files = get_changed_files(project_root, compare_branch)

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
        changed_files = get_changed_files(project_root, compare_branch)

        if changed_files:
            app.display_info(f"Found {len(changed_files)} changed files")

        pipelines_to_generate = pipelines_config.get_triggered_pipelines(changed_files)
        triggered_names = [p.name for p in pipelines_to_generate]

        if not pipelines_to_generate:
            app.display_warning("No pipelines triggered by changes")
            return

        app.display_info(f"Triggered pipelines: {', '.join(triggered_names)}")
    else:
        # Default: show help
        app.display_info("Usage: dda ci generate [OPTIONS]")
        app.display_info("")
        app.display_info("Options:")
        app.display_info("  --all                 Generate all pipelines merged together")
        app.display_info("  --pipeline, -p NAME   Generate specific pipeline(s)")
        app.display_info("  --filter-by-changes   Generate pipelines affected by your changes")
        app.display_info("  --show-pipelines      Show which pipelines would be triggered")
        app.display_info("")
        app.display_info("Available pipelines:")
        for p in pipelines_config.pipelines:
            entrypoint = f" → {p.entrypoint}" if p.entrypoint else ""
            app.display_info(f"  • {p.name}{entrypoint}")
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

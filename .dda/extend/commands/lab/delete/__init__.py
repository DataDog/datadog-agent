# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Delete a lab environment")
@click.option(
    "--id",
    "-i",
    default=None,
    help="Environment id to delete (interactive if not provided)",
)
@click.option("--yes", "-y", is_flag=True, help="Auto-accept confirmation prompt")
@pass_app
def cmd(app: Application, *, id: str | None, yes: bool) -> None:
    """
    Delete a lab environment.

    Works with any environment type (kind, gke, eks, etc.).
    The provider-specific cleanup is handled automatically.

    If --id is not provided, shows an interactive selection menu.
    """
    from lab import LabEnvironment
    from lab.providers import get_provider

    # If no name provided, show interactive selection
    if id is None:
        environments = LabEnvironment.load_all(app)
        if not environments:
            app.display_info("No lab environments found.")
            return

        choices = [f"{e.name} ({e.env_type})" for e in environments]

        app.display_info("Select an environment to delete:\n")
        for i, choice in enumerate(choices, 1):
            app.display_info(f"  {i}. {choice}")
        app.display_info("")

        selection = app.prompt(
            "Enter number",
            type=click.IntRange(1, len(choices)),
        )
        id = environments[selection - 1].name

    try:
        env = LabEnvironment.load(app, id)
    except Exception as exception:
        app.abort(f"Error loading environment '{id}': {exception}")
        return

    if env is None:
        app.display_error(f"Environment '{id}' not found.")

        environments = LabEnvironment.load_all(app)
        if environments:
            app.display_info("\nAvailable environments:")
            for e in environments:
                app.display_info(f"  - {e.name} ({e.env_type})")
        return

    if not yes:
        if not click.confirm(f"Delete {env.env_type} environment '{id}'?"):
            app.display_info("Aborting.")
            return

    app.display_info(f"Deleting {env.env_type} environment '{id}'...")

    from lab.providers import ProviderConfig

    try:
        provider = get_provider(env.env_type)
        config = ProviderConfig(name=id)
        options = provider.options_class.from_config(config)
        missing = [p for p in provider.check_prerequisites(app, options) if "delete" in p.actions]
        if missing:
            lines = ["Missing prerequisites:"]
            for prereq in missing:
                lines.append(f"  • {prereq.name}")
                lines.append(f"    → {prereq.remediation}")
            app.abort("\n".join(lines))

        provider.destroy(app, id)
    except ValueError:
        app.display_warning(f"Provider '{env.env_type}' not found, removing from storage only.")

    try:
        env.delete()
    except FileNotFoundError:
        app.display_error(f"Environment '{id}' not found.")
        return

    app.display_success(f"Environment '{id}' deleted.")

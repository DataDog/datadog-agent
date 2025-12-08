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
@click.option("--name", "-n", default=None, help="Name of the environment to delete (interactive if not provided)")
@click.option("--force", "-f", is_flag=True, help="Skip confirmation prompt")
@pass_app
def cmd(app: Application, *, name: str | None, force: bool) -> None:
    """
    Delete a lab environment.

    Works with any environment type (kind, gke, eks, etc.).
    The provider-specific cleanup is handled automatically.

    If --name is not provided, shows an interactive selection menu.
    """
    from lab import LabEnvironment
    from lab.providers import get_provider

    # If no name provided, show interactive selection
    if name is None:
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
        name = environments[selection - 1].name

    env = LabEnvironment.load(app, name)

    if env is None:
        app.display_error(f"Environment '{name}' not found.")

        environments = LabEnvironment.load_all(app)
        if environments:
            app.display_info("\nAvailable environments:")
            for e in environments:
                app.display_info(f"  - {e.name} ({e.env_type})")
        return

    if not force:
        if not click.confirm(f"Delete {env.env_type} environment '{name}'?"):
            app.display_info("Aborting.")
            return

    app.display_info(f"Deleting {env.env_type} environment '{name}'...")

    try:
        provider = get_provider(env.env_type)
        provider.destroy(app, name)
    except ValueError:
        app.display_warning(f"Provider '{env.env_type}' not found, removing from storage only.")

    env.delete()
    app.display_success(f"Environment '{name}' deleted.")

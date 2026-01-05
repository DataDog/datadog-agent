# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Create an installer lab on AWS",
)
@click.option(
    "--debug/--no-debug",
    default=False,
    help="Enable Pulumi debug mode",
)
@click.option(
    "--pipeline-id",
    help="Pipeline ID of the Agent build to install",
)
@click.option(
    "--site",
    default="datad0g.com",
    help="Datadog site to contact (default: 'datad0g.com')",
)
@click.option(
    "--agent-flavor",
    help="Agent flavor (e.g., 'datadog-fips-agent')",
)
@pass_app
def cmd(
    app: Application,
    *,
    debug: bool,
    pipeline_id: str | None,
    site: str,
    agent_flavor: str | None,
) -> None:
    """
    Create an installer lab environment on AWS.

    This command provisions an environment for testing the Datadog Installer.

    Examples:

        # Create a default installer lab
        dda lab aws installer create

        # Create an installer lab with a specific pipeline
        dda lab aws installer create --pipeline-id 12345678

        # Create an installer lab targeting a specific site
        dda lab aws installer create --site datadoghq.com
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.installer import create_installer_lab as create_installer_task

    ctx = Context()

    app.display_info("Creating installer lab on AWS...")

    try:
        create_installer_task(
            ctx,
            debug=debug,
            pipeline_id=pipeline_id,
            site=site,
            agent_flavor=agent_flavor,
        )
        app.display_success("Installer lab created successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to create installer lab (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to create installer lab: {e}")


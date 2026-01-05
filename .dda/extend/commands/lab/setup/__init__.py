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
    short_help="Setup your local environment for lab commands",
)
@click.option(
    "--config-path",
    type=click.Path(exists=False),
    help="Path to custom config file (default: ~/.test_infra_config.yaml)",
)
@click.option(
    "--interactive/--non-interactive",
    default=True,
    help="Enable interactive mode (default: True)",
)
@click.option(
    "--debug/--no-debug",
    default=False,
    help="Run debug checks after setup",
)
@pass_app
def cmd(
    app: Application,
    *,
    config_path: str | None,
    interactive: bool,
    debug: bool,
) -> None:
    """
    Setup your local environment for using lab commands.

    This interactive command will:
    - Check and install required tools (Pulumi, AWS CLI, Azure CLI, GCloud CLI)
    - Configure your AWS, Azure, and GCP credentials
    - Setup SSH key pairs for VM access
    - Configure Datadog API and APP keys

    Run this command before using any other lab commands.

    Examples:

        # Interactive setup (recommended for first-time users)
        dda lab setup

        # Non-interactive setup (for CI/automation)
        dda lab setup --non-interactive

        # Setup with debug checks
        dda lab setup --debug
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.setup import setup as setup_task

    ctx = Context()

    app.display_info("Setting up lab environment...")

    try:
        setup_task(
            ctx,
            config_path=config_path,
            interactive=interactive,
            debug=debug,
        )
        app.display_success("Lab environment setup completed!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Setup failed (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Setup failed: {e}")


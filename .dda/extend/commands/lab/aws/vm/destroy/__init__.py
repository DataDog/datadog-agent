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
    short_help="Destroy an AWS virtual machine",
)
@click.option(
    "--config-path",
    type=click.Path(exists=False),
    help="Path to custom config file (default: ~/.test_infra_config.yaml)",
)
@click.option(
    "--stack-name",
    help="Name of the stack to destroy",
)
@click.option(
    "--clean-known-hosts/--no-clean-known-hosts",
    default=True,
    help="Remove host from SSH known_hosts file (default: True)",
)
@pass_app
def cmd(
    app: Application,
    *,
    config_path: str | None,
    stack_name: str | None,
    clean_known_hosts: bool,
) -> None:
    """
    Destroy an AWS EC2 virtual machine.

    This command destroys the Pulumi stack and associated AWS resources.

    Examples:

        # Destroy the default VM stack
        dda lab aws vm destroy

        # Destroy a specific stack
        dda lab aws vm destroy --stack-name my-custom-vm

        # Destroy without cleaning known_hosts
        dda lab aws vm destroy --no-clean-known-hosts
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.vm import destroy_vm as destroy_vm_task

    ctx = Context()

    app.display_info("Destroying AWS VM...")

    try:
        destroy_vm_task(
            ctx,
            config_path=config_path,
            stack_name=stack_name,
            clean_known_hosts=clean_known_hosts,
        )
        app.display_success("AWS VM destroyed successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to destroy VM (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to destroy VM: {e}")


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
    short_help="Show AWS virtual machine details",
)
@click.option(
    "--stack-name",
    help="Name of the stack to show",
)
@pass_app
def cmd(
    app: Application,
    *,
    stack_name: str | None,
) -> None:
    """
    Show connection details for an AWS EC2 virtual machine.

    Displays the IP address, username, and SSH command to connect to the VM.

    Examples:

        # Show details of the default VM
        dda lab aws vm show

        # Show details of a specific stack
        dda lab aws vm show --stack-name my-custom-vm
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.vm import show_vm as show_vm_task

    ctx = Context()

    try:
        show_vm_task(ctx, stack_name=stack_name)
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to show VM details (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to show VM details: {e}")


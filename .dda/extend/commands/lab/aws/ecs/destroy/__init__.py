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
    short_help="Destroy an AWS ECS cluster",
)
@click.option(
    "--stack-name",
    help="Name of the stack to destroy",
)
@pass_app
def cmd(
    app: Application,
    *,
    stack_name: str | None,
) -> None:
    """
    Destroy an AWS ECS cluster.

    This command destroys the Pulumi stack and all associated AWS resources.

    Examples:

        # Destroy the default ECS stack
        dda lab aws ecs destroy

        # Destroy a specific stack
        dda lab aws ecs destroy --stack-name my-custom-ecs
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.ecs import destroy_ecs as destroy_ecs_task

    ctx = Context()

    app.display_info("Destroying AWS ECS cluster...")

    try:
        destroy_ecs_task(ctx, stack_name=stack_name)
        app.display_success("AWS ECS cluster destroyed successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to destroy ECS cluster (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to destroy ECS cluster: {e}")


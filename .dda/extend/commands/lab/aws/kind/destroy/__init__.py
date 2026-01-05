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
    short_help="Destroy a Kind cluster on AWS",
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
@pass_app
def cmd(
    app: Application,
    *,
    config_path: str | None,
    stack_name: str | None,
) -> None:
    """
    Destroy a Kind cluster on AWS.

    This command destroys the Pulumi stack and all associated AWS resources.

    Examples:

        # Destroy the default Kind stack
        dda lab aws kind destroy

        # Destroy a specific stack
        dda lab aws kind destroy --stack-name my-custom-kind
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.kind import destroy_kind as destroy_kind_task

    ctx = Context()

    app.display_info("Destroying Kind cluster on AWS...")

    try:
        destroy_kind_task(ctx, config_path=config_path, stack_name=stack_name)
        app.display_success("Kind cluster destroyed successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to destroy Kind cluster (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to destroy Kind cluster: {e}")


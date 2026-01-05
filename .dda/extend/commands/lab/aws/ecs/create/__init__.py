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
    short_help="Create an AWS ECS cluster",
)
@click.option(
    "--config-path",
    type=click.Path(exists=False),
    help="Path to custom config file (default: ~/.test_infra_config.yaml)",
)
@click.option(
    "--stack-name",
    help="Custom name for the stack",
)
@click.option(
    "--install-agent/--no-install-agent",
    default=True,
    help="Install the Datadog Agent (default: True)",
)
@click.option(
    "--install-workload/--no-install-workload",
    default=True,
    help="Install test workload (default: True)",
)
@click.option(
    "--agent-version",
    help="Container Agent version (e.g., '7.50.0-rc.3')",
)
@click.option(
    "--use-fargate/--no-fargate",
    default=True,
    help="Use Fargate capacity provider (default: True)",
)
@click.option(
    "--linux-node-group/--no-linux-node-group",
    default=True,
    help="Include Linux ECS-optimized node group (default: True)",
)
@click.option(
    "--linux-arm-node-group/--no-linux-arm-node-group",
    default=False,
    help="Include Linux ARM ECS-optimized node group (default: False)",
)
@click.option(
    "--bottlerocket-node-group/--no-bottlerocket-node-group",
    default=True,
    help="Include Bottlerocket node group (default: True)",
)
@click.option(
    "--windows-node-group/--no-windows-node-group",
    default=False,
    help="Include Windows LTSC node group (default: False)",
)
@click.option(
    "--full-image-path",
    help="Full image path (registry:tag) of the Agent image",
)
@click.option(
    "--agent-flavor",
    help="Agent flavor (e.g., 'datadog-fips-agent')",
)
@click.option(
    "--agent-env",
    help="Extra environment variables (format: 'VAR1=val1,VAR2=val2')",
)
@pass_app
def cmd(
    app: Application,
    *,
    config_path: str | None,
    stack_name: str | None,
    install_agent: bool,
    install_workload: bool,
    agent_version: str | None,
    use_fargate: bool,
    linux_node_group: bool,
    linux_arm_node_group: bool,
    bottlerocket_node_group: bool,
    windows_node_group: bool,
    full_image_path: str | None,
    agent_flavor: str | None,
    agent_env: str | None,
) -> None:
    """
    Create an AWS ECS (Elastic Container Service) cluster for testing.

    This command provisions an ECS cluster using Pulumi.

    Examples:

        # Create a default ECS cluster
        dda lab aws ecs create

        # Create an ECS cluster with Windows nodes
        dda lab aws ecs create --windows-node-group

        # Create an ECS cluster without Fargate
        dda lab aws ecs create --no-fargate
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.ecs import create_ecs as create_ecs_task

    ctx = Context()

    app.display_info("Creating AWS ECS cluster...")

    try:
        create_ecs_task(
            ctx,
            config_path=config_path,
            stack_name=stack_name,
            install_agent=install_agent,
            install_workload=install_workload,
            agent_version=agent_version,
            use_fargate=use_fargate,
            linux_node_group=linux_node_group,
            linux_arm_node_group=linux_arm_node_group,
            bottlerocket_node_group=bottlerocket_node_group,
            windows_node_group=windows_node_group,
            full_image_path=full_image_path,
            agent_flavor=agent_flavor,
            agent_env=agent_env,
        )
        app.display_success("AWS ECS cluster created successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to create ECS cluster (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to create ECS cluster: {e}")


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
    short_help="Create an AWS EKS cluster",
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
    "--debug/--no-debug",
    default=False,
    help="Enable Pulumi debug mode",
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
    "--install-argorollout/--no-install-argorollout",
    default=False,
    help="Install Argo Rollout (default: False)",
)
@click.option(
    "--agent-version",
    help="Container Agent version (e.g., '7.50.0-rc.3')",
)
@click.option(
    "--linux-node-group/--no-linux-node-group",
    default=True,
    help="Include Linux x86_64 node group (default: True)",
)
@click.option(
    "--linux-arm-node-group/--no-linux-arm-node-group",
    default=False,
    help="Include Linux ARM64 node group (default: False)",
)
@click.option(
    "--bottlerocket-node-group/--no-bottlerocket-node-group",
    default=True,
    help="Include Bottlerocket node group (default: True)",
)
@click.option(
    "--windows-node-group/--no-windows-node-group",
    default=False,
    help="Include Windows node group (default: False)",
)
@click.option(
    "--instance-type",
    help="EC2 instance type for nodes",
)
@click.option(
    "--full-image-path",
    help="Full image path (registry:tag) of the Agent image",
)
@click.option(
    "--cluster-agent-full-image-path",
    help="Full image path (registry:tag) of the Cluster Agent image",
)
@click.option(
    "--agent-flavor",
    help="Agent flavor (e.g., 'datadog-fips-agent')",
)
@click.option(
    "--helm-config",
    type=click.Path(exists=True),
    help="Path to custom helm config file to merge with defaults",
)
@click.option(
    "--local-chart-path",
    type=click.Path(exists=True),
    help="Path to local helm chart for the Datadog Agent",
)
@pass_app
def cmd(
    app: Application,
    *,
    config_path: str | None,
    stack_name: str | None,
    debug: bool,
    install_agent: bool,
    install_workload: bool,
    install_argorollout: bool,
    agent_version: str | None,
    linux_node_group: bool,
    linux_arm_node_group: bool,
    bottlerocket_node_group: bool,
    windows_node_group: bool,
    instance_type: str | None,
    full_image_path: str | None,
    cluster_agent_full_image_path: str | None,
    agent_flavor: str | None,
    helm_config: str | None,
    local_chart_path: str | None,
) -> None:
    """
    Create an AWS EKS (Elastic Kubernetes Service) cluster for testing.

    This command provisions an EKS cluster using Pulumi. The cluster creation
    takes approximately 20 minutes.

    Examples:

        # Create a default EKS cluster
        dda lab aws eks create

        # Create an EKS cluster with Windows nodes
        dda lab aws eks create --windows-node-group

        # Create an EKS cluster with ARM nodes
        dda lab aws eks create --linux-arm-node-group

        # Create an EKS cluster with custom Agent version
        dda lab aws eks create --agent-version 7.50.0-rc.3
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.eks import create_eks as create_eks_task

    ctx = Context()

    app.display_info("Creating AWS EKS cluster (this takes ~20 minutes)...")

    try:
        create_eks_task(
            ctx,
            config_path=config_path,
            debug=debug,
            stack_name=stack_name,
            install_agent=install_agent,
            install_workload=install_workload,
            install_argorollout=install_argorollout,
            agent_version=agent_version,
            linux_node_group=linux_node_group,
            linux_arm_node_group=linux_arm_node_group,
            bottlerocket_node_group=bottlerocket_node_group,
            windows_node_group=windows_node_group,
            instance_type=instance_type,
            full_image_path=full_image_path,
            cluster_agent_full_image_path=cluster_agent_full_image_path,
            agent_flavor=agent_flavor,
            helm_config=helm_config,
            local_chart_path=local_chart_path,
        )
        app.display_success("AWS EKS cluster created successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to create EKS cluster (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to create EKS cluster: {e}")


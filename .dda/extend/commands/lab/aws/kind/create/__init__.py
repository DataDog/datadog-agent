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
    short_help="Create a Kind cluster on AWS",
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
    "--install-agent-with-operator/--no-install-agent-with-operator",
    default=None,
    help="Install the Agent with Operator (default: False)",
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
    "--architecture",
    type=click.Choice(["x86_64", "arm64"]),
    help="CPU architecture",
)
@click.option(
    "--use-fakeintake/--no-fakeintake",
    default=False,
    help="Deploy a fake Datadog intake for testing",
)
@click.option(
    "--use-load-balancer/--no-load-balancer",
    default=False,
    help="Use a load balancer for the fakeintake",
)
@click.option(
    "--interactive/--non-interactive",
    default=True,
    help="Enable interactive mode (notifications, clipboard)",
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
@pass_app
def cmd(
    app: Application,
    *,
    config_path: str | None,
    stack_name: str | None,
    install_agent: bool,
    install_agent_with_operator: bool | None,
    install_argorollout: bool,
    agent_version: str | None,
    architecture: str | None,
    use_fakeintake: bool,
    use_load_balancer: bool,
    interactive: bool,
    full_image_path: str | None,
    cluster_agent_full_image_path: str | None,
    agent_flavor: str | None,
    helm_config: str | None,
) -> None:
    """
    Create a Kind (Kubernetes in Docker) cluster on an AWS EC2 instance.

    This command provisions an EC2 instance with Kind installed for Kubernetes testing.

    Examples:

        # Create a default Kind cluster
        dda lab aws kind create

        # Create a Kind cluster with ARM architecture
        dda lab aws kind create --architecture arm64

        # Create a Kind cluster with fakeintake
        dda lab aws kind create --use-fakeintake
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.kind import create_kind as create_kind_task

    ctx = Context()

    app.display_info("Creating Kind cluster on AWS...")

    try:
        create_kind_task(
            ctx,
            config_path=config_path,
            stack_name=stack_name,
            install_agent=install_agent,
            install_agent_with_operator=install_agent_with_operator,
            install_argorollout=install_argorollout,
            agent_version=agent_version,
            architecture=architecture,
            use_fakeintake=use_fakeintake,
            use_loadBalancer=use_load_balancer,
            interactive=interactive,
            full_image_path=full_image_path,
            cluster_agent_full_image_path=cluster_agent_full_image_path,
            agent_flavor=agent_flavor,
            helm_config=helm_config,
        )
        app.display_success("Kind cluster created successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to create Kind cluster (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to create Kind cluster: {e}")


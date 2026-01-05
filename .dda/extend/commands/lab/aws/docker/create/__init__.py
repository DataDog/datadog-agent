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
    short_help="Create a Docker environment on AWS",
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
    agent_version: str | None,
    architecture: str | None,
    use_fakeintake: bool,
    use_load_balancer: bool,
    interactive: bool,
    full_image_path: str | None,
    agent_flavor: str | None,
    agent_env: str | None,
) -> None:
    """
    Create a Docker environment on an AWS EC2 instance.

    This command provisions an EC2 instance with Docker installed for container testing.

    Examples:

        # Create a default Docker environment
        dda lab aws docker create

        # Create a Docker environment with ARM architecture
        dda lab aws docker create --architecture arm64

        # Create a Docker environment with fakeintake
        dda lab aws docker create --use-fakeintake
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.docker import create_docker as create_docker_task

    ctx = Context()

    app.display_info("Creating Docker environment on AWS...")

    try:
        create_docker_task(
            ctx,
            config_path=config_path,
            stack_name=stack_name,
            install_agent=install_agent,
            agent_version=agent_version,
            architecture=architecture,
            use_fakeintake=use_fakeintake,
            use_loadBalancer=use_load_balancer,
            interactive=interactive,
            full_image_path=full_image_path,
            agent_flavor=agent_flavor,
            agent_env=agent_env,
        )
        app.display_success("Docker environment created successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to create Docker environment (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to create Docker environment: {e}")


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
    short_help="Create an AWS virtual machine",
)
@click.option(
    "--config-path",
    type=click.Path(exists=False),
    help="Path to custom config file (default: ~/.test_infra_config.yaml)",
)
@click.option(
    "--stack-name",
    help="Custom name for the stack (useful for multiple environments)",
)
@click.option(
    "--pipeline-id",
    help="Pipeline ID of the Agent build to install (e.g., '12345678')",
)
@click.option(
    "--install-agent/--no-install-agent",
    default=True,
    help="Install the Datadog Agent (default: True)",
)
@click.option(
    "--install-installer/--no-install-installer",
    default=False,
    help="Install the Datadog Installer instead of Agent (default: False)",
)
@click.option(
    "--agent-version",
    help="Agent version to install (e.g., '7.50.0~rc.1-1')",
)
@click.option(
    "--debug/--no-debug",
    default=False,
    help="Enable Pulumi debug mode",
)
@click.option(
    "--os-family",
    type=click.Choice(["ubuntu", "debian", "centos", "amazonlinux", "redhat", "suse", "windows", "fedora", "macos"]),
    help="Operating system family",
)
@click.option(
    "--os-version",
    help="Operating system version",
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
    "--ami-id",
    help="Specific AMI ID to use",
)
@click.option(
    "--architecture",
    type=click.Choice(["x86_64", "arm64"]),
    help="CPU architecture",
)
@click.option(
    "--interactive/--non-interactive",
    default=True,
    help="Enable interactive mode (notifications, clipboard)",
)
@click.option(
    "--instance-type",
    help="EC2 instance type (e.g., 't3.medium')",
)
@click.option(
    "--no-verify",
    is_flag=True,
    default=False,
    help="Skip verification of deploy jobs",
)
@click.option(
    "--ssh-user",
    help="SSH user for connection (auto-detected by default)",
)
@click.option(
    "--add-known-host/--no-add-known-host",
    default=True,
    help="Add host to SSH known_hosts file",
)
@click.option(
    "--agent-flavor",
    help="Agent flavor (e.g., 'datadog-fips-agent')",
)
@click.option(
    "--agent-config-path",
    type=click.Path(exists=True),
    help="Path to agent config file to merge with defaults",
)
@click.option(
    "--local-package",
    type=click.Path(exists=True),
    help="Path to local package to install",
)
@click.option(
    "--latest-ami/--no-latest-ami",
    default=False,
    help="Use the latest AMI for the OS and architecture",
)
@pass_app
def cmd(
    app: Application,
    *,
    config_path: str | None,
    stack_name: str | None,
    pipeline_id: str | None,
    install_agent: bool,
    install_installer: bool,
    agent_version: str | None,
    debug: bool,
    os_family: str | None,
    os_version: str | None,
    use_fakeintake: bool,
    use_load_balancer: bool,
    ami_id: str | None,
    architecture: str | None,
    interactive: bool,
    instance_type: str | None,
    no_verify: bool,
    ssh_user: str | None,
    add_known_host: bool,
    agent_flavor: str | None,
    agent_config_path: str | None,
    local_package: str | None,
    latest_ami: bool,
) -> None:
    """
    Create a new AWS EC2 virtual machine for testing.

    This command provisions an EC2 instance using Pulumi and optionally installs
    the Datadog Agent. The VM details will be displayed upon completion.

    Examples:

        # Create a default Ubuntu VM with Agent
        dda lab aws vm create

        # Create a Windows VM
        dda lab aws vm create --os-family windows

        # Create a VM with specific Agent version from a pipeline
        dda lab aws vm create --pipeline-id 12345678 --agent-version 7.50.0

        # Create a VM without Agent installation
        dda lab aws vm create --no-install-agent

        # Create an ARM64 VM
        dda lab aws vm create --architecture arm64
    """
    import sys

    # Add test/e2e-framework to path to import existing task modules
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.vm import create_vm as create_vm_task

    # Create an invoke context for the task
    ctx = Context()

    app.display_info("Creating AWS VM...")

    try:
        create_vm_task(
            ctx,
            config_path=config_path,
            stack_name=stack_name,
            pipeline_id=pipeline_id,
            install_agent=install_agent,
            install_installer=install_installer,
            agent_version=agent_version,
            debug=debug,
            os_family=os_family,
            os_version=os_version,
            use_fakeintake=use_fakeintake,
            use_loadBalancer=use_load_balancer,
            ami_id=ami_id,
            architecture=architecture,
            interactive=interactive,
            instance_type=instance_type,
            no_verify=no_verify,
            ssh_user=ssh_user,
            add_known_host=add_known_host,
            agent_flavor=agent_flavor,
            agent_config_path=agent_config_path,
            local_package=local_package,
            latest_ami=latest_ami,
        )
        app.display_success("AWS VM created successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to create VM (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to create VM: {e}")


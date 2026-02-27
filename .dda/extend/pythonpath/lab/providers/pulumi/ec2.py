# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""AWS EC2 VM provider for lab environments."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

import click

from lab.capabilities import ExecCapability, FakeintakeCapability
from lab.providers import register_provider
from lab.providers.pulumi.base import PulumiProvider

if TYPE_CHECKING:
    from dda.cli.application import Application


@register_provider
class EC2Provider(PulumiProvider, ExecCapability, FakeintakeCapability):
    """AWS EC2 virtual machine provider."""

    name = "vm"
    category = "aws"
    description = "AWS EC2 virtual machine"

    # SSH connection settings
    SSH_TIMEOUT_SEC = 10
    REMOTE_HOSTNAME = "aws-vm"

    # Fakeintake settings (when deployed with VM)
    FAKEINTAKE_DEFAULT_PORT = 80

    create_options = [
        click.option(
            "--agent-version",
            help="Datadog agent version to deploy",
        ),
        click.option(
            "--fakeintake",
            is_flag=True,
            help="Deploy fakeintake alongside the VM",
        ),
        click.option(
            "--os-family",
            type=click.Choice(["ubuntu", "debian", "amazonlinux", "centos", "redhat", "suse", "windows"]),
            default="ubuntu",
            help="Operating system family",
        ),
        click.option(
            "--os-version",
            help="Operating system version (e.g., 22.04 for Ubuntu)",
        ),
        click.option(
            "--architecture",
            type=click.Choice(["x86_64", "arm64"]),
            default="x86_64",
            help="CPU architecture",
        ),
        click.option(
            "--instance-type",
            help="EC2 instance type (e.g., t3.medium, t4g.medium for ARM)",
        ),
        click.option(
            "--region",
            default="us-east-1",
            help="AWS region",
        ),
        click.option(
            "--ami-id",
            help="Custom AMI ID (overrides os-family/version)",
        ),
    ]

    def __init__(self):
        """Initialize EC2Provider with aws/vm scenario."""
        super().__init__(scenario="aws/vm")

    def build_deploy_args(self, app: Application, opts: dict[str, Any]) -> dict[str, Any]:
        """Build deployment arguments for EC2 VM."""
        # Start with base args
        deploy_args = super().build_deploy_args(app, opts)

        # Add EC2-specific flags
        extra_flags = deploy_args.get("extra_flags", {})

        if opts.get("os_family"):
            os_family = opts["os_family"]
            os_version = opts.get("os_version", "")
            architecture = opts.get("architecture", "x86_64")
            extra_flags["ddinfra:osDescriptor"] = f"{os_family}:{os_version}:{architecture}"

        if opts.get("ami_id"):
            extra_flags["ddinfra:osImageID"] = opts["ami_id"]

        if opts.get("instance_type"):
            architecture = opts.get("architecture", "x86_64")
            if architecture == "x86_64":
                extra_flags["ddinfra:aws/defaultInstanceType"] = opts["instance_type"]
            else:
                extra_flags["ddinfra:aws/defaultARMInstanceType"] = opts["instance_type"]

        if opts.get("region"):
            extra_flags["ddinfra:aws/region"] = opts["region"]

        deploy_args["extra_flags"] = extra_flags
        deploy_args["key_pair_required"] = True

        return deploy_args

    def _get_ssh_connection_info(self, app: Application, stack_name: str, outputs: dict) -> dict[str, str]:
        """Extract SSH connection information from stack outputs."""
        host_key = f"dd-Host-{self.REMOTE_HOSTNAME}"
        if host_key not in outputs:
            app.abort(f"No host information found in stack outputs (key: {host_key})")

        host_info = outputs[host_key]
        return {
            "address": host_info.get("address", ""),
            "username": host_info.get("username", "ubuntu"),
            "port": str(host_info.get("port", 22)),
        }

    def exec_command(self, app: Application, name: str, command: list[str], **kwargs: Any) -> str:
        """Execute a command on the VM via SSH."""
        # Load environment to get stack info
        from lab import LabEnvironment

        env = LabEnvironment.load(app, name)
        if env is None:
            app.abort(f"Environment '{name}' not found")

        metadata_output = env.metadata.get("output", {})
        stack_name = metadata_output.get("stack_name")
        outputs = metadata_output.get("outputs", {})

        if not stack_name or not outputs:
            app.abort(f"Environment '{name}' is not a valid Pulumi environment")

        # Get SSH connection info
        ssh_info = self._get_ssh_connection_info(app, stack_name, outputs)
        address = ssh_info["address"]
        username = ssh_info["username"]
        port = ssh_info["port"]

        # Build SSH command
        ssh_cmd = [
            "ssh",
            "-o",
            "StrictHostKeyChecking=no",
            "-o",
            "UserKnownHostsFile=/dev/null",
            "-o",
            f"ConnectTimeout={self.SSH_TIMEOUT_SEC}",
            "-p",
            port,
            f"{username}@{address}",
        ] + command

        app.display(f"Executing on {username}@{address}: {' '.join(command)}")

        # Execute command
        result = app.subprocess.run(
            ssh_cmd,
            stdout=True,
            stderr=True,
            check=False,
        )

        if result.returncode != 0:
            app.abort(f"SSH command failed with exit code {result.returncode}")

        return result.stdout

    def get_fakeintake_url(self, app: Application, name: str) -> str:
        """Get the URL to access fakeintake on the VM."""
        # Load environment to get stack info
        from lab import LabEnvironment

        env = LabEnvironment.load(app, name)
        if env is None:
            app.abort(f"Environment '{name}' not found")

        metadata_output = env.metadata.get("output", {})
        outputs = metadata_output.get("outputs", {})
        fakeintake_enabled = metadata_output.get("fakeintake_enabled", False)

        if not fakeintake_enabled:
            app.abort(f"Fakeintake is not enabled for environment '{name}'")

        # Get fakeintake URL from outputs
        # The fakeintake is typically deployed with a load balancer
        fakeintake_url = None
        for key, value in outputs.items():
            if "fakeintake" in key.lower() and "url" in key.lower():
                fakeintake_url = value
                break

        if not fakeintake_url:
            # Try to construct from VM address
            ssh_info = self._get_ssh_connection_info(app, metadata_output.get("stack_name", ""), outputs)
            fakeintake_url = f"http://{ssh_info['address']}:{self.FAKEINTAKE_DEFAULT_PORT}"

        return fakeintake_url

    def query_fakeintake(
        self, app: Application, name: str, metric: str | None = None, list_metrics: bool = False
    ) -> str:
        """Query fakeintake metrics.

        Note: This assumes fakeintake is accessible from the VM's public IP.
        """
        # Get fakeintake URL
        fakeintake_url = self.get_fakeintake_url(app, name)
        app.display(f"Querying fakeintake at {fakeintake_url}")

        # Use fakeintake client
        from lab.fakeintake_client import FakeintakeClient

        client = FakeintakeClient(app, fakeintake_url=fakeintake_url)

        if list_metrics:
            return client.list_metrics()
        elif metric:
            return client.query_metrics(metric_name=metric)
        else:
            app.abort("Either --metric or --list must be specified")

# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""AWS EKS provider for lab environments."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

import click

from lab.capabilities import ExecCapability, FakeintakeCapability, LogCapability
from lab.providers import register_provider
from lab.providers.pulumi.base import PulumiOptions, PulumiProvider

if TYPE_CHECKING:
    from dda.cli.application import Application


@register_provider
class EKSProvider(PulumiProvider, FakeintakeCapability, LogCapability, ExecCapability):
    """AWS EKS (Elastic Kubernetes Service) cluster provider."""

    name = "eks"
    category = "aws"
    description = "AWS EKS cluster"
    options_class = PulumiOptions

    # EKS fakeintake configuration
    FAKEINTAKE_NAMESPACE = "fakeintake"
    FAKEINTAKE_SERVICE = "fakeintake"
    FAKEINTAKE_LOCAL_PORT = 8080
    FAKEINTAKE_REMOTE_PORT = 80

    create_options = [
        click.option("--agent-version", default=None, help="Datadog agent version to deploy"),
        click.option("--fakeintake", is_flag=True, default=False, help="Deploy fakeintake for local testing"),
        click.option("--region", default="us-east-1", help="AWS region"),
        click.option("--instance-type", default="t3.medium", help="EC2 instance type for nodes"),
        click.option("--debug", is_flag=True, default=False, help="Enable debug logging"),
    ]

    def __init__(self):
        """Initialize EKS provider."""
        super().__init__(scenario="aws/eks")

    # Fakeintake Capability Implementation

    def _get_fakeintake_port_forward(self, app: Application, name: str) -> int:
        """Get or create port-forward for fakeintake service.

        Args:
            app: Application instance
            name: Environment name (used as stack name)

        Returns:
            Local port number
        """
        from lab.port_forward import get_port_forward_manager

        # Get kubeconfig context from stack outputs
        outputs = self._get_stack_outputs(app, name)
        context = self._get_kube_context(app, name, outputs)

        pf_manager = get_port_forward_manager()
        return pf_manager.get_port_forward(
            app=app,
            env_name=name,
            service=self.FAKEINTAKE_SERVICE,
            namespace=self.FAKEINTAKE_NAMESPACE,
            local_port=self.FAKEINTAKE_LOCAL_PORT,
            remote_port=self.FAKEINTAKE_REMOTE_PORT,
            context=context,
        )

    def query_fakeintake(
        self,
        app: Application,
        name: str,
        *,
        metric_name: str | None = None,
        list_metrics: bool = False,
        output_format: str = "pretty",
    ) -> str:
        """Query fakeintake for metrics."""
        from lab.fakeintake_client import ensure_client_built, query_metrics

        # Ensure client is built
        ensure_client_built(app)

        # Get or create port-forward
        local_port = self._get_fakeintake_port_forward(app, name)

        # Query fakeintake
        fakeintake_url = f"http://localhost:{local_port}"
        return query_metrics(
            fakeintake_url=fakeintake_url,
            metric_name=metric_name,
            list_metrics=list_metrics,
            output_format=output_format,
        )

    def get_fakeintake_url(self, app: Application, name: str) -> str:
        """Get the URL to access fakeintake (with automatic port-forwarding)."""
        local_port = self._get_fakeintake_port_forward(app, name)
        return f"http://localhost:{local_port}"

    # Log Capability Implementation

    def get_logs(
        self,
        app: Application,
        name: str,
        *,
        follow: bool = False,
        tail: int = 50,
        component: str | None = None,
    ) -> None:
        """Get agent logs from EKS pods."""
        outputs = self._get_stack_outputs(app, name)
        context = self._get_kube_context(app, name, outputs)

        # Build kubectl logs command
        cmd = [
            "kubectl",
            "logs",
            "-n",
            "datadog",
            "-l",
            "app=datadog",
            "--context",
            context,
        ]

        if follow:
            cmd.append("-f")

        if tail:
            cmd.extend(["--tail", str(tail)])

        if component:
            # Filter by component label if available
            cmd.extend(["-l", f"component={component}"])

        app.subprocess.run(cmd, check=True)

    # Exec Capability Implementation

    def exec_command(
        self,
        app: Application,
        name: str,
        command: list[str],
        *,
        interactive: bool = False,
    ) -> str:
        """Execute command in agent pod."""
        outputs = self._get_stack_outputs(app, name)
        context = self._get_kube_context(app, name, outputs)

        # Get first agent pod
        result = app.subprocess.capture(
            [
                "kubectl",
                "get",
                "pods",
                "-n",
                "datadog",
                "-l",
                "app=datadog",
                "-o",
                "jsonpath={.items[0].metadata.name}",
                "--context",
                context,
            ],
            check=True,
        )

        pod_name = result.strip()
        if not pod_name:
            raise RuntimeError("No agent pods found in datadog namespace")

        # Build kubectl exec command
        cmd = ["kubectl", "exec"]

        if interactive:
            cmd.append("-it")

        cmd.extend(["-n", "datadog", pod_name, "--context", context, "--", *command])

        if interactive:
            app.subprocess.run(cmd, check=True)
            return ""
        else:
            return app.subprocess.capture(cmd, check=True)

    # Helper methods

    def _get_kube_context(self, app: Application, stack_name: str, outputs: dict[str, Any]) -> str:
        """Get Kubernetes context name from stack outputs.

        For EKS, we need to update kubeconfig first.
        """
        # Check if we need to update kubeconfig
        cluster_name = outputs.get("cluster-name", stack_name)
        region = outputs.get("region", "us-east-1")

        # Update kubeconfig to add EKS cluster
        app.display_info(f"Updating kubeconfig for EKS cluster: {cluster_name}")
        app.subprocess.run(
            ["aws", "eks", "update-kubeconfig", "--name", cluster_name, "--region", region],
            check=True,
        )

        # Context name follows AWS convention
        return f"arn:aws:eks:{region}:{outputs.get('account-id', '')}:cluster/{cluster_name}"

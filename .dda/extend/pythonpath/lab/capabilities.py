# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Lab capabilities - optional features that providers can implement.

This module defines capability interfaces that providers can implement
to expose additional functionality like metric querying, log viewing, etc.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from dda.cli.application import Application


class FakeintakeCapability(ABC):
    """
    Capability for providers that support fakeintake integration.

    Providers implementing this capability can:
    - Query metrics from fakeintake
    - List available metrics
    - Access fakeintake programmatically

    The implementation handles provider-specific details like:
    - Port forwarding (for Kubernetes)
    - Direct access (for Docker)
    - Client binary management
    """

    @abstractmethod
    def query_fakeintake(
        self,
        app: Application,
        name: str,
        *,
        metric_name: str | None = None,
        list_metrics: bool = False,
        output_format: str = "pretty",
    ) -> str:
        """
        Query fakeintake for metrics.

        Args:
            app: Application instance
            name: Environment name
            metric_name: Specific metric to query (e.g., "system.cpu.idle")
            list_metrics: If True, list all available metrics
            output_format: Output format ("pretty", "json", "table")

        Returns:
            Formatted metric data

        Raises:
            RuntimeError: If fakeintake is not enabled for this environment
        """
        pass

    @abstractmethod
    def get_fakeintake_url(self, app: Application, name: str) -> str:
        """
        Get the URL to access fakeintake.

        For Kubernetes: starts port-forward and returns localhost URL
        For Docker: returns direct container URL

        Args:
            app: Application instance
            name: Environment name

        Returns:
            Fakeintake URL (e.g., "http://localhost:8080")
        """
        pass


class LogCapability(ABC):
    """
    Capability for providers that support log viewing.

    Providers can stream agent logs, filter them, etc.
    """

    @abstractmethod
    def get_logs(
        self,
        app: Application,
        name: str,
        *,
        follow: bool = False,
        tail: int = 50,
        component: str | None = None,
    ) -> None:
        """
        View agent logs.

        Args:
            app: Application instance
            name: Environment name
            follow: If True, stream logs continuously
            tail: Number of lines to show (if not following)
            component: Specific component to show logs for
        """
        pass


class ExecCapability(ABC):
    """
    Capability for providers that support command execution.

    Allows running commands inside the agent container/pod.
    """

    @abstractmethod
    def exec_command(
        self,
        app: Application,
        name: str,
        command: list[str],
        *,
        interactive: bool = False,
    ) -> str:
        """
        Execute a command in the agent environment.

        Args:
            app: Application instance
            name: Environment name
            command: Command to execute
            interactive: If True, attach stdin/stdout

        Returns:
            Command output (if not interactive)
        """
        pass


class StatusCapability(ABC):
    """
    Capability for providers that support status checking.

    Provides health information about the lab environment.
    """

    @abstractmethod
    def get_status(self, app: Application, name: str) -> dict[str, Any]:
        """
        Get environment status.

        Args:
            app: Application instance
            name: Environment name

        Returns:
            Status information dict with keys like:
            - "healthy": bool
            - "agent_running": bool
            - "fakeintake_running": bool (if enabled)
            - "components": list of component statuses
            - "metrics_flowing": bool (if fakeintake enabled)
        """
        pass

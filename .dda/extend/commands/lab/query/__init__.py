# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Query metrics from fakeintake in a lab environment."""

from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Query fakeintake metrics")
@click.option("--id", "-i", "env_id", required=False, help="Environment ID")
@click.option("--metric", "-m", "metric_name", help="Metric name to query (e.g., system.cpu.idle)")
@click.option("--list", "-l", "list_metrics", is_flag=True, help="List all available metrics")
@click.option(
    "--format", "-f", "output_format", type=click.Choice(["pretty", "json"]), default="pretty", help="Output format"
)
@pass_app
def cmd(
    app: Application, *, env_id: str | None, metric_name: str | None, list_metrics: bool, output_format: str
) -> None:
    """
    Query metrics from fakeintake.

    Examples:
        # List all metrics
        dda lab query --list --id dev

        # Query specific metric
        dda lab query --metric system.cpu.idle --id dev

        # JSON output
        dda lab query --metric system.cpu.idle --format json --id dev
    """
    from lab import LabEnvironment
    from lab.capabilities import FakeintakeCapability
    from lab.providers import get_provider

    # Get environment
    if not env_id:
        app.abort("Must specify --id. Example: dda lab query --id dev --list")

    try:
        env = LabEnvironment.load(app, env_id)
        if env is None:
            raise ValueError(f"Environment '{env_id}' not found")
    except Exception as e:
        app.abort(f"Error loading environment '{env_id}': {e}")

    # Get provider
    provider = get_provider(env.env_type)
    if provider is None:
        app.abort(f"Unknown provider type: {env.env_type}")

    # Check if provider supports fakeintake
    if not isinstance(provider, FakeintakeCapability):
        app.abort(f"Provider '{env.env_type}' does not support fakeintake querying.")

    # Check if fakeintake is enabled for this environment
    # The provider's output is stored under metadata["output"]
    metadata_output = env.metadata.get("output", {})
    if not metadata_output.get("fakeintake_enabled"):
        app.abort(f"Fakeintake is not enabled for environment '{env.name}'. Recreate with --fakeintake flag.")

    # Validate arguments
    if not list_metrics and not metric_name:
        app.abort("Must specify either --list or --metric")

    # Query fakeintake
    try:
        result = provider.query_fakeintake(
            app,
            env.name,
            metric_name=metric_name,
            list_metrics=list_metrics,
            output_format=output_format,
        )
        app.display(result)
    except Exception as e:
        app.abort(f"Failed to query fakeintake: {e}")

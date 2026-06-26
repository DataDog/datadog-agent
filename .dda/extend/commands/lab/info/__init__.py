# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Show environment information."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Show environment information")
@click.option("--id", "-i", "env_id", required=True, help="Environment ID")
@click.option(
    "--format", "-f", "output_format", type=click.Choice(["pretty", "json"]), default="pretty", help="Output format"
)
@pass_app
def cmd(app: Application, *, env_id: str, output_format: str) -> None:
    """
    Show detailed information about a lab environment.

    For Pulumi environments: Shows stack outputs
    For local environments: Shows cluster configuration

    Examples:
        dda lab info --id my-eks
        dda lab info --id my-kind --format json
    """
    from lab import LabEnvironment
    from lab.providers.pulumi.base import PulumiProvider

    # Load environment
    try:
        env = LabEnvironment.load(app, env_id)
        if env is None:
            raise ValueError(f"Environment '{env_id}' not found")
    except Exception as e:
        app.abort(f"Error loading environment '{env_id}': {e}")

    # Get provider
    from lab.providers import get_provider

    provider = get_provider(env.env_type)
    if provider is None:
        app.abort(f"Unknown provider type: {env.env_type}")

    # Show environment info based on provider type
    if isinstance(provider, PulumiProvider):
        _show_pulumi_info(app, env, output_format)
    else:
        _show_local_info(app, env, output_format)


def _show_pulumi_info(app, env, output_format: str):
    """Show Pulumi environment information."""
    metadata_output = env.metadata.get("output", {})
    stack_name = metadata_output.get("stack_name", "N/A")
    scenario = metadata_output.get("scenario", "N/A")
    outputs = metadata_output.get("outputs", {})
    fakeintake_enabled = metadata_output.get("fakeintake_enabled", False)

    if output_format == "json":
        info = {
            "name": env.name,
            "type": env.env_type,
            "category": env.category,
            "stack_name": stack_name,
            "scenario": scenario,
            "fakeintake_enabled": fakeintake_enabled,
            "outputs": outputs,
        }
        app.display(json.dumps(info, indent=2))
    else:
        app.display(f"\n📊 Environment: {env.name}")
        app.display("=" * 60)
        app.display(f"Type: {env.env_type}")
        app.display(f"Category: {env.category}")
        app.display(f"Scenario: {scenario}")
        app.display(f"Stack: {stack_name}")
        app.display(f"Fakeintake: {'Yes' if fakeintake_enabled else 'No'}")

        if outputs:
            app.display("\nStack Outputs:")
            for key, value in outputs.items():
                # Truncate long values
                value_str = str(value)
                if len(value_str) > 80:
                    value_str = value_str[:77] + "..."
                app.display(f"  {key}: {value_str}")


def _show_local_info(app, env, output_format: str):
    """Show local environment information."""
    metadata_output = env.metadata.get("output", {})
    context = metadata_output.get("context", "N/A")
    k8s_version = metadata_output.get("k8s_version", "N/A")
    fakeintake_enabled = metadata_output.get("fakeintake_enabled", False)
    agent_installed = metadata_output.get("agent_installed", False)
    agent_image = metadata_output.get("agent_image")

    if output_format == "json":
        info = {
            "name": env.name,
            "type": env.env_type,
            "category": env.category,
            "context": context,
            "k8s_version": k8s_version,
            "fakeintake_enabled": fakeintake_enabled,
            "agent_installed": agent_installed,
            "agent_image": agent_image,
        }
        app.display(json.dumps(info, indent=2))
    else:
        app.display(f"\n📊 Environment: {env.name}")
        app.display("=" * 60)
        app.display(f"Type: {env.env_type}")
        app.display(f"Category: {env.category}")
        app.display(f"Context: {context}")
        app.display(f"Kubernetes: {k8s_version}")
        app.display(f"Agent: {'Installed' if agent_installed else 'Not installed'}")
        if agent_image:
            app.display(f"Agent Image: {agent_image}")
        app.display(f"Fakeintake: {'Yes' if fakeintake_enabled else 'No'}")

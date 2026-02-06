# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Dynamic command generation for lab providers.

Providers get auto-generated create commands:
    dda lab local kind create --id foo

Deletion is generic (works for any provider):
    dda lab delete --id foo
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

import click
from dda.cli.base import DynamicCommand, DynamicGroup

if TYPE_CHECKING:
    from lab.providers import BaseProvider


def create_provider_group(category: str, short_help: str) -> DynamicGroup:
    """
    Create a dynamic group that auto-discovers providers for a category.

    Each provider gets a subcommand that directly runs create (no subgroup).
    """

    class ProviderGroup(DynamicGroup):
        """Dynamic group that generates create commands from providers."""

        def __init__(self, *args: Any, **kwargs: Any) -> None:
            super().__init__(*args, **kwargs)
            self._provider_category = category

        def list_commands(self, ctx: click.Context) -> list[str]:
            from lab.providers import get_providers_by_category

            providers = get_providers_by_category().get(self._provider_category, [])
            return sorted(p.name for p in providers)

        def get_command(self, ctx: click.Context, cmd_name: str) -> click.Command | None:
            from lab.providers import get_providers_by_category

            providers = get_providers_by_category().get(self._provider_category, [])

            for provider_cls in providers:
                if provider_cls.name == cmd_name:
                    return generate_create_command(provider_cls)

            return None

    @click.group(cls=ProviderGroup, short_help=short_help)
    def group() -> None:
        pass

    group.__doc__ = f"Commands for {short_help.lower()}."
    return group


def generate_create_command(provider_cls: type[BaseProvider]) -> DynamicCommand:
    """Generate a create command for a provider."""
    from dda.cli.application import Application
    from dda.cli.base import dynamic_command, pass_app

    from lab import LabEnvironment
    from lab.providers import ProviderConfig

    # Build the command function
    @pass_app
    def create_cmd(app: Application, *, id: str | None, **kwargs: Any) -> None:
        from lab.config import load_config

        provider = provider_cls()
        if not id:
            id = f"{provider.category}-{provider.name}"

        # Load lab config once and inject into provider config
        lab_config = load_config()
        config = ProviderConfig(name=id, options=kwargs, lab_config=lab_config)

        # Create typed options for this provider
        options = provider.options_class.from_config(config)

        # Check prerequisites (action-specific filtering)
        missing = [p for p in provider.check_prerequisites(app, options) if "create" in p.actions]
        if missing:
            lines = ["Missing prerequisites:"]
            for prereq in missing:
                lines.append(f"  • {prereq.name}")
                lines.append(f"    → {prereq.remediation}")
            app.abort("\n".join(lines))

        result = provider.create(app, options)

        # Merge provider-returned metadata with options
        metadata = {"category": provider_cls.category, **kwargs}
        if result:
            metadata["output"] = result  # Store under _info to separate from options

        # Automatically save environment after successful create
        LabEnvironment(
            app,
            name=id,
            category=provider_cls.category,
            env_type=provider_cls.name,
            metadata=metadata or {},
        ).save()

    # Apply options dynamically
    create_cmd = click.option("--id", "-i", default=None, help="Environment id")(create_cmd)

    for opt in reversed(provider_cls.create_options):
        opt(create_cmd)

    # Wrap with dynamic_command
    create_cmd = dynamic_command(short_help=f"Create a {provider_cls.description}", dependencies=["pyyaml"])(create_cmd)
    create_cmd.__doc__ = f"Create a {provider_cls.description}."
    create_cmd.name = provider_cls.name

    return create_cmd

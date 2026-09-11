# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application

_MODULE_PREFIX = "github.com/DataDog/datadog-agent"


def _demo_pulumi_dir_fallback() -> str:
    """Return the standard demo Pulumi program directory for backward compatibility.

    Envs created before pulumi_dir was stored in metadata lack that key.
    We reconstruct the path by walking up to the repo root.
    """
    candidate = Path(__file__).resolve().parent
    while candidate != candidate.parent:
        go_mod = candidate / "go.mod"
        if go_mod.exists() and _MODULE_PREFIX in go_mod.read_text():
            return str(candidate / "test" / "new-e2e" / "run")
        candidate = candidate.parent
    raise RuntimeError("Could not find repo root — this extended command must run inside the agent repo")


@dynamic_command(short_help="Delete a lab environment")
@click.option(
    "--id",
    "-i",
    default=None,
    help="Environment id to delete (interactive if not provided)",
)
@click.option("--yes", "-y", is_flag=True, help="Auto-accept confirmation prompt")
@pass_app
def cmd(app: Application, *, id: str | None, yes: bool) -> None:
    """
    Delete a lab environment.

    Works with any environment type (kind, gke, eks, etc.).
    The provider-specific cleanup is handled automatically.

    If --id is not provided, shows an interactive selection menu.
    """
    from lab import LabEnvironment
    from lab.providers import get_provider

    # If no name provided, show interactive selection
    if id is None:
        environments = LabEnvironment.load_all(app)
        if not environments:
            app.display_info("No lab environments found.")
            return

        choices = [f"{e.name} ({e.env_type})" for e in environments]

        app.display_info("Select an environment to delete:\n")
        for i, choice in enumerate(choices, 1):
            app.display_info(f"  {i}. {choice}")
        app.display_info("")

        selection = app.prompt(
            "Enter number",
            type=click.IntRange(1, len(choices)),
        )
        id = environments[selection - 1].name

    try:
        env = LabEnvironment.load(app, id)
    except Exception as exception:
        app.abort(f"Error loading environment '{id}': {exception}")
        return

    if env is None:
        app.display_error(f"Environment '{id}' not found.")

        environments = LabEnvironment.load_all(app)
        if environments:
            app.display_info("\nAvailable environments:")
            for e in environments:
                app.display_info(f"  - {e.name} ({e.env_type})")
        return

    if not yes:
        if not click.confirm(f"Delete {env.env_type} environment '{id}'?"):
            app.display_info("Aborting.")
            return

    app.display_info(f"Deleting {env.env_type} environment '{id}'...")

    from lab.providers import ProviderConfig

    try:
        provider = get_provider(env.env_type)
        config = ProviderConfig(name=id)
        options = provider.options_class.from_config(config)
        missing = [p for p in provider.check_prerequisites(app, options) if "delete" in p.actions]
        if missing:
            lines = ["Missing prerequisites:"]
            for prereq in missing:
                lines.append(f"  • {prereq.name}")
                lines.append(f"    → {prereq.remediation}")
            app.abort("\n".join(lines))

        provider.destroy(app, id)
    except ValueError:
        # No registered provider for this env type.  If the environment was
        # created by a Pulumi-backed command (e.g. dda lab demo), the
        # pulumi_dir and stack name are stored in metadata — use them to
        # destroy the stack before removing the local record.
        stack = env.metadata.get("stack")
        # pulumi_dir was added to metadata in a later version; fall back to the
        # known standard path for envs created before that migration.
        pulumi_dir = env.metadata.get("pulumi_dir") or _demo_pulumi_dir_fallback()
        if stack and pulumi_dir:
            import os

            from dda.utils.process import EnvVars

            from lab.config import load_config as _load_config

            _passphrase = _load_config().pulumi.passphrase
            _pulumi_env = (
                EnvVars({"PULUMI_CONFIG_PASSPHRASE": _passphrase})
                if _passphrase and "PULUMI_CONFIG_PASSPHRASE" not in os.environ
                else None
            )
            app.display_info(f"Destroying Pulumi stack '{stack}' ...")
            exit_code = app.subprocess.run(
                ["pulumi", "destroy", "--yes", "-s", stack, "-C", pulumi_dir],
                check=False,
                env=_pulumi_env,
            )
            if exit_code != 0:
                app.abort(
                    f"pulumi destroy failed (exit {exit_code}). "
                    f"Local record kept — re-run 'dda lab delete {id}' after fixing credentials."
                )
                return
            app.subprocess.run(
                ["pulumi", "stack", "rm", "--yes", "-s", stack, "-C", pulumi_dir],
                check=False,
                env=_pulumi_env,
            )
        else:
            app.display_warning(f"Provider '{env.env_type}' not found, removing from storage only.")

    try:
        env.delete()
    except FileNotFoundError:
        app.display_error(f"Environment '{id}' not found.")
        return

    app.display_success(f"Environment '{id}' deleted.")

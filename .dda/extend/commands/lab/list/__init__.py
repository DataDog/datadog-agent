from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application

    from lab import LabEnvironment


def _format_age(created_at: str) -> str:
    """Format creation time as human-readable age."""
    from datetime import datetime

    if not created_at or created_at == "unknown":
        return "unknown"

    try:
        created = datetime.fromisoformat(created_at)
        now = datetime.now()
        delta = now - created

        if delta.days > 30:
            months = delta.days // 30
            return f"{months}mo ago"
        if delta.days > 0:
            return f"{delta.days}d ago"
        hours = delta.seconds // 3600
        if hours > 0:
            return f"{hours}h ago"
        minutes = delta.seconds // 60
        return f"{minutes}m ago"
    except (ValueError, TypeError):
        return created_at[:10] if len(created_at) >= 10 else created_at


def _get_category(env_type: str) -> str:
    """Get the category for an environment type."""
    local_types = {"kind", "minikube", "k3d", "docker"}
    if env_type in local_types:
        return "local"
    return "cloud"


def _format_env_row(env: LabEnvironment) -> dict[str, str]:
    """Extract display values for an environment."""
    return {
        "name": env.name,
        "type": env.env_type,
        "age": _format_age(env.created_at),
    }


@dynamic_command(short_help="List all lab environments")
@click.option("--type", "-t", "env_type", default=None, help="Filter by environment type (e.g., kind, gke, eks)")
@click.option("--verbose", "-v", is_flag=True, help="Show detailed information including all metadata")
@click.option("--json", "as_json", is_flag=True, help="Output as JSON")
@pass_app
def cmd(app: Application, *, env_type: str | None, verbose: bool, as_json: bool) -> None:
    """
    List all lab environments.

    Examples:

        # List all environments
        dda lab list

        # List only kind environments
        dda lab list --type kind

        # Show detailed info
        dda lab list --verbose

        # Output as JSON for scripting
        dda lab list --json
    """
    import json

    from lab import LabEnvironment

    environments = LabEnvironment.load_all(app, env_type=env_type)

    if not environments:
        if as_json:
            app.output("[]")
            return
        if env_type:
            app.display_info(f"No {env_type} environments found.")
        else:
            app.display_info("No lab environments found.")
        return

    # JSON output mode
    if as_json:
        output = [
            {
                "name": env.name,
                "type": env.env_type,
                "category": env.metadata.get("category", _get_category(env.env_type)),
                "created_at": env.created_at,
                "info": env.info,
                "metadata": env.metadata,
            }
            for env in environments
        ]
        app.j(json.dumps(output, indent=2))
        return

    rows = [_format_env_row(env) for env in sorted(environments, key=lambda e: e.name)]

    columns = ["name", "type", "age"]
    headers = {"name": "NAME", "type": "TYPE", "age": "AGE"}
    widths = {col: max(len(headers[col]), max(len(row[col]) for row in rows)) for col in columns}

    header_line = "  ".join(headers[col].ljust(widths[col]) for col in columns)
    separator = "  ".join("─" * widths[col] for col in columns)

    app.display_info("")
    app.display_info(header_line)
    app.display_info(separator)

    for row in rows:
        line = "  ".join(row[col].ljust(widths[col]) for col in columns)
        app.display_info(line)

    app.display_info("")

    if verbose:
        for env in sorted(environments, key=lambda e: e.name):
            app.display_info(f"{env.name}:")
            # Show all non-internal metadata
            for key, value in env.metadata.items():
                if not key.startswith("_") and value:
                    app.display_info(f"  {key}: {value}")
            app.display_info("")

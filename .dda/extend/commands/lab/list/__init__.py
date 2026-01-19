from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="List all lab active environments")
@click.option("--type", "-t", "env_type", default=None, help="Filter by environment type (e.g., kind, gke, eks)")
@click.option("--json", "as_json", is_flag=True, help="Output as JSON")
@pass_app
def cmd(app: Application, *, env_type: str | None, as_json: bool) -> None:
    """
    List all lab environments.

    Examples:

        # List all environments
        dda lab list

        # List only kind environments
        dda lab list --type kind

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
                "id": env.name,
                "type": env.env_type,
                "category": env.category,
                "created_at": env.created_at,
                "metadata": env.metadata,
            }
            for env in environments
        ]
        app.display(json.dumps(output, indent=2))
        return

    table = {}
    for env in environments:
        if env.category not in table:
            table[env.category] = {}
        if env.env_type not in table[env.category]:
            table[env.category][env.env_type] = {}
        table[env.category][env.env_type][env.name] = env.to_dict()
    app.display_table(table)

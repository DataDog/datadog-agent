from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, ensure_features_installed, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Run command in the legacy invoke virtual env",
    context_settings={"help_option_names": [], "ignore_unknown_options": True},
)
@click.argument("args", nargs=-1, required=True)
@pass_app
def cmd(app: Application, *, args: tuple[str, ...]) -> None:
    """
    Run command in the legacy invoke virtual env.
    """
    venv_path = app.config.storage.join("venvs", "legacy").data
    with app.tools.uv.virtual_env(venv_path) as venv:
        ensure_features_installed(["legacy-tasks"], app=app, prefix=str(venv.path))
        app.subprocess.attach(list(args))

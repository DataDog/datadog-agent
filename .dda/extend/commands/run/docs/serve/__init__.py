from __future__ import annotations

from typing import TYPE_CHECKING

import click

from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application

SITE_CONFIGS = {
    "main": "mkdocs.yml",
    "architecture": "mkdocs.architecture.yml",
}


@dynamic_command(short_help="Serve documentation")
@click.option("--port", type=int, default=8000, help="Port used to serve documentation")
@click.option("--launch", is_flag=True, help="Launch documentation in browser")
@click.option(
    "--site",
    type=click.Choice(list(SITE_CONFIGS)),
    default="main",
    help="Site to serve (the architecture docs are a separate site)",
)
@pass_app
def cmd(app: Application, *, port: int, launch: bool, site: str) -> None:
    """
    Serve documentation.

    Only one site can be served at a time; links between the main developer
    documentation and the architecture documentation point to the deployed
    sites rather than the local server.
    """
    from dda.utils.fs import Path
    from dda.utils.process import EnvVars

    from utils.docs.constants import SOURCE_DATE_EPOCH
    from utils.docs.deps import DEPENDENCIES
    from utils.docs.source_links import get_source_ref

    group_dir = Path(__file__).parent.parent
    venv_path = app.config.storage.join("venvs", group_dir.id).data
    with app.tools.uv.virtual_env(venv_path):
        with app.status("Syncing dependencies"):
            app.tools.uv.run(["pip", "install", "-q", *DEPENDENCIES])

        env_vars = EnvVars({"SOURCE_DATE_EPOCH": SOURCE_DATE_EPOCH, "DOCS_SOURCE_REF": get_source_ref()})
        serve_command = [
            "zensical",
            "serve",
            "--dev-addr",
            f"localhost:{port}",
            "--config-file",
            SITE_CONFIGS[site],
        ]
        if launch:
            serve_command.append("--open")
        app.subprocess.exit_with(serve_command, env=env_vars)

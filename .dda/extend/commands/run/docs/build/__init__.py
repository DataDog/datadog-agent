from __future__ import annotations

from typing import TYPE_CHECKING

import click

from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Build documentation")
@click.option("--check", is_flag=True, help="Ensure links are valid")
@pass_app
def cmd(app: Application, *, check: bool) -> None:
    """
    Build documentation.
    """
    from dda.utils.fs import Path
    from dda.utils.process import EnvVars

    from utils.docs.constants import SOURCE_DATE_EPOCH
    from utils.docs.deps import DEPENDENCIES

    group_dir = Path(__file__).parent.parent
    venv_path = app.config.storage.join("venvs", group_dir.id).data
    with app.tools.uv.virtual_env(venv_path):
        with app.status("Syncing dependencies"):
            app.tools.uv.run(["pip", "install", "-q", *DEPENDENCIES])

        env_vars = EnvVars({"SOURCE_DATE_EPOCH": SOURCE_DATE_EPOCH})
        build_command = ["zensical", "build", "--strict", "--clean"]
        cache_marker = Path(".cache", ".gitkeep")
        try:
            app.subprocess.run(build_command, env=env_vars)
        finally:
            cache_marker.parent.mkdir(parents=True, exist_ok=True)
            cache_marker.touch()

        if check:
            app.subprocess.exit_with(["lychee", "--config", ".lychee.toml", "site"], env=env_vars)

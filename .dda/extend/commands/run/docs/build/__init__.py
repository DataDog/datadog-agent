from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app
from utils.docs.deps import COMMAND_DEPENDENCIES

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Build documentation", dependencies=COMMAND_DEPENDENCIES)
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
    from utils.docs.links import check_links

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

    # CI runs `dda run docs check-links` as a step of its own instead, so that a rotted link on
    # somebody else's site is reported separately from documentation that fails to build.
    if check:
        check_links(app)

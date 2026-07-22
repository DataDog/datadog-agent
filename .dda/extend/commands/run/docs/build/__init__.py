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

    This builds two sites: the main developer documentation from `mkdocs.yml`
    and the architecture documentation from `mkdocs.architecture.yml`, which is
    published under the `architecture/` path of the main site.
    """
    from dda.utils.fs import Path
    from dda.utils.process import EnvVars

    from utils.docs.constants import SOURCE_DATE_EPOCH
    from utils.docs.deps import DEPENDENCIES
    from utils.docs.source_links import (
        get_source_ref,
        source_link_exclusion_pattern,
        validate_source_links,
    )

    group_dir = Path(__file__).parent.parent
    venv_path = app.config.storage.join("venvs", group_dir.id).data
    with app.tools.uv.virtual_env(venv_path):
        with app.status("Syncing dependencies"):
            app.tools.uv.run(["pip", "install", "-q", *DEPENDENCIES])

        source_ref = get_source_ref()
        env_vars = EnvVars({"SOURCE_DATE_EPOCH": SOURCE_DATE_EPOCH, "DOCS_SOURCE_REF": source_ref})
        # The architecture site is built second so that its output replaces the
        # orphan renders the main site produces for `docs/public/architecture`
        build_commands = [
            ["zensical", "build", "--strict", "--clean"],
            ["zensical", "build", "--strict", "--clean", "--config-file", "mkdocs.architecture.yml"],
        ]
        cache_marker = Path(".cache", ".gitkeep")
        try:
            for build_command in build_commands:
                app.subprocess.run(build_command, env=env_vars)
        finally:
            cache_marker.parent.mkdir(parents=True, exist_ok=True)
            cache_marker.touch()

        if check:
            repo_root = Path.cwd()
            site_dir = repo_root / "site"

            # Links to repository source code are resolved against the local
            # checkout so that pull requests adding new code may link to it
            with app.status("Validating source code links"):
                errors = validate_source_links(site_dir, repo_root, source_ref)
            if errors:
                for error in errors:
                    app.display_error(error)
                app.abort(f"Found {len(errors)} broken source code links")

            # Links to the deployed documentation are resolved against the
            # freshly built site so that new pages may be cross-referenced
            # before they are published
            lychee_command = [
                "lychee",
                "--config",
                ".lychee.toml",
                "--exclude",
                source_link_exclusion_pattern(source_ref),
                "--remap",
                f"https://datadoghq\\.dev/datadog-agent/(.*) {site_dir.as_uri()}/$1",
                "site",
            ]
            app.subprocess.exit_with(lychee_command, env=env_vars)

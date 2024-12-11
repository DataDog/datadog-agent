from __future__ import annotations

from typing import TYPE_CHECKING

from invoke import task

if TYPE_CHECKING:
    from invoke.context import Context


@task
def build(ctx: Context, validate: bool = False) -> None:
    """
    Build documentation in the `site` directory.
    """
    build_command = "mkdocs build --strict --clean"
    env_vars = {"SOURCE_DATE_EPOCH": "1580601600"}

    if validate:
        # https://github.com/linkchecker/linkchecker/issues/678
        ctx.run(f"{build_command} --no-directory-urls", env=env_vars)
        ctx.run("linkchecker --config .linkcheckerrc site")
    else:
        ctx.run(build_command, env=env_vars)


@task
def serve(ctx: Context, port: int = 8000, launch: bool = False) -> None:
    """
    Serve documentation from a temporary directory.
    """
    address = f"localhost:{port}"
    if launch:
        import webbrowser

        webbrowser.open(f"http://{address}")

    ctx.run(f"mkdocs serve --dev-addr {address}")

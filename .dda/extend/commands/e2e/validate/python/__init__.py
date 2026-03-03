from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Static analysis for Python code",
    # features=["legacy-test-infra-definitions"],
    dependencies=[
        "ruff==0.3.5",
        "mypy==1.10.0",
        "vulture==2.6",
    ],
)
@click.argument("paths", nargs=-1, type=click.Path(exists=True), default=["test/e2e-framework/tasks"])
@click.option("--fix", is_flag=True, help="Fix linting errors")
@pass_app
def cmd(app: Application, paths: list[str], fix: bool) -> None:
    """
    Lint python code.
    """
    paths_list = list[str](paths)
    app.display(f"Running mypy on {paths_list}")
    app.subprocess.run(["mypy", "--warn-unused-configs"] + paths_list)
    if fix:
        app.subprocess.run(["ruff", "check", "--fix"] + paths_list)
    app.subprocess.run(["ruff", "check"] + paths_list)
    app.subprocess.run(["vulture"] + paths_list)

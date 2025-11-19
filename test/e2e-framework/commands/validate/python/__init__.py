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
        "ruff==0.12.2",
        "vulture==2.3",
        "invoke==2.2.0",
        "termcolor==3.1.0",
        "pyyaml==6.0.2",
        "pyperclip==1.11.0",
        "pydantic==2.11.4",
        "colorama>=0.4.6",
        "lxml~=5.2.2",
        "python-gitlab==6.4.0",
        "PyGithub==1.59.1",
        "boto3>=1.28.0",
        "pyright==1.1.405",
    ],
)
@click.argument("paths", nargs=-1, type=click.Path(exists=True), default=["tasks", ".dda"])
@click.option("--fix", is_flag=True, help="Fix linting errors")
@pass_app
def cmd(app: Application, paths: list[str], fix: bool) -> None:
    """
    Lint python code.
    """
    paths_list = list[str](paths)
    app.display(f"Running pyright on {paths_list}")
    app.subprocess.run(["pyright"] + paths_list)
    if fix:
        app.subprocess.run(["ruff", "check", "--fix"] + paths_list)
    app.subprocess.run(["ruff", "check"] + paths_list)
    app.subprocess.run(
        ["vulture", "--ignore-decorators", "@task,@dynamic_command,@dynamic_group", "--ignore-names", "test_*,Test*"]
        + paths_list
    )

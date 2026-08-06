from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Show the computed local code review prompt")
@click.argument("extra_prompt", required=False)
@click.option("--base", default=None, help="Base branch or ref. Defaults to the repository default branch.")
@pass_app
def cmd(app: Application, *, extra_prompt: str | None, base: str | None) -> None:
    """
    Show the computed local code review prompt.
    """
    args = ["dda", "-q", "inv", "code-review.show-prompt"]
    _add_option(args, "base", base)
    _add_option(args, "extra-prompt", extra_prompt)
    app.subprocess.run(args)


def _add_option(args: list[str], name: str, value: str | None) -> None:
    if value is not None:
        args.append(f"--{name}={value}")

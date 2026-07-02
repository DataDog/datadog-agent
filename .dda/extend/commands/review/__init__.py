from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_group, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_group(
    short_help="Run a local AI code review",
    invoke_without_command=True,
)
@click.option("--base", default=None, help="Base branch or ref. Defaults to the repository default branch.")
@click.option("--provider", default="codex", help="Review provider: codex, claude, gemini, or all.")
@click.option("--extra-prompt", default=None, help="Additional instructions appended to generated guidelines.")
@click.option("--prompt", default=None, help="Full prompt override. Cannot be combined with --extra-prompt.")
@pass_app
def cmd(
    app: Application,
    *,
    base: str | None,
    provider: str,
    extra_prompt: str | None,
    prompt: str | None,
) -> None:
    """
    Run a local AI code review.
    """
    context = click.get_current_context()
    if context.invoked_subcommand is not None:
        return

    args = ["dda", "-q", "inv", "code-review.run", f"--provider={provider}"]
    _add_option(args, "base", base)
    _add_option(args, "extra-prompt", extra_prompt)
    _add_option(args, "prompt", prompt)
    app.subprocess.run(args)


def _add_option(args: list[str], name: str, value: str | None) -> None:
    if value is not None:
        args.append(f"--{name}={value}")

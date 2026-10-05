from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import DynamicCommand, DynamicGroup, dynamic_group, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


class ReviewGroup(DynamicGroup):
    def resolve_command(
        self, ctx: click.Context, args: list[str]
    ) -> tuple[str | None, click.Command | None, list[str]]:
        try:
            return super().resolve_command(ctx, args)
        except click.UsageError:
            # Treat unknown subcommand tokens as the optional extra prompt so
            # `dda review "focus on X"` can coexist with real subcommands.
            ctx.meta["review_extra_prompt"] = " ".join(args)
            return "extra-prompt", DynamicCommand("extra-prompt", callback=lambda: None), []


@dynamic_group(
    short_help="Run a local AI code review",
    invoke_without_command=True,
    context_settings={"ignore_unknown_options": True, "allow_extra_args": True},
    cls=ReviewGroup,
)
@click.option("--base", default=None, help="Base branch or ref. Defaults to the repository default branch.")
@click.option("--provider", default="codex", help="Review provider: codex, claude, gemini, or all.")
@click.option("--override-prompt", default=None, help="Full prompt override. Cannot be combined with extra prompt.")
@click.pass_context
@pass_app
def cmd(
    app: Application,
    context: click.Context,
    *,
    base: str | None,
    provider: str,
    override_prompt: str | None,
) -> None:
    """
    Run a local AI code review.

    Examples:
        dda review --provider claude "Please also check shutdown paths"
        dda review --override-prompt "Check the CI config for validity"
    """
    if context.invoked_subcommand is not None and "review_extra_prompt" not in context.meta:
        return

    extra_prompt = context.meta.get("review_extra_prompt") or (" ".join(context.args) if context.args else None)
    args = ["dda", "-q", "inv", "code-review.run", f"--provider={provider}"]
    _add_option(args, "base", base)
    _add_option(args, "extra-prompt", extra_prompt)
    _add_option(args, "override-prompt", override_prompt)
    app.subprocess.run(args)


def _add_option(args: list[str], name: str, value: str | None) -> None:
    if value is not None:
        args.append(f"--{name}={value}")

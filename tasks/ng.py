"""
Tools for working with the next-gen version of the CLI
"""
from __future__ import annotations

from invoke import task


@task
def ng(ctx, args: str = '') -> None:
    """
    Invoke the next-gen version of the CLI
    """
    from deva.cli import deva

    argument_list = args.split() if args else []
    deva(args=argument_list, prog_name='deva', windows_expand_args=False)

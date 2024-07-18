# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

from functools import partial

import rich_click as click

from deva import __version__
from deva.cli.application import Application
from deva.cli.base import DynamicGroup

click.group = partial(click.group, cls=DynamicGroup)
click.rich_click.USE_MARKDOWN = True
click.rich_click.SHOW_METAVARS_COLUMN = False
click.rich_click.APPEND_METAVARS_HELP = True
click.rich_click.STYLE_OPTION = 'purple'
click.rich_click.STYLE_ARGUMENT = 'purple'
click.rich_click.STYLE_COMMAND = 'purple'


@click.group(
    context_settings={'help_option_names': ['-h', '--help']},
    invoke_without_command=True,
    external_plugins=True,
    subcommands=('env',),
)
@click.version_option(version=__version__, prog_name='deva')
@click.pass_context
def deva(ctx: click.Context) -> None:
    """
    ```
         _
      __| | _____   ____ _
     / _` |/ _ \\ \\ / / _` |
    | (_| |  __/\\ V / (_| |
     \\__,_|\\___| \\_/ \\__,_|
    ```
    """
    app = Application(terminator=ctx.exit)
    if not ctx.invoked_subcommand:
        click.echo(ctx.get_help())
        app.abort()

    # Persist app data for sub-commands
    ctx.obj = app


def main():
    try:
        deva(prog_name='deva', windows_expand_args=False)
    except Exception:  # noqa: BLE001
        import os
        import sys

        import click as click_core
        from rich.console import Console

        console = Console()
        deva_debug = os.getenv('DEVA_DEBUG') in {'1', 'true'}
        console.print_exception(suppress=[click, click_core], show_locals=deva_debug)
        sys.exit(1)

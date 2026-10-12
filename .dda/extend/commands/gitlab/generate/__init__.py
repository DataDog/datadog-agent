# SPDX-FileCopyrightText: 2026-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Generate the GitLab pipeline configuration", dependencies=["pyyaml"])
@click.option(
    "--context",
    help="Path to the change context JSON sent by the DDCI task engine, `-` for stdin. Without it, the full pipeline is generated",
)
@click.option(
    "--source",
    default="gitlab-source.yml",
    show_default=True,
    help="Path to the source GitLab configuration",
)
@click.option(
    "--output",
    "-o",
    default="gitlab-config.yml",
    show_default=True,
    help="Path of the generated configuration, `-` for stdout",
)
@pass_app
def cmd(app: Application, *, context: str | None, source: str, output: str) -> None:
    """
    Generate the GitLab pipeline configuration from the source configuration and the change context.

    The datadog-agent-ci DDCI task engine runs the same generator without dda, see .gitlab/generator.yml.
    """
    from utils.gitlab.generate import main

    args = ["--source", source, "--output", output]
    if context:
        args += ["--context", context]
    if exit_code := main(args):
        app.abort(code=exit_code)

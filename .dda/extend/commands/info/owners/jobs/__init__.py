# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app
from dda.utils.fs import Path

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Find owners of GitLab CI jobs", features=["codeowners"])
@click.argument(
    "jobs",
    required=True,
    nargs=-1,
)
@click.option(
    "--owners",
    "-f",
    "owners_filepath",
    type=click.Path(exists=True, dir_okay=False, path_type=Path),
    help="Path to JOBOWNERS file",
    default=".gitlab/JOBOWNERS",
)
# TODO(@agent-devx): Make this respect any --non-interactive flag or other way to detect CI environment
@click.option(
    "--json",
    is_flag=True,
    help="Format the output as JSON",
)
@pass_app
def cmd(app: Application, jobs: tuple[str, ...], *, owners_filepath: Path, json: bool) -> None:
    """
    Gets the owners for the specified GitLab CI jobs.
    """
    import codeowners

    owners = codeowners.CodeOwners(owners_filepath.read_text(encoding="utf-8"))

    res = {job: [owner[1] for owner in owners.of(job)] for job in jobs}

    if json:
        from json import dumps

        app.output(dumps(res))
    else:
        display_res = {job: ", ".join(owners) for job, owners in res.items()}
        app.display_table(display_res, stderr=False)

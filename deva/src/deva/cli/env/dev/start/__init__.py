# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

from typing import TYPE_CHECKING

import rich_click as click

from deva.env.dev import AVAILABLE_DEV_ENVS, DEFAULT_DEV_ENV, get_dev_env

if TYPE_CHECKING:
    from deva.cli.application import Application


@click.command(short_help='Start a developer environment')
@click.option(
    '--type',
    '-t',
    'env_type',
    type=click.Choice(AVAILABLE_DEV_ENVS),
    default=DEFAULT_DEV_ENV,
    show_default=True,
    help='The type of developer environment',
)
@click.pass_obj
def start(app: Application, env_type: str) -> None:
    """
    Start a developer environment.
    """
    from deva.env.models import EnvironmentStage

    dev_env = get_dev_env(env_type)(
        name=env_type,
        platform_dirs=app.platform_dirs,
    )
    status = dev_env.status()
    expected_stage = EnvironmentStage.INACTIVE
    if status.stage != expected_stage:
        click.echo(
            f'Cannot start developer environment {env_type} in stage {status.stage.value}, '
            f'must be {expected_stage.value}'
        )
        app.abort(1)

    dev_env.start()

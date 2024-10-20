# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

from typing import TYPE_CHECKING

import rich_click as click

from deva.env.dev import AVAILABLE_DEV_ENVS, DEFAULT_DEV_ENV, get_dev_env

if TYPE_CHECKING:
    from deva.cli.application import Application

    from deva.env.dev.interface import DeveloperEnvironmentInterface


@click.command(short_help='Stop a developer environment')
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
def cmd(app: Application, env_type: str) -> None:
    """
    Stop a developer environment.
    """
    from deva.env.models import EnvironmentStage

    env_class: type[DeveloperEnvironmentInterface] = get_dev_env(env_type)
    env = env_class(
        name=env_type,
        platform_dirs=app.platform_dirs,
        config=env_class.config_class()(),
    )

    status = env.status()
    expected_stage = EnvironmentStage.ACTIVE
    if status.stage != expected_stage:
        click.echo(
            f'Cannot stop developer environment {env_type} in stage {status.stage.value}, '
            f'must be {expected_stage.value}'
        )
        app.abort(1)

    env.stop()

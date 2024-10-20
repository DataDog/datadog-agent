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


@click.command(short_help='Enter a developer environment')
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
    Enter a developer environment.
    """
    env_class: type[DeveloperEnvironmentInterface] = get_dev_env(env_type)
    env = env_class(
        name=env_type,
        platform_dirs=app.platform_dirs,
        config=env_class.config_class()(),
    )
    env.shell()

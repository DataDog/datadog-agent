# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

from typing import TYPE_CHECKING, Any

import rich_click as click

from deva.env.dev import AVAILABLE_DEV_ENVS, DEFAULT_DEV_ENV, get_dev_env

if TYPE_CHECKING:
    from deva.cli.application import Application

    from deva.env.dev.interface import DeveloperEnvironmentInterface


def resolve_environment(ctx, param, value):
    from msgspec_click import generate_options

    dev_env_class: type[DeveloperEnvironmentInterface] = get_dev_env(value)
    ctx.params['env_class'] = dev_env_class
    ctx.command.params.extend(generate_options(dev_env_class.config_class()))
    return value


@click.command(short_help='Start a developer environment', context_settings={'ignore_unknown_options': True})
@click.option(
    '--type',
    '-t',
    'env_type',
    type=click.Choice(AVAILABLE_DEV_ENVS),
    default=DEFAULT_DEV_ENV,
    show_default=True,
    is_eager=True,
    callback=resolve_environment,
    help='The type of developer environment',
)
@click.pass_context
def cmd(ctx: click.Context, env_type: str, **kwargs: Any) -> None:
    """
    Start a developer environment.
    """
    import msgspec

    from deva.env.models import EnvironmentStage

    app: Application = ctx.obj

    env_class: type[DeveloperEnvironmentInterface] = ctx.params['env_class']
    config = msgspec.convert(kwargs, env_class.config_class())
    env: DeveloperEnvironmentInterface = env_class(
        name=env_type,
        platform_dirs=app.platform_dirs,
        config=config,
    )

    status = env.status()
    expected_stage = EnvironmentStage.INACTIVE
    if status.stage != expected_stage:
        click.echo(
            f'Cannot start developer environment {env_type} in stage {status.stage.value}, '
            f'must be {expected_stage.value}'
        )
        app.abort(1)

    env.start()

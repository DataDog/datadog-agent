# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

import rich_click as click

from deva.cli.env.dev import dev


@click.group(short_help='Work with environments')
def env() -> None:
    pass


env.add_command(dev)

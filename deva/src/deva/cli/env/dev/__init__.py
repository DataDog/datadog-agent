# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

import rich_click as click


@click.group(
    short_help='Work with developer environments',
    subcommands=('run', 'shell', 'start', 'status', 'stop'),
)
def cmd() -> None:
    pass

# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

import rich_click as click

from deva.cli.env.dev.run import run
from deva.cli.env.dev.shell import shell
from deva.cli.env.dev.start import start
from deva.cli.env.dev.status import status
from deva.cli.env.dev.stop import stop


@click.group(short_help='Work with developer environments')
def dev() -> None:
    pass


dev.add_command(run)
dev.add_command(shell)
dev.add_command(start)
dev.add_command(status)
dev.add_command(stop)

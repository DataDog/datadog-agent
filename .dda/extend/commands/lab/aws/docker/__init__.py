# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Manage Docker environments on AWS",
)
def cmd() -> None:
    """
    Create and manage Docker environments on AWS EC2.

    Examples:

        # Create a Docker environment
        dda lab aws docker create

        # Destroy a Docker environment
        dda lab aws docker destroy --stack-name my-docker
    """
    pass


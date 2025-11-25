# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Manage installer lab environments on AWS",
)
def cmd() -> None:
    """
    Create and manage installer lab environments on AWS.

    Examples:

        # Create an installer lab
        dda lab aws installer create

        # Destroy an installer lab
        dda lab aws installer destroy
    """
    pass


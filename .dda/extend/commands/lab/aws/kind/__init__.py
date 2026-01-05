# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Manage Kind clusters on AWS",
)
def cmd() -> None:
    """
    Create and manage Kind (Kubernetes in Docker) clusters on AWS EC2.

    Examples:

        # Create a Kind cluster
        dda lab aws kind create

        # Destroy a Kind cluster
        dda lab aws kind destroy --stack-name my-kind
    """
    pass


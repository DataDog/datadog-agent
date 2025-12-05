# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Manage AWS ECS clusters",
)
def cmd() -> None:
    """
    Create and manage AWS ECS (Elastic Container Service) clusters for testing.

    Examples:

        # Create an ECS cluster with default settings
        dda lab aws ecs create

        # Create an ECS cluster with Fargate
        dda lab aws ecs create --use-fargate

        # Destroy an ECS cluster
        dda lab aws ecs destroy --stack-name my-ecs
    """
    pass


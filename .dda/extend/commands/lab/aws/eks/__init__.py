# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Manage AWS EKS clusters",
)
def cmd() -> None:
    """
    Create and manage AWS EKS (Elastic Kubernetes Service) clusters for testing.

    Examples:

        # Create an EKS cluster with default settings
        dda lab aws eks create

        # Create an EKS cluster with Windows nodes
        dda lab aws eks create --windows-node-group

        # Destroy an EKS cluster
        dda lab aws eks destroy --stack-name my-eks
    """
    pass


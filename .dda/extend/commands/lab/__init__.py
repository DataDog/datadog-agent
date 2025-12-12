# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Create and manage remote test infrastructure (VMs, EKS, ECS, etc.)",
)
def cmd() -> None:
    """
    Lab commands for creating and managing remote test infrastructure.

    These commands use Pulumi to provision cloud resources for testing the Datadog Agent.
    Before using these commands, run `dda lab setup` to configure your environment.

    Examples:

        # Create an AWS VM
        dda lab aws vm create --os-family ubuntu

        # Create an EKS cluster
        dda lab aws eks create

        # Destroy resources
        dda lab aws vm destroy --stack-name my-stack
    """
    pass


# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Manage AWS virtual machines",
)
def cmd() -> None:
    """
    Create and manage AWS EC2 virtual machines for testing.

    Examples:

        # Create a VM with default settings (Ubuntu)
        dda lab aws vm create

        # Create a Windows VM
        dda lab aws vm create --os-family windows

        # Create a VM with a specific agent version
        dda lab aws vm create --pipeline-id 12345678 --agent-version 7.50.0

        # Destroy a VM
        dda lab aws vm destroy --stack-name my-vm

        # Show VM connection details
        dda lab aws vm show --stack-name my-vm
    """
    pass


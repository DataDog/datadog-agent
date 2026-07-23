# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
AWS environment providers.

This group auto-discovers providers with category="aws" and generates
subcommands for each:
    dda lab aws eks --id my-cluster --fakeintake
    dda lab aws vm --id my-vm
"""

from __future__ import annotations

from lab.providers.commands import create_provider_group

# Auto-generate commands for all "aws" category providers
cmd = create_provider_group(
    category="aws",
    short_help="AWS environment providers",
)

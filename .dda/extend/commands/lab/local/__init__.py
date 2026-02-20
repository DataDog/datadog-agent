# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Local environment providers.

This group auto-discovers providers with category="local" and generates
subcommands for each:
    dda lab local kind create
    dda lab local kind destroy
    dda lab local kind status
"""

from __future__ import annotations

from lab.providers.commands import create_provider_group

# Auto-generate commands for all "local" category providers
cmd = create_provider_group(
    category="local",
    short_help="Local environment providers",
)

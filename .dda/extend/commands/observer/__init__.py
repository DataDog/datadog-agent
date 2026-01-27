# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Observer anomaly detection tools",
)
def cmd() -> None:
    """Observer anomaly detection evaluation and testing tools."""
    pass

# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group

# This command group is already defined in `dda` itself, so we do not need to redefine it here.
# Redefining it here does not do anything, the help text and commands are taken from the original definition in `dda`.
# However, if using an older version of `dda`, where the group does not exist, we need to define it, otherwise we will get an error.


@dynamic_group(
    short_help="Get information about the repo, CI and more",
)
def cmd() -> None:
    pass

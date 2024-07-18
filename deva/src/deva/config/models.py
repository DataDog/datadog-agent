# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

from typing import TYPE_CHECKING, Any

from msgspec import Struct

if TYPE_CHECKING:
    from deva.utils.fs import Path


class PlatformDirs(Struct, frozen=True, forbid_unknown_fields=True):
    data: Path
    cache: Path

    def join(self, *parts: Any) -> PlatformDirs:
        return PlatformDirs(
            data=self.data.joinpath(*parts),
            cache=self.cache.joinpath(*parts),
        )

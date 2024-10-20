# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

from functools import cached_property
from typing import TYPE_CHECKING, NoReturn

if TYPE_CHECKING:
    from collections.abc import Callable

    from deva.config.models import PlatformDirs


class Application:
    def __init__(self, *, terminator: Callable[[int], NoReturn]) -> None:
        self.__terminator = terminator

    def abort(self, code: int = 0) -> NoReturn:
        self.__terminator(code)

    @cached_property
    def platform_dirs(self) -> PlatformDirs:
        import platformdirs

        from deva.config.models import PlatformDirs
        from deva.utils.fs import Path

        return PlatformDirs(
            data=Path(platformdirs.user_data_dir('deva', appauthor=False)),
            cache=Path(platformdirs.user_cache_dir('deva', appauthor=False)),
        )

# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

from abc import ABC, abstractmethod
from typing import TYPE_CHECKING, NoReturn, TypeVar

import msgspec

if TYPE_CHECKING:
    from deva.config.models import PlatformDirs
    from deva.env.models import EnvironmentStatus


DeveloperEnvironmentConfig = TypeVar('DeveloperEnvironmentConfig', bound=msgspec.Struct)


class DeveloperEnvironmentInterface(ABC):
    """
    This interface defines the behavior of a developer environment.
    """

    def __init__(self, *, name: str, platform_dirs: PlatformDirs, config: DeveloperEnvironmentConfig) -> None:
        self.__name = name
        self.__storage_dirs = platform_dirs.join('env', 'dev', self.__name)
        self.__config = config

    @property
    def name(self) -> str:
        return self.__name

    @property
    def storage_dirs(self) -> PlatformDirs:
        return self.__storage_dirs

    @property
    def config(self) -> DeveloperEnvironmentConfig:
        return self.__config

    @classmethod
    def config_class(self) -> type[DeveloperEnvironmentConfig]:
        return msgspec.Struct

    @abstractmethod
    def start(self) -> None:
        """
        This method starts the developer environment. If this method returns early, the `status`
        method should contain information about the startup progress.

        This method will never be called if the environment is already running.
        """

    @abstractmethod
    def stop(self) -> None:
        """
        This method stops the developer environment. If this method returns early, the `status`
        method should contain information about the shutdown progress.
        """

    @abstractmethod
    def status(self) -> EnvironmentStatus:
        """
        This method returns the current status of the developer environment.
        """

    @abstractmethod
    def shell(self) -> NoReturn:
        """
        This method starts an interactive shell inside the developer environment.
        """

    @abstractmethod
    def run_command(self, command: list[str]) -> None:
        """
        This method runs a command inside the developer environment.
        """

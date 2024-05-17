"""
Standardized management of architecture values
"""

from __future__ import annotations

import platform
import sys
from typing import Literal

KMTArchName = Literal['x86_64', 'arm64']


class Arch:
    def __init__(
        self,
        name: str,
        go_arch: str,
        gcc_arch: str,
        kernel_arch: str,
        kmt_arch: KMTArchName | None,
        windows_arch: str,
        spellings: set[str],
    ):
        self.name = name
        self.go_arch = go_arch
        self.spellings = spellings
        self.gcc_arch = gcc_arch
        self._kmt_arch: KMTArchName | None = kmt_arch
        self.windows_arch = windows_arch
        self.kernel_arch = kernel_arch

    def is_cross_compiling(self) -> bool:
        return platform.machine().lower() not in self.spellings

    def gcc_compiler(self, platform: str = sys.platform) -> str:
        if platform == "darwin":
            return f"{self.gcc_arch}-apple-darwin23-gcc"
        elif platform == "linux":
            return f"{self.gcc_arch}-linux-gnu-gcc"
        elif platform == "windows":
            return f"{self.gcc_arch}-w64-mingw32-gcc"
        else:
            raise ValueError(f"Unknown platform: {platform}")

    @property
    def kmt_arch(self) -> KMTArchName:
        """
        Return the KMT arch name for this architecture. Raises ValueError if not defined.

        Useful to avoid constant None checks
        """
        if self._kmt_arch is None:
            raise ValueError(f"KMT arch not defined for {self.name}")
        return self._kmt_arch

    def __eq__(self, other: Arch) -> bool:
        if not isinstance(other, Arch):
            return False
        return self.name == other.name

    def __str__(self) -> str:
        return self.name

    def __hash__(self) -> int:
        return hash(self.name)

    def __repr__(self) -> str:
        return f"<Arch:{self.name}>"


ARCH_ARM64 = Arch(
    name="arm64",
    go_arch="arm64",
    gcc_arch="aarch64",
    kernel_arch="arm64",
    kmt_arch="arm64",
    windows_arch="arm64",
    spellings={"arm64", "aarch64"},
)
ARCH_AMD64 = Arch(
    name="amd64",
    go_arch="amd64",
    gcc_arch="x86_64",
    kernel_arch="x86",
    kmt_arch="x86_64",
    windows_arch="x64",
    spellings={"amd64", "x86_64", "x64", "x86-64"},
)
ARCH_I386 = Arch(
    name="i386",
    go_arch="386",
    gcc_arch="i386",
    kernel_arch="x86",
    kmt_arch=None,
    windows_arch="x86",
    spellings={"386", "i386", "x86"},
)

ALL_ARCHS = [ARCH_AMD64, ARCH_ARM64, ARCH_I386]


def get_arch(arch: str | Literal["local"] | Arch) -> Arch:
    if isinstance(arch, Arch):
        return arch

    if arch == "local":
        arch = platform.machine().lower()

    # Not the most efficient way to do this, but the list is small
    # enough and this way we avoid having to maintain a dictionary
    for arch_obj in ALL_ARCHS:
        if arch in arch_obj.spellings:
            return arch_obj
    raise KeyError(f"Unknown architecture: {arch}")

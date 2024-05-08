"""
Standardized management of architecture values
"""

from __future__ import annotations

import platform
import sys


class Arch:
    def __init__(self, name: str, go_arch: str, gcc_arch: str, alternative_spellings: set[str]):
        self.name = name
        self.go_arch = go_arch
        self.alternative_spellings = alternative_spellings
        self.gcc_arch = gcc_arch

    def is_cross_compiling(self) -> bool:
        return platform.machine() not in self.alternative_spellings

    def gcc_compiler(self, platform: str = sys.platform) -> str:
        if platform == "darwin":
            return f"{self.gcc_arch}-apple-darwin23-gcc"
        elif platform == "linux":
            return f"{self.gcc_arch}-linux-gnu-gcc"
        elif platform == "windows":
            return f"{self.gcc_arch}-w64-mingw32-gcc"
        else:
            raise ValueError(f"Unknown platform: {platform}")

    def __eq__(self, other: Arch) -> bool:
        return self.name == other.name


ARCH_ARM64 = Arch(name="arm64", go_arch="arm64", gcc_arch="aarch64", alternative_spellings={"arm64", "aarch64"})
ARCH_AMD64 = Arch(name="amd64", go_arch="amd64", gcc_arch="x86_64", alternative_spellings={"amd64", "x86_64", "x64"})
ARCH_I386 = Arch(name="i386", go_arch="386", gcc_arch="i386", alternative_spellings={"386", "i386", "x86"})

ALL_ARCHES = [ARCH_AMD64, ARCH_ARM64, ARCH_I386]


def get_arch(name: str) -> Arch:
    # Not the most efficient way to do this, but the list is small
    # enough and this way we avoid having to maintain a dictionary
    for arch in ALL_ARCHES:
        if name in arch.alternative_spellings:
            return arch
    raise KeyError(f"Unknown architecture: {name}")

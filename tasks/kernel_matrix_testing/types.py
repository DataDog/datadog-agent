"""
File with type definitions that should be imported *only* when type checking, as they require
extra packages that might not be available in runtime.
"""

from __future__ import annotations

import os
from typing import Literal, Protocol, TypedDict, TypeVar

Arch = Literal['x86_64', 'arm64']
ArchOrLocal = Arch | Literal['local']
PathOrStr = os.PathLike | str
Component = Literal['system-probe', 'security-agent']


class DependenciesLayout(TypedDict):  # noqa: F841
    layout: list[str]  # noqa: F841
    copy: dict[str, str]
    run: list[str]


class PlatformInfo(TypedDict, total=False):
    os_name: str  # Official OS name  # noqa: F841
    os_version: str  # Version  # noqa: F841
    image_version: str  # Image version  # noqa: F841
    kernel: str  # Kernel version
    os_id: str  # Short ID for the OS (e.g., "centos" for CentOS)  # noqa: F841
    image: str  # Name of the image file
    alt_version_names: list[str]  # Alternative version names (e.g., "jammy" for Ubuntu 22)  # noqa: F841


class Platforms(TypedDict):  # noqa: F841
    url_base: str
    x86_64: dict[str, PlatformInfo]  # noqa: F841
    arm64: dict[str, PlatformInfo]  # noqa: F841


class Disk(TypedDict):
    mount_point: str  # noqa: F841
    source: str
    target: str
    type: str


class DistroKernel(TypedDict):
    tag: str
    image_source: str  # noqa: F841
    dir: str


class CustomKernel(TypedDict):
    tag: str
    extra_params: dict[str, str]
    dir: str


Kernel = DistroKernel | CustomKernel


class VMSetDict(TypedDict, total=False):
    tags: list[str]
    recipe: str
    arch: ArchOrLocal
    console_type: str  # noqa: F841
    kernels: list[Kernel]
    disks: list[Disk]  # noqa: F841
    image: dict[str, str]
    vcpu: list[int]
    memory: list[int]
    machine: str


class VMConfig(TypedDict):  # noqa: F841
    vmsets: list[VMSetDict]


Recipe = Literal["distro", "custom"]
VMDef = tuple[Recipe, str, ArchOrLocal]


class HasName(Protocol):
    def name(self) -> str:  # noqa: U100
        ...


TNamed = TypeVar('TNamed', bound=HasName)


class SSHKey(TypedDict):
    path: (
        str | None
    )  # Path to the key in the local filesystem. Note that some keys (like 1Password ones) might not be found locally
    aws_key_name: str  # Name of the key in AWS
    name: str  # Name of the public key (identification for the agent, based on the public key comment)


class KMTConfig(TypedDict, total=False):
    ssh: SSHKey  # noqa: F841


StackOutputMicroVM = TypedDict(
    'StackOutputMicroVM', {'id': str, 'ip': 'str', 'ssh-key-path': str, 'tag': str, 'vmset-tags': list[str]}
)


class StackOutputArchData(TypedDict):
    ip: str
    microvms: list[StackOutputMicroVM]


StackOutput = dict[Arch, StackOutputArchData]

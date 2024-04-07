"""
File with type definitions that should be imported *only* when type checking, as they require
extra packages that might not be available in runtime.
"""

import os
from typing import Dict, List, Tuple, TypeVar, Union

from typing_extensions import Literal, Protocol, TypedDict

Arch = Literal['x86_64', 'arm64']
ArchOrLocal = Union[Arch, Literal['local']]
PathOrStr = Union[os.PathLike, str]
Component = Literal['system-probe', 'security-agent']


class DependencyBuild(TypedDict):  # We cannot do 'TypedDict' as a string import as it's a base class here
    directory: str
    command: str
    artifact: str


class DependenciesLayout(TypedDict):  # noqa: F841
    layout: List[str]  # noqa: F841
    copy: Dict[str, str]
    build: Dict[str, DependencyBuild]


class Platforms(TypedDict):  # noqa: F841
    url_base: str
    x86_64: Dict[str, str]  # noqa: F841
    arm64: Dict[str, str]  # noqa: F841


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
    extra_params: Dict[str, str]
    dir: str


Kernel = Union[DistroKernel, CustomKernel]


class VMSetDict(TypedDict, total=False):
    tags: List[str]
    recipe: str
    arch: ArchOrLocal
    console_type: str  # noqa: F841
    kernels: List[Kernel]
    disks: List[Disk]  # noqa: F841
    image: Dict[str, str]
    vcpu: List[int]
    memory: List[int]
    machine: str


class VMConfig(TypedDict):  # noqa: F841
    vmsets: List[VMSetDict]


Recipe = Literal["distro", "custom"]
VMDef = Tuple[Recipe, str, ArchOrLocal]


class HasName(Protocol):
    def name(self) -> str:
        ...


TNamed = TypeVar('TNamed', bound=HasName)


StackOutputMicroVM = TypedDict(
    'StackOutputMicroVM', {'id': str, 'ip': 'str', 'ssh-key-path': str, 'tag': str, 'vmset-tags': List[str]}
)


class StackOutputArchData(TypedDict):
    ip: str
    microvms: List[StackOutputMicroVM]


StackOutput = Dict[Arch, StackOutputArchData]

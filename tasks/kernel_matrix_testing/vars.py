import os
from typing import Dict, Literal, Union

Arch = Literal['x86_64', 'arm64']
ArchOrLocal = Union[Arch, Literal['local']]
PathOrStr = Union[os.PathLike, str]


arch_mapping: Dict[str, Arch] = {
    "amd64": "x86_64",
    "x86": "x86_64",
    "x86_64": "x86_64",
    "arm64": "arm64",
    "arm": "arm64",
    "aarch64": "arm64",
}


VMCONFIG = "vmconfig.json"

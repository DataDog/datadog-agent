from __future__ import annotations

from typing import TYPE_CHECKING, Dict, List

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Arch


arch_mapping: Dict[str, Arch] = {
    "amd64": "x86_64",
    "x86": "x86_64",
    "x86_64": "x86_64",
    "arm64": "arm64",
    "arm": "arm64",
    "aarch64": "arm64",
}
arch_ls: List[Arch] = ["x86_64", "arm64"]

VMCONFIG = "vmconfig.json"

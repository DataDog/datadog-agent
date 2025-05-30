from __future__ import annotations

import json
from enum import Enum
from pathlib import Path

from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import Exit


class KMTSetupType(Enum):
    remote = (1,)
    full = (2,)


class KMTSetupInfo:
    def __init__(self, setup: KMTSetupType):
        self.setup = setup

    def save(self, output: Path):
        setup_dict = {"setup": self.setup.name}
        with open(output, 'w') as f:
            json.dump(setup_dict, f)

    def __str__(self):
        return f"KMTSetupInfo<{self.setup.name}>"


def kmt_setup_info() -> KMTSetupInfo:
    kmt_os = get_kmt_os()
    if not kmt_os.kmt_setup_info.exists():
        raise Exit("Unable to find KMT setup information. Run `dda inv kmt.init` to generate it")

    with open(kmt_os.kmt_setup_info) as f:
        setup_info = json.load(f)

    return KMTSetupInfo(KMTSetupType[setup_info["setup"]])

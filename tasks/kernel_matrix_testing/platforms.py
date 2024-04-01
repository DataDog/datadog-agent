from __future__ import annotations

import json
from pathlib import Path
from typing import TYPE_CHECKING, Dict, List, cast

import yaml

from tasks.kernel_matrix_testing.tool import Exit
from tasks.pipeline import GitlabYamlLoader

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Arch, Component, Platforms


platforms_file = "test/new-e2e/system-probe/config/platforms.json"


def get_platforms():
    with open(platforms_file) as f:
        return cast('Platforms', json.load(f))


def filter_by_ci_component(platforms: Platforms, component: Component) -> Platforms:
    job_arch_mapping: Dict[Arch, str] = {
        "x86_64": "x64",
        "arm64": "arm64",
    }
    job_component_mapping: Dict[Component, str] = {
        "system-probe": "sysprobe",
        "security-agent": "secagent",
    }
    new_platforms = platforms.copy()

    target_file = (
        Path(__file__).parent.parent.parent / '.gitlab' / "kernel_matrix_testing" / f"{component.replace('-', '_')}.yml"
    )
    with open(target_file) as f:
        ci_config = yaml.load(f, Loader=GitlabYamlLoader())

    arch_ls: List[Arch] = ["x86_64", "arm64"]
    for arch in arch_ls:
        job_name = f"kmt_run_{job_component_mapping[component]}_tests_{job_arch_mapping[arch]}"
        if job_name not in ci_config:
            raise Exit(f"Job {job_name} not found in {target_file}, cannot extract used platforms")

        try:
            kernels = set(ci_config[job_name]["parallel"]["matrix"][0]["TAG"])
        except (KeyError, IndexError):
            raise Exit(f"Cannot find list of kernels (parallel.matrix[0].TAG) in {job_name} job in {target_file}")

        new_platforms[arch] = {k: v for k, v in new_platforms[arch].items() if k in kernels}

    return new_platforms

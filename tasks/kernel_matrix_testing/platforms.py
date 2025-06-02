from __future__ import annotations

import json
from pathlib import Path
from typing import TYPE_CHECKING, cast

import yaml

from tasks.kernel_matrix_testing.tool import Exit
from tasks.kernel_matrix_testing.vars import KMT_SUPPORTED_ARCHS
from tasks.libs.ciproviders.gitlab_api import ReferenceTag
from tasks.libs.types.arch import Arch

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import (
        Component,
        KMTArchName,
        Platforms,
    )


platforms_file = "test/new-e2e/system-probe/config/platforms.json"


def get_platforms():
    with open(platforms_file) as f:
        return cast("Platforms", json.load(f))


class KMTTestJob:
    def __init__(self, name: str, arch: KMTArchName, test_set: str, kernels: set[str]):
        self.name = name
        self.arch = arch
        self.test_set = test_set
        self.kernels = kernels


def filter_by_ci_component(platforms: Platforms, component: Component) -> dict[str, Platforms]:
    job_arch_mapping: dict[KMTArchName, str] = {
        "x86_64": "x64",
        "arm64": "arm64",
    }
    job_component_mapping: dict[Component, str] = {
        "system-probe": "sysprobe",
        "security-agent": "secagent",
    }

    target_file = (
        Path(__file__).parent.parent.parent / ".gitlab" / "kernel_matrix_testing" / f"{component.replace('-', '_')}.yml"
    )
    yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
    with open(target_file) as f:
        ci_config = yaml.safe_load(f)

    job_prefixes = []
    for arch in KMT_SUPPORTED_ARCHS:
        job_prefixes.append(f"kmt_run_{job_component_mapping[component]}_tests_{job_arch_mapping[arch]}")

    test_jobs = []
    for job in ci_config:
        for prefix in job_prefixes:
            if not job.startswith(prefix):
                continue

            arch = ci_config[job]["variables"]["ARCH"]
            sets = ci_config[job]["parallel"]["matrix"][0]["TEST_SET"]
            kernels = ci_config[job]["parallel"]["matrix"][0]["TAG"]

            test_jobs.append(KMTTestJob(job, Arch.from_str(arch).kmt_arch, sets, set(kernels)))

    new_platforms_by_set = {}
    for job in test_jobs:
        for s in job.test_set:
            if s not in new_platforms_by_set:
                new_platforms_by_set[s] = platforms.copy()

            # we need to index `new_platforms_by_set` by a literal to
            # avoid mypy errors, which is why assign arch to `cur_arch`
            cur_arch = None
            for arch in KMT_SUPPORTED_ARCHS:
                if job.arch == arch:
                    cur_arch = arch

            if cur_arch is None:
                raise Exit(f"Unsupported architecture {job.arch} detected for job {job.name}")

            new_platforms_by_set[s][cur_arch] = {
                k: v for k, v in new_platforms_by_set[s][cur_arch].items() if k in job.kernels
            }

            missing_kernels = job.kernels - set(new_platforms_by_set[s][cur_arch].keys())
            if missing_kernels:
                raise Exit(f"Kernels {missing_kernels} not found in {platforms_file} for {job.arch}")

    return new_platforms_by_set

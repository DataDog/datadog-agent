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


# TestSetShard identifies one (test set, shard) pair: shard is None for a job
# that does not shard, so an unsharded and a sharded job never collide on the
# same key.
TestSetShard = tuple[str, str | None]


class KMTTestJob:
    def __init__(
        self,
        name: str,
        arch: KMTArchName,
        test_set: list[TestSetShard],
        kernels: set[str],
        shards: str | None = None,
    ):
        self.name = name
        self.arch = arch
        self.test_set = test_set
        self.kernels = kernels
        # The SHARDS job variable, carried alongside for the matrix
        # consistency check in tasks/unit_tests/kmt_tests.py.
        self.shards = shards


def get_ci_test_jobs(component: Component) -> list[KMTTestJob]:
    """Return the KMT test jobs declared in the gitlab CI config for the given component."""
    job_arch_mapping: dict[KMTArchName, str] = {
        "x86_64": "x64",
        "arm64": "arm64",
    }
    job_component_mapping: dict[Component, str] = {
        "system-probe": "sysprobe",
        "security-agent": "secagent",
    }

    target_file = (
        Path(__file__).parent.parent.parent
        / ".gitlab"
        / "test"
        / "kernel_matrix_testing"
        / f"{component.replace('-', '_')}.yml"
    )
    yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
    with open(target_file) as f:
        ci_config = yaml.safe_load(f)

    job_prefixes = []
    for arch in KMT_SUPPORTED_ARCHS:
        job_prefixes.append(f"kmt_run_{job_component_mapping[component]}_tests_{job_arch_mapping[arch]}")

    test_jobs: list[KMTTestJob] = []
    for job in ci_config:
        for prefix in job_prefixes:
            if not job.startswith(prefix):
                continue

            arch = ci_config[job]["variables"]["ARCH"]
            sets = ci_config[job]["parallel"]["matrix"][0]["TEST_SET"]
            kernels = ci_config[job]["parallel"]["matrix"][0]["TAG"]
            shard_values = ci_config[job]["parallel"]["matrix"][0].get("SHARD")

            if shard_values:
                test_set_shards = [(s, shard) for s in sets for shard in shard_values]
            else:
                test_set_shards = [(s, None) for s in sets]

            shards = ci_config[job]["variables"].get("SHARDS")

            test_jobs.append(KMTTestJob(job, Arch.from_str(arch).kmt_arch, test_set_shards, set(kernels), shards))

    return test_jobs


def filter_by_ci_component(platforms: Platforms, component: Component) -> dict[TestSetShard, Platforms]:
    test_jobs = get_ci_test_jobs(component)

    new_platforms_by_set: dict[TestSetShard, Platforms] = {}
    for job in test_jobs:
        # we need to index `new_platforms_by_set` by a literal to
        # avoid mypy errors, which is why assign arch to `cur_arch`
        cur_arch = None
        for arch in KMT_SUPPORTED_ARCHS:
            if job.arch == arch:
                cur_arch = arch

        if cur_arch is None:
            raise Exit(f"Unsupported architecture {job.arch} detected for job {job.name}")

        missing_kernels = job.kernels - set(platforms[cur_arch].keys())
        if missing_kernels:
            raise Exit(f"Kernels {missing_kernels} not found in {platforms_file} for {job.arch}")

        for s in job.test_set:
            if s not in new_platforms_by_set:
                # Every architecture starts empty: a test set only gets microVMs on the
                # architectures that actually have a CI job running it. Seeding with the
                # full platform list would provision VMs for architectures with no job
                # (e.g. `cws_req`, which only runs on x86_64) that nothing ever uses.
                new_platforms_by_set[s] = cast("Platforms", {**platforms, **{arch: {} for arch in KMT_SUPPORTED_ARCHS}})

            new_platforms_by_set[s][cur_arch].update({k: v for k, v in platforms[cur_arch].items() if k in job.kernels})

    return new_platforms_by_set

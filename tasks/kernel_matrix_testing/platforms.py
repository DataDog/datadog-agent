from __future__ import annotations

import json
import re
import yaml
from pathlib import Path
from typing import TYPE_CHECKING, Dict, List, cast

from tasks.kernel_matrix_testing.tool import Exit, warn
from tasks.pipeline import GitlabYamlLoader

from invoke.context import Context


if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Arch, Component, Platforms, PlatformInfo  # noqa: F401


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


def update_image_info(ctx: Context, base_path: Path, image_info: PlatformInfo):
    if "image" not in image_info:
        raise Exit("No 'image' attribute, cannot fill info")
    image = image_info["image"]
    if image.endswith(".xz"):
        image = image[:-3]
    image = image.split('/')[-1]  # Image may have branch name in path

    image_path = base_path / image
    if not image_path.exists():
        warn(f"[!] Image not found at {image_path} skipping")
        return

    res = ctx.run(f"sudo guestfish --ro -a {image_path} -i cat /etc/os-release")
    if res is None or not res.ok:
        raise Exit(f"Failed to get /etc/os-release for {image}: {res.stderr if res is not None else ''}")

    parts = [line.split("=", 1) for line in res.stdout.splitlines() if "=" in line]
    filevars = {k.strip(): v.strip('"') for k, v in parts}

    image_info["os_name"] = filevars["NAME"]
    image_info["os_id"] = filevars["ID"]
    image_info["version"] = filevars["VERSION_ID"]

    if "VERSION_CODENAME" in filevars:
        # Update without adding duplicates
        existing_alts = set(image_info.get("alt_version_names", []))
        existing_alts.add(filevars["VERSION_CODENAME"])
        image_info["alt_version_names"] = list(existing_alts)

    if image_info["os_id"] == "centos":
        # CentOS does not provide minor version in /etc/os-release, check /etc/redhat-release
        res = ctx.run(f"sudo guestfish --ro -a {image_path} -i cat /etc/redhat-release")
        if res is not None and res.ok:
            version_match = re.search(r"release ([\d\.]+)", res.stdout)
            if version_match:
                image_info["version"] = version_match.group(1)

    # Check what kernel is installed
    res = ctx.run(f"sudo guestfish --ro -a {image_path} -i ls /boot")
    if res is None or not res.ok:
        raise Exit(f"Failed to list /boot for {image}: {res.stderr if res is not None else ''}")

    for file in res.stdout.splitlines():
        if file.startswith("vmlinuz-") or file.startswith("vmlinux-"):
            kernel_version = file.split("-", 1)[1]
            if kernel_version.endswith(".gz"):
                kernel_version = kernel_version[: -len(".gz")]
            image_info["kernel"] = kernel_version

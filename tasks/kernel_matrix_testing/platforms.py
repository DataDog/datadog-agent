from __future__ import annotations

import json
import os
import re
import yaml
from pathlib import Path
from typing import TYPE_CHECKING, Dict, List, cast, Set

from tasks.kernel_matrix_testing.tool import Exit, warn, debug, error
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
    try:
        import guestfs
    except ImportError:
        error(
            "guestfs is not installed, please install it (pip install https://download.libguestfs.org/python/guestfs-1.40.2.tar.gz). You might need to install the libguestfs-dev system package before installing it."
        )
        raise

    echo = ctx.config.run["echo"]

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

    # Prepare the GuestFS system
    g = guestfs.GuestFS(python_return_dict=True)
    g.add_drive_opts(os.fspath(image_path), readonly=1)

    try:
        g.launch()
    except Exception:
        error("[x] Failed to launch GuestFS. Are you running with sudo?")
        raise

    roots = g.inspect_os()
    if len(roots) != 1:
        raise Exit(f"Expected exactly one root, got {len(roots)}")
    root = roots[0]

    # Mount the disks automatically
    #
    # Sort keys by length, shortest first, so that we end up
    # mounting the filesystems in the correct order.
    mountpoints = g.inspect_get_mountpoints(root)
    for device, mountpoint in sorted(mountpoints.items(), key=lambda k: len(k[0])):
        g.mount_ro(mountpoint, device)

    os_release = g.cat("/etc/os-release")
    if echo:
        debug("cat /etc/os-release")
        debug(os_release)
    parts = [line.strip().split("=", 1) for line in os_release.splitlines() if "=" in line]
    filevars = {k.strip(): v.strip('"') for k, v in parts}

    image_info["os_name"] = filevars["NAME"]
    image_info["os_id"] = filevars["ID"]
    image_info["version"] = filevars["VERSION_ID"]

    if "VERSION_CODENAME" in filevars and filevars["VERSION_CODENAME"].strip() != "":
        # Update without adding duplicates
        existing_alts = set(image_info.get("alt_version_names", []))
        existing_alts.add(filevars["VERSION_CODENAME"])
        image_info["alt_version_names"] = list(existing_alts)

    if image_info["os_id"] == "centos":
        # CentOS does not provide minor version in /etc/os-release, check /etc/redhat-release
        rh_release = g.cat("/etc/redhat-release")
        if echo:
            debug("cat /etc/redhat-release")
            debug(rh_release)
        version_match = re.search(r"release ([\d\.]+)", rh_release)
        if version_match:
            image_info["version"] = version_match.group(1)

    # Check what kernel is installed
    found_kernel_versions: Set[str] = set()
    for file in g.ls("/boot"):
        if echo:
            debug(f"ls /boot/{file}")
        if (file.startswith("vmlinuz-") or file.startswith("vmlinux-")) and "rescue" not in file:
            kernel_version = file.split("-", 1)[1]
            if kernel_version.endswith(".gz"):
                kernel_version = kernel_version[: -len(".gz")]
            found_kernel_versions.add(kernel_version)

    if len(found_kernel_versions) > 1:
        debug("Found multiple kernel versions, inspecting grub.cfg")
        boot_dir_files: List[str] = g.find("/boot")
        boot_cfg_files = [f for f in boot_dir_files if f.endswith("grub.cfg")]

        found_kernel = False
        if len(boot_cfg_files) == 0:
            warn("No grub.cfg found, skipping kernel version detection")
        for boot_cfg in boot_cfg_files:
            grub_cfg = g.cat(f"/boot/{boot_cfg}")
            if echo:
                debug(f"cat /boot/{boot_cfg}")

            default_entry_match = re.search("set default=\"?([0-9]+)\"?", grub_cfg)
            default_entry = int(default_entry_match.group(1)) if default_entry_match is not None else 0
            linux_entries = re.findall("linux[^ ]* /boot/vmlinu[xz]-([^ ]+)", grub_cfg)
            if len(linux_entries) > default_entry:
                kernel = linux_entries[default_entry]
                debug(f"Found kernel version {kernel} from grub.cfg")
                found_kernel = True
                break

        if not found_kernel:
            warn(
                f"Could not find kernel version in grub.cfg, default entry is {default_entry}, we found {len(linux_entries)} entries"
            )
    elif len(found_kernel_versions) == 1:
        kernel = list(found_kernel_versions)[0]
    else:
        kernel = None
        warn("No kernel found in /boot, skipping kernel version detection")

    if kernel is not None:
        # Replace architecture suffixes, not needed
        kernel = re.sub('[-\\.](aarch64|amd64|x86_64|arm64)', '', kernel)
        image_info["kernel"] = kernel

    debug(f'Updated image info: {image_info}')

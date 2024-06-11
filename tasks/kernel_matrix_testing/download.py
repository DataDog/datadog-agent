from __future__ import annotations

import os
import platform
import tempfile
from typing import TYPE_CHECKING

from invoke.context import Context

from tasks.kernel_matrix_testing.platforms import get_platforms
from tasks.kernel_matrix_testing.tool import Exit, debug, info, warn
from tasks.kernel_matrix_testing.vars import arch_mapping
from tasks.kernel_matrix_testing.vmconfig import get_vmconfig_template

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import ArchOrLocal  # noqa: F401

try:
    import requests
except ImportError:
    requests = None

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Arch, PathOrStr


def requires_update(url_base: str, rootfs_dir: PathOrStr, image: str, branch: str):
    if requests is None:
        raise Exit("requests module is not installed, please install it to continue")

    sum_url = os.path.join(url_base, branch, image + ".sum")
    r = requests.get(sum_url)
    new_sum = r.text.rstrip().split(' ')[0]
    debug(f"[debug] {branch}/{image} new_sum: {new_sum}")

    if not os.path.exists(os.path.join(rootfs_dir, f"{image}.sum")):
        return True

    with open(os.path.join(rootfs_dir, f"{image}.sum")) as f:
        original_sum = f.read().rstrip().split(' ')[0]
        debug(f"[debug] {image} original_sum: {original_sum}")
    if new_sum != original_sum:
        return True
    return False


def download_rootfs(
    ctx: Context,
    rootfs_dir: PathOrStr,
    vmconfig_template_name: str,
    arch: Arch | None = None,
    images: str | None = None,
):
    platforms = get_platforms()
    vmconfig_template = get_vmconfig_template(vmconfig_template_name)

    url_base = platforms["url_base"]

    if arch is None:
        arch = arch_mapping[platform.machine()]
    to_download: list[str] = list()
    file_ls: list[str] = list()
    branch_mapping: dict[str, str] = dict()

    selected_image_list = images.split(",") if images is not None else []

    for tag in platforms[arch]:
        platinfo = platforms[arch][tag]
        if "image" not in platinfo:
            raise Exit("image is not defined in platform info")

        if images is not None:
            image_possible_names = [tag] + platinfo.get("alt_version_names", [])
            if "os_id" in platinfo and "os_version" in platinfo:
                image_possible_names.append(f"{platinfo['os_id']}-{platinfo['os_version']}")

            matching_names = set(selected_image_list) & set(image_possible_names)
            if len(matching_names) == 0:
                continue

            info(f"[+] Image {tag} matched filters: {', '.join(matching_names)}")
            selected_image_list = list(set(selected_image_list) - matching_names)

        path = os.path.basename(platinfo["image"])
        if path.endswith(".xz"):
            path = path[: -len(".xz")]

        branch_mapping[path] = platinfo.get('image_version', 'master')
        file_ls.append(os.path.basename(path))

    if len(selected_image_list) > 0:
        raise Exit(f"Couldn't find images for the following names: {', '.join(selected_image_list)}")

    # if file does not exist download it.
    for f in file_ls:
        path = os.path.join(rootfs_dir, f)
        if not os.path.exists(path):
            to_download.append(f)

    for vmset in vmconfig_template["vmsets"]:
        if "arch" not in vmset:
            raise Exit("arch is not defined in vmset")

        if vmset["arch"] != arch:
            continue

        for disk in vmset.get("disks", []):
            # Use the uncompressed disk name, avoid errors due to images being downloaded but not extracted
            d = os.path.basename(disk["target"])
            if not os.path.exists(os.path.join(rootfs_dir, d)):
                if d.endswith(".xz"):
                    d = d[: -len(".xz")]
                to_download.append(d)

    # download and compare hash sums
    present_files = list(set(file_ls) - set(to_download))
    for f in present_files:
        if requires_update(url_base, rootfs_dir, f, branch_mapping.get(f, "master")):
            debug(f"[debug] updating {f} from S3.")
            ctx.run(f"rm -f {f}")
            ctx.run(f"rm -f {f}.sum")
            to_download.append(f)

    if len(to_download) == 0:
        warn("[-] No update required for rootfs images")
        return

    # download files to be updates
    fd, path = tempfile.mkstemp()
    try:
        with os.fdopen(fd, 'w') as tmp:
            for f in to_download:
                branch = branch_mapping.get(f, "master")
                info(f"[+] {f} needs to be downloaded, using branch {branch}")
                filename = f"{f}.xz"
                sum_file = f"{f}.sum"
                wo_qcow2 = '.'.join(f.split('.')[:-1])
                manifest_file = f"{wo_qcow2}.manifest"
                # remove this file and sum, uncompressed file too if it exists
                ctx.run(f"rm -f {os.path.join(rootfs_dir, filename)}")
                ctx.run(f"rm -f {os.path.join(rootfs_dir, sum_file)}")
                ctx.run(f"rm -f {os.path.join(rootfs_dir, manifest_file)}")
                ctx.run(f"rm -f {os.path.join(rootfs_dir, f)} || true")  # remove old file if it exists
                # download package entry
                tmp.write(os.path.join(url_base, branch, filename) + "\n")
                tmp.write(f" dir={rootfs_dir}\n")
                tmp.write(f" out={filename}\n")
                # download sum entry
                tmp.write(os.path.join(url_base, branch, f"{sum_file}") + "\n")
                tmp.write(f" dir={rootfs_dir}\n")
                tmp.write(f" out={sum_file}\n")
                # download manifest file
                if "docker" not in f:
                    tmp.write(os.path.join(url_base, branch, f"{manifest_file}") + "\n")
                    tmp.write(f" dir={rootfs_dir}\n")
                    tmp.write(f" out={manifest_file}\n")
            tmp.write("\n")
        ctx.run(f"cat {path}")
        res = ctx.run(f"aria2c -i {path} -j {len(to_download)}")
        if res is None or not res.ok:
            raise Exit("Failed to download image files")
    finally:
        os.remove(path)

    # extract files
    res = ctx.run(f"find {rootfs_dir} -name \"*qcow*.xz\" -type f -exec xz -d {{}} \\;")
    if res is None or not res.ok:
        raise Exit("Failed to extract qcow2 files")

    # set permissions
    res = ctx.run(f"find {rootfs_dir} -name \"*qcow*\" -type f -exec chmod 0766 {{}} \\;")
    if res is None or not res.ok:
        raise Exit("Failed to set permissions 0766 to rootfs")


def full_arch(arch: str) -> Arch:
    if arch == "local":
        return arch_mapping[platform.machine()]
    else:
        try:
            return arch_mapping[arch]
        except KeyError:
            raise Exit(f"Invalid architecture {arch}, valid values are {', '.join(arch_mapping.keys())}")


def update_rootfs(
    ctx: Context, rootfs_dir: PathOrStr, vmconfig_template: str, all_archs: bool = False, images: str | None = None
):
    if all_archs:
        arch_ls: list[Arch] = ["x86_64", "arm64"]
        for arch in arch_ls:
            info(f"[+] Updating root filesystem for {arch}")
            download_rootfs(ctx, rootfs_dir, vmconfig_template, arch, images)
    else:
        download_rootfs(ctx, rootfs_dir, vmconfig_template, full_arch("local"), images)

    info("[+] Root filesystem and bootables images updated")

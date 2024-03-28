from __future__ import annotations

import os
import platform
import tempfile
from typing import TYPE_CHECKING, List

from invoke.context import Context

from tasks.kernel_matrix_testing.platforms import get_platforms
from tasks.kernel_matrix_testing.tool import Exit, debug, info, warn
from tasks.kernel_matrix_testing.vars import arch_mapping
from tasks.kernel_matrix_testing.vmconfig import get_vmconfig_template

try:
    import requests
except ImportError:
    requests = None

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import PathOrStr


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


def download_rootfs(ctx: Context, rootfs_dir: PathOrStr, vmconfig_template_name: str):
    platforms = get_platforms()
    vmconfig_template = get_vmconfig_template(vmconfig_template_name)

    url_base = platforms["url_base"]

    arch = arch_mapping[platform.machine()]
    to_download: List[str] = list()
    file_ls: List[str] = list()
    branch_mapping: dict[str, str] = dict()

    for tag in platforms[arch]:
        path = os.path.basename(platforms[arch][tag])
        if path.endswith(".xz"):
            path = path[: -len(".xz")]

        branch_mapping[path] = os.path.dirname(platforms[arch][tag]) or "master"
        file_ls.append(os.path.basename(path))

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
                # remove this file and sum, uncompressed file too if it exists
                ctx.run(f"rm -f {os.path.join(rootfs_dir, filename)}")
                ctx.run(f"rm -f {os.path.join(rootfs_dir, sum_file)}")
                ctx.run(f"rm -f {os.path.join(rootfs_dir, f)} || true")  # remove old file if it exists
                # download package entry
                tmp.write(os.path.join(url_base, branch, filename) + "\n")
                tmp.write(f" dir={rootfs_dir}\n")
                tmp.write(f" out={filename}\n")
                # download sum entry
                tmp.write(os.path.join(url_base, branch, f"{sum_file}") + "\n")
                tmp.write(f" dir={rootfs_dir}\n")
                tmp.write(f" out={sum_file}\n")
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


def update_rootfs(ctx: Context, rootfs_dir: PathOrStr, vmconfig_template: str):
    download_rootfs(ctx, rootfs_dir, vmconfig_template)

    info("[+] Root filesystem and bootables images updated")

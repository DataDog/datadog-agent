from __future__ import annotations

from invoke.context import Context

from tasks.kernel_matrix_testing.config import ConfigManager
from tasks.kernel_matrix_testing.download import download_rootfs
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.setup import check_requirements, get_requirements
from tasks.kernel_matrix_testing.tool import Exit, info, warn


def init_kernel_matrix_testing_system(
    ctx: Context,
    images: str | None = None,
    all_images: bool = False,
    remote_setup_only: bool = False,
    exclude_requirements: list[str] | None = None,
    only_requirements: list[str] | None = None,
):
    kmt_os = get_kmt_os()

    requirements = get_requirements(remote_setup_only, exclude_requirements, only_requirements)
    if check_requirements(ctx, requirements, fix=True, echo=True, verbose=ctx.config["run"]["echo"]):
        raise Exit("KMT setup failed")

    # download dependencies
    if not remote_setup_only:
        if all_images or images:
            info("[+] Downloading VM images")
            download_rootfs(ctx, kmt_os.rootfs_dir, "system-probe", arch=None, images=None if all_images else images)
    else:
        warn("[-] Skipping local VM image download since remote only setup is selected")

    cm = ConfigManager()
    cm.config["setup"] = "remote" if remote_setup_only else "full"
    cm.save()

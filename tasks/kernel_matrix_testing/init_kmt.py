from __future__ import annotations

import getpass
import os
import shutil
from typing import TYPE_CHECKING

from invoke.context import Context

from tasks.kernel_matrix_testing.compiler import all_compilers
from tasks.kernel_matrix_testing.download import download_rootfs
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import Exit, info, is_root

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import PathOrStr


def gen_ssh_key(ctx: Context, kmt_dir: PathOrStr):
    ctx.run(f"cp tasks/kernel_matrix_testing/ddvm_rsa {kmt_dir}")
    ctx.run(f"chmod 600 {kmt_dir}/ddvm_rsa")


def init_kernel_matrix_testing_system(ctx: Context, lite: bool):
    kmt_os = get_kmt_os()

    if shutil.which("pulumi") is None:
        raise Exit(
            "pulumi not found in $PATH. Follow the instructions in tasks/kernel_matrix_testing/README.md to install it"
        )

    pulumi_test_cmd = "pulumi --non-interactive plugin ls"
    res = ctx.run(pulumi_test_cmd)
    if res is None or not res.ok:
        raise Exit(
            f"Running {pulumi_test_cmd} failed, check that the installation is correct (see tasks/kernel_matrix_testing/README.md)"
        )

    if not lite:
        if shutil.which("libvirtd") is None:
            raise Exit("libvirtd not found in $PATH, did you run tasks/kernel_matrix_testing/env-setup.sh?")

        info("[+] OS-specific setup")
        kmt_os.init_local(ctx)

    info("[+] Creating KMT directories")
    sudo = "sudo" if not is_root() else ""
    user = getpass.getuser()
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.kmt_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.packages_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.stacks_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.libvirt_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.rootfs_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.shared_dir}")

    # download dependencies
    if not lite:
        info("[+] Downloading VM images")
        download_rootfs(ctx, kmt_os.rootfs_dir, "system-probe")
        gen_ssh_key(ctx, kmt_os.kmt_dir)

    # build docker compile image
    info("[+] Building compiler image")
    kmt_os.assert_user_in_docker_group(ctx)
    info(f"[+] User '{os.getlogin()}' in group 'docker'")

    if kmt_os.name == "macos"
        for cc in all_compilers(ctx):
            cc.build()
    else:
        get_compiler(ctx, "local").build()

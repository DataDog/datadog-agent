from __future__ import annotations

import getpass
import os
from typing import TYPE_CHECKING

from invoke.context import Context

from tasks.kernel_matrix_testing.compiler import build_compiler
from tasks.kernel_matrix_testing.download import download_rootfs
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import info

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import PathOrStr


def is_root() -> bool:
    return os.getuid() == 0


def gen_ssh_key(ctx: Context, kmt_dir: PathOrStr):
    ctx.run(f"cp tasks/kernel_matrix_testing/ddvm_rsa {kmt_dir}")
    ctx.run(f"chmod 400 {kmt_dir}/ddvm_rsa")


def init_kernel_matrix_testing_system(ctx: Context, lite: bool):
    kmt_os = get_kmt_os()

    sudo = "sudo" if not is_root() else ""
    user = getpass.getuser()
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.kmt_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.packages_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.stacks_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.libvirt_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.rootfs_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.shared_dir}")

    if not lite:
        ## fix libvirt conf
        ctx.run(
            f"{sudo} sed --in-place 's/#security_driver = \"selinux\"/security_driver = \"none\"/' {kmt_os.qemu_conf}"
        )
        ctx.run(f"{sudo} sed --in-place 's/#user = \"root\"/user = \"{user}\"/' {kmt_os.qemu_conf}")
        ctx.run(f"{sudo} sed --in-place 's/#group = \"root\"/group = \"kvm\"/' {kmt_os.qemu_conf}")

        kmt_os.restart_libvirtd(ctx, sudo)

    # download dependencies
    if not lite:
        download_rootfs(ctx, kmt_os.rootfs_dir, "system-probe")
        gen_ssh_key(ctx, kmt_os.kmt_dir)

    # build docker compile image
    kmt_os.assert_user_in_docker_group(ctx)
    info(f"[+] User '{os.getlogin()}' in group 'docker'")

    build_compiler(ctx)

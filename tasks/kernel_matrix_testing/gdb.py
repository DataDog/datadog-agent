from __future__ import annotations

from typing import TYPE_CHECKING

import semver
from invoke.context import Context

from tasks.kernel_matrix_testing import stacks
from tasks.kernel_matrix_testing.infra import LibvirtDomain, build_infrastructure
from tasks.kernel_matrix_testing.platforms import get_platforms
from tasks.kernel_matrix_testing.tool import Exit, info
from tasks.kernel_matrix_testing.vars import KMTPaths
from tasks.libs.common.utils import get_repo_root
from tasks.libs.types.arch import Arch

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import (
        Component,  # noqa: F401
        KMTArchNameOrLocal,
    )


class GDBPaths:
    def __init__(self, vm_tag: str, image_version: str, stack: str, arch: KMTArchNameOrLocal):
        self.tag = vm_tag
        self.image_version = image_version
        self.kmt_paths = KMTPaths(stack, Arch.from_str(arch))

    @property
    def vmlinux(self):
        return self.kmt_paths.gdb / self.tag / self.image_version / "vmlinux.dbg"

    @property
    def kernel_source(self):
        return self.kmt_paths.gdb / self.tag / self.image_version / "kernel-source"


class UbuntuGDBProvision:
    def __init__(self, vm: LibvirtDomain, image_version: str, kernel: str):
        self.target = vm
        self.image_version = image_version
        self.kernel = semver.VersionInfo.parse(kernel)

    def run(self, ctx: Context, stack: str):
        self.target.copy(
            ctx, get_repo_root() / "tasks/kernel_matrix_testing/provision/ubuntu-dbg.sh", "/tmp/provision.sh"
        )
        self.target.run_cmd(ctx, "chmod +x /tmp/provision.sh && /tmp/provision.sh")

        gdb_paths = GDBPaths(self.target.tag, self.image_version, stack, self.target.arch)
        gdb_paths.vmlinux.parent.mkdir(exist_ok=True, parents=True)
        self.target.download(ctx, "/usr/lib/debug/boot/vmlinux.dbg", f"{gdb_paths.vmlinux}")

        # make sure we are deleteing from a sane path
        if gdb_paths.kernel_source.name == "kernel-source" and f"{gdb_paths.kernel_source.absolute()}".startswith(
            f"{get_repo_root().absolute()}"
        ):
            ctx.run(f"rm -rf {gdb_paths.kernel_source}")

        gdb_paths.kernel_source.mkdir(parents=True)
        self.target.download(
            ctx,
            f"/usr/src/linux-source-{self.kernel.finalize_version()}/linux-source-{self.kernel.finalize_version()}.tar.bz2",
            f"{gdb_paths.kernel_source.parent}",
        )
        ctx.run(
            f"cd {gdb_paths.kernel_source.parent} && tar xvf linux-source-{self.kernel.finalize_version()}.tar.bz2 -C {gdb_paths.kernel_source} --strip-components=1",
            hide="out",
            echo=True,
        )

        if self.kernel > semver.VersionInfo.parse("4.4.0"):
            ctx.run(f"cd {gdb_paths.kernel_source} && make defconfig && make scripts_gdb")

        self.target.run_cmd(ctx, "shutdown -h now", verbose=True, allow_fail=True)


gdb_provision = {
    "ubuntu": {
        "22.04": UbuntuGDBProvision,
        "23.10": UbuntuGDBProvision,
        "24.04": UbuntuGDBProvision,
        "24.10": UbuntuGDBProvision,
        "20.04": UbuntuGDBProvision,
        # TODO: Add support for bionic/ubuntu_18.04. Currently failing to find debug kernel build.
        "16.04": UbuntuGDBProvision,
    }
}


def setup_gdb_debugging(ctx: Context, stack: str) -> None:
    infra = build_infrastructure(stack)
    platforms = get_platforms()

    arch = Arch.local().kmt_arch
    for kmt_arch, instance in infra.items():
        if kmt_arch != "local":
            # TODO: add support to attach gdb to remote VMs
            raise Exit("stacks with remote VMs cannot be launched with GDB")

        for vm in instance.microvms:
            platinfo = platforms[arch][vm.tag]
            os_id = platinfo['os_id']
            os_version = platinfo['os_version']
            image_version = platinfo['image_version']
            kernel = platinfo['kernel']

            if os_id not in gdb_provision:
                raise Exit(f"{os_id} is currently not supported for kernel debugging")

            if os_version not in gdb_provision[os_id]:
                raise Exit(f"{os_id}_{os_version} is currently not supported for kernel debugging")

            provisioner = gdb_provision[os_id][os_version](vm, image_version, kernel)
            info(f"[+] Provisioning {vm.tag} for debugging.")
            provisioner.run(ctx, stack)

    stacks.pause_stack(stack)
    stacks.resume_stack(stack)

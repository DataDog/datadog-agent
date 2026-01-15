import getpass
import platform

from invoke.context import Context

from tasks.libs.common.status import Status

from ..kmt_os import get_kmt_os
from .requirement import Requirement, RequirementState
from .utils import UbuntuPackageManager, check_directories, check_user_in_group, ensure_options_in_config


def get_requirements() -> list[Requirement]:
    return [
        LinuxPackages(),
        NFSKernelServer(),
        LinuxQemuConfig(),
        NFSExport(),
        LinuxLocalVMDirectories(),
        LinuxGroups(),
    ]


class LinuxPackages(Requirement):
    @property
    def _packages(self) -> list[str]:
        packages = [
            "aria2",
            "fio",
            "socat",
            "qemu-system",
            "libvirt-daemon-system",
            "curl",
            "debootstrap",
            "libguestfs-tools",
            "libvirt-dev",
            "python3-pip",
            "nfs-kernel-server",
            "rpcbind",
            "ssh-askpass",
            "clang",  # required to build libvirt-python
        ]

        if platform.machine() == "aarch64":
            packages.append("qemu-efi-aarch64")

        return packages

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        return UbuntuPackageManager(ctx).check(self._packages, fix)


class NFSKernelServer(Requirement):
    def check(self, ctx: Context, fix: bool) -> RequirementState:
        service_name = "nfs-kernel-server.service"
        res = ctx.run(f"systemctl is-active {service_name}", warn=True, hide=True)
        if res is None or not res.ok:
            if not fix:
                return RequirementState(Status.FAIL, "NFS kernel server is not enabled.", fixable=True)

            ctx.run(f"sudo systemctl enable --now {service_name}")

        return RequirementState(Status.OK, "NFS kernel server is enabled.")


class LinuxQemuConfig(Requirement):
    def check(self, ctx: Context, fix: bool):
        from ..kmt_os import Linux
        from ..tool import is_root

        user = getpass.getuser()

        expected_options: dict[str, str | int] = {
            "security_driver": "none",
            "user": user,
            "group": "kvm",
        }

        try:
            incorrect_options = ensure_options_in_config(
                ctx,
                Linux.qemu_conf,
                expected_options,
                change=fix,
                write_with_sudo=not is_root(),
                read_with_sudo=not is_root(),
            )
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to check QEMU config: {e}")

        if len(incorrect_options) == 0:
            return RequirementState(Status.OK, "QEMU config is correct.")
        elif fix:
            try:
                sudo = "sudo" if not is_root() else ""
                ctx.run(f"{sudo} systemctl restart libvirtd.service")
            except Exception as e:
                return RequirementState(Status.WARN, f"Failed to restart libvirtd to apply changes {e}")

            return RequirementState(Status.OK, "QEMU config fixed.")


class NFSExport(Requirement):
    def check(self, ctx: Context, fix: bool) -> RequirementState:
        export_line = "/opt/kernel-version-testing 100.0.0.0/8(ro,no_root_squash,no_subtree_check)"
        with open("/etc/exports") as f:
            if export_line in f.read():
                return RequirementState(Status.OK, "NFS export already present.")
        if not fix:
            return RequirementState(Status.FAIL, "NFS export missing.", fixable=True)

        try:
            ctx.run(f"sudo sh -c 'echo \"{export_line}\" >> /etc/exports'")
            ctx.run("sudo exportfs -a")
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to add NFS export: {e}")

        return RequirementState(Status.OK, "NFS export added.", fixable=True)


class LinuxLocalVMDirectories(Requirement):
    dependencies: list[type[Requirement]] = [LinuxPackages]

    def check(self, ctx: Context, fix: bool) -> list[RequirementState]:
        kmt_os = get_kmt_os()
        dirs = [
            kmt_os.libvirt_dir,
            kmt_os.rootfs_dir,
        ]

        user = getpass.getuser()
        group = kmt_os.libvirt_group

        return check_directories(ctx, dirs, fix, user, group, 0o755)


class LinuxGroups(Requirement):
    def check(self, ctx: Context, fix: bool) -> list[RequirementState]:
        groups = ["kvm", "libvirt"]
        states = []
        for group in groups:
            states.append(check_user_in_group(ctx, group, fix=fix))

        return states

import getpass
import os
import sys

from invoke import Context

from tasks.kernel_matrix_testing.tool import Exit
from tasks.system_probe import is_root


def get_home_linux():
    return os.path.join("/", "/home", "kernel-version-testing")


def get_home_macos():
    return os.path.join(os.path.expanduser('~'), "kernel-version-testing")


def get_kmt_os():
    if sys.platform == "linux" or sys.platform == "linux2":
        return Linux
    elif sys.platform == "darwin":
        return MacOS

    raise Exit(f"unsupported platform: {sys.platform}")


class Linux:
    kmt_dir = get_home_linux()
    libvirt_group = "libvirt"
    rootfs_dir = os.path.join(kmt_dir, "rootfs")
    stacks_dir = os.path.join(kmt_dir, "stacks")
    packages_dir = os.path.join(kmt_dir, "kernel-packages")
    libvirt_dir = os.path.join(kmt_dir, "libvirt")
    shared_dir = os.path.join("/", "opt", "kernel-version-testing")
    libvirt_socket = "qemu:///system"

    qemu_conf = os.path.join("/", "etc", "libvirt", "qemu.conf")

    @staticmethod
    def restart_libvirtd(ctx, sudo):
        ctx.run(f"{sudo} systemctl restart libvirtd.service")

    @staticmethod
    def assert_user_in_docker_group(ctx):
        ctx.run("cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep 'docker:' | cut -d ':' -f 3)")

    @staticmethod
    def init_local(ctx):
        sudo = "sudo" if not is_root() else ""
        user = getpass.getuser()

        ctx.run(
            f"{sudo} sed --in-place 's/#security_driver = \"selinux\"/security_driver = \"none\"/' {Linux.qemu_conf}"
        )
        ctx.run(f"{sudo} sed --in-place 's/#user = \"root\"/user = \"{user}\"/' {Linux.qemu_conf}")
        ctx.run(f"{sudo} sed --in-place 's/#group = \"root\"/group = \"kvm\"/' {Linux.qemu_conf}")

        Linux.restart_libvirtd(ctx, sudo)


class MacOS:
    kmt_dir = get_home_macos()
    libvirt_group = "staff"
    rootfs_dir = os.path.join(kmt_dir, "rootfs")
    stacks_dir = os.path.join(kmt_dir, "stacks")
    packages_dir = os.path.join(kmt_dir, "kernel-packages")
    libvirt_dir = os.path.join(kmt_dir, "libvirt")
    libvirt_conf = "/opt/homebrew/etc/libvirt/libvirtd.conf"
    shared_dir = os.path.join("/", "opt", "kernel-version-testing")
    libvirt_system_dir = "/opt/homebrew/var/run/libvirt"
    libvirt_socket = f"qemu:///session?socket={libvirt_system_dir}/libvirt-sock"

    @staticmethod
    def assert_user_in_docker_group(_):
        return True

    @staticmethod
    def init_local(ctx: Context):
        ctx.run("brew install libvirt")
        ctx.run(
            f"gsed -i -E 's%(# *)?unix_sock_dir = .*%unix_sock_dir = \"{MacOS.libvirt_system_dir}\"%' {MacOS.libvirt_conf}"
        )

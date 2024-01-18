import os
import sys

from .tool import Exit


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

    qemu_conf = os.path.join("/", "etc", "libvirt", "qemu.conf")

    @staticmethod
    def restart_libvirtd(ctx, sudo):
        ctx.run(f"{sudo} systemctl restart libvirtd.service")

    @staticmethod
    def assert_user_in_docker_group(ctx):
        ctx.run("cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep 'docker:' | cut -d ':' -f 3)")


class MacOS:
    kmt_dir = get_home_macos()
    libvirt_group = "staff"
    rootfs_dir = os.path.join(kmt_dir, "rootfs")
    stacks_dir = os.path.join(kmt_dir, "stacks")
    packages_dir = os.path.join(kmt_dir, "kernel-packages")
    libvirt_dir = os.path.join(kmt_dir, "libvirt")
    shared_dir = os.path.join("/", "opt", "kernel-version-testing")

    @staticmethod
    def assert_user_in_docker_group(_):
        return True

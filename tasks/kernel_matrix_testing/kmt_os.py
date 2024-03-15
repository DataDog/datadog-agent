import os
import sys

from .tool import Exit


def get_home_linux():
    return os.path.join("/", "/home", "kernel-version-testing")


def get_kmt_os():
    if sys.platform == "linux" or sys.platform == "linux2":
        return Linux
    elif sys.platform == "darwin":
        return MacOS

    raise Exit(f"unsupported platform: {sys.platform}")


class Linux:
    kmt_dir = get_home_linux()
    rootfs_dir = os.path.join(kmt_dir, "rootfs")
    stacks_dir = os.path.join(kmt_dir, "stacks")
    packages_dir = os.path.join(kmt_dir, "kernel-packages")
    backup_dir = os.path.join(kmt_dir, "backups")
    libvirt_dir = os.path.join(kmt_dir, "libvirt")
    shared_dir = os.path.join("/", "opt", "kernel-version-testing")
    kheaders_dir = os.path.join(shared_dir, "kernel-headers")

    qemu_conf = os.path.join("/", "etc", "libvirt", "qemu.conf")

    def restart_libvirtd(self, ctx, sudo):
        ctx.run(f"{sudo} systemctl restart libvirtd.service")

    def assert_user_in_docker_group(self, ctx):
        ctx.run("cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep 'docker:' | cut -d ':' -f 3)")


class MacOS:
    pass

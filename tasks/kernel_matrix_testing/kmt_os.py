import sys
import os

from .tool import Exit

def get_home_linux():
    return os.path.join("/", "/home", "kernel-version-testing")
    #return os.path.join("/", "opt", "kernel-matrix-testing")

def get_home_macos():
    return ""

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
    shared_dir = os.path.join(kmt_dir, "vm-shared")
    kheaders_dir = os.path.join(shared_dir, "kernel-headers")

    qemu_conf = os.path.join("/", "etc", "libvirt", "qemu.conf")

    def restart_libvirtd(ctx, sudo):
        ctx.run(f"{sudo} systemctl restart libvirtd.service")

    def assert_user_in_docker_group(ctx):
        ctx.run("cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep 'docker:' | cut -d ':' -f 3)")

class MacOS:
    pass

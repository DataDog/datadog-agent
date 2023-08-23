import getpass
import os
from pathlib import Path

from .download import download_kernel_packages, download_rootfs
from .tool import info

KMT_DIR = os.path.join("/", "home", "kernel-version-testing")
KMT_ROOTFS_DIR = os.path.join(KMT_DIR, "rootfs")
KMT_STACKS_DIR = os.path.join(KMT_DIR, "stacks")
KMT_PACKAGES_DIR = os.path.join(KMT_DIR, "kernel-packages")
KMT_BACKUP_DIR = os.path.join(KMT_DIR, "backups")
KMT_LIBVIRT_DIR = os.path.join(KMT_DIR, "libvirt")
KMT_SHARED_DIR = os.path.join("/", "opt", "kernel-version-testing")
KMT_KHEADERS_DIR = os.path.join("/", "opt", "kernel-version-testing", "kernel-headers")

VMCONFIG = "vm-config.json"

priv_key = """-----BEGIN OPENSSH PRIVATE KEY-----
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
-----END OPENSSH PRIVATE KEY-----
"""


def is_root():
    return os.getuid() == 0


def get_active_branch_name():
    head_dir = Path(".") / ".git" / "HEAD"
    with head_dir.open("r") as f:
        content = f.read().splitlines()

    for line in content:
        if line[0:4] == "ref:":
            return line.partition("refs/heads/")[2].replace("/", "-")


def check_and_get_stack(stack):
    if stack is None:
        stack = get_active_branch_name()

    if not stack.endswith("-ddvm"):
        return f"{stack}-ddvm"
    else:
        return stack


def gen_ssh_key(ctx):
    with open(f"{KMT_DIR}/ddvm_rsa", "w") as f:
        f.write(priv_key)

    ctx.run(f"chmod 400 {KMT_DIR}/ddvm_rsa")


def init_kernel_matrix_testing_system(ctx, lite):
    sudo = "sudo" if not is_root() else ""
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd $USER | cut -d ':' -f 1) {KMT_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd $USER | cut -d ':' -f 1) {KMT_PACKAGES_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd $USER | cut -d ':' -f 1) {KMT_BACKUP_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd $USER | cut -d ':' -f 1) {KMT_STACKS_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd $USER | cut -d ':' -f 1) {KMT_LIBVIRT_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd $USER | cut -d ':' -f 1) {KMT_ROOTFS_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd $USER | cut -d ':' -f 1) {KMT_SHARED_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd $USER | cut -d ':' -f 1) {KMT_KHEADERS_DIR}")

    ## fix libvirt conf
    user = getpass.getuser()
    ctx.run(
        f"{sudo} sed --in-place 's/#security_driver = \"selinux\"/security_driver = \"none\"/' /etc/libvirt/qemu.conf"
    )
    ctx.run(f"{sudo} sed --in-place 's/#user = \"root\"/user = \"{user}\"/' /etc/libvirt/qemu.conf")
    ctx.run(f"{sudo} sed --in-place 's/#group = \"root\"/group = \"kvm\"/' /etc/libvirt/qemu.conf")
    ctx.run(f"{sudo} systemctl restart libvirtd.service")

    # download dependencies
    if not lite:
        download_rootfs(ctx, KMT_ROOTFS_DIR, KMT_BACKUP_DIR)
        download_kernel_packages(ctx, KMT_PACKAGES_DIR, KMT_KHEADERS_DIR, KMT_BACKUP_DIR)
        gen_ssh_key(ctx)

    # build docker compile image
    ctx.run("cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep 'docker:' | cut -d ':' -f 3)")
    info(f"[+] User '{os.getlogin()}' in group 'docker'")
    ctx.run("docker build -f ../datadog-agent-buildimages/system-probe_x64/Dockerfile -t kmt:compile .")

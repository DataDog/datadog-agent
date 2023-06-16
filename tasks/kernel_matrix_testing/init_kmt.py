import os
from pathlib import Path
from invoke.exceptions import Exit
import platform
from glob import glob
import getpass

KMT_DIR = os.path.join("/", "home", "kernel-version-testing")
KMT_ROOTFS_DIR = os.path.join(KMT_DIR, "rootfs")
KMT_STACKS_DIR = os.path.join(KMT_DIR, "stacks")
KMT_PACKAGES_DIR = os.path.join(KMT_DIR, "kernel-packages")
KMT_BACKUP_DIR = os.path.join(KMT_DIR, "backups")
KMT_LIBVIRT_DIR = os.path.join(KMT_DIR, "libvirt")
KMT_SHARED_DIR = os.path.join("/", "opt", "kernel-version-testing")
KMT_KHEADERS_DIR = os.path.join("/", "opt", "kernel-version-testing", "kernel-headers")

VMCONFIG = "vm-config.json"

archs_mapping = {
    "amd64": "x86_64",
    "x86": "x86_64",
    "x86_64": "x86_64",
    "arm64": "arm64",
    "aarch64": "arm64",
    "arm": "arm64",
    "local": "local",
}

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


def check_and_get_stack(stack, branch):
    if stack is None and not branch:
        raise Exit("Stack name required if not using current branch")

    if stack and branch:
        raise Exit("Cannot specify stack when branch parameter is set")

    if branch:
        stack = get_active_branch_name()

    if not stack.endswith("-ddvm"):
        return f"{stack}-ddvm"
    else:
        return stack

def revert_kernel_packages(ctx):
    arch = archs_mapping[platform.machine()]
    kernel_packages_sum = f"kernel-packages-{arch}.sum"
    kernel_packages_tar = f"kernel-packages-{arch}.tar"
    ctx.run(f"rm -f {KMT_PACKAGES_DIR}/*")
    ctx.run(f"mv {KMT_BACKUP_DIR}/{kernel_packages_sum} {KMT_PACKAGES_DIR}")
    ctx.run(f"mv {KMT_BACKUP_DIR}/{kernel_packages_tar} {KMT_PACKAGES_DIR}")
    ctx.run(f"tar xvf {KMT_PACKAGES_DIR}/{kernel_packages_tar} | xargs -i tar xzf {{}}")


def revert_rootfs(ctx, rootfs):
    ctx.run(f"rm -f {KMT_ROOTFS_DIR}/*")
    ctx.run(f"mv {KMT_ROOTFS_DIR}/{rootfs}.sum {KMT_ROOTFS_DIR}")
    ctx.run(f"mv {KMT_ROOTFS_DIR}/{rootfs}.tar.gz {KMT_ROOTFS_DIR}")


def download_rootfs(ctx, revert=False):
    arch = archs_mapping[platform.machine()]
    if arch == "x86_64":
        rootfs = "rootfs-amd64"
    elif arch == "arm64":
        rootfs = "rootfs-arm64"
    else:
        Exit(f"Unsupported arch detected {arch}")

    # download rootfs
    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{rootfs}.sum -O {KMT_ROOTFS_DIR}/{rootfs}.sum"
    )
    if not res.ok:
        if revert:
            revert_rootfs(ctx)
        raise Exit("Failed to download rootfs check sum file")

    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{rootfs}.tar.gz -O {KMT_ROOTFS_DIR}/{rootfs}.tar.gz"
    )
    if not res.ok:
        if revert:
            revert_rootfs(ctx)
        raise Exit("Failed to download rootfs")

    # extract rootfs
    res = ctx.run(f"cd {KMT_ROOTFS_DIR} && tar xzvf {rootfs}.tar.gz")
    if not res.ok:
        if revert:
            revert_rootfs(ctx)
        raise Exit("Failed to extract rootfs")

    # set permissions
    res = ctx.run(f"find {KMT_ROOTFS_DIR} -name \"*qcow*\" -type f -exec chmod 0766 {{}} \\;")
    if not res.ok:
        if revert:
            revert_rootfs(ctx)
        raise Exit("Failed to set permissions 0766 to rootfs")


def download_kernel_packages(ctx, revert=False):
    arch = archs_mapping[platform.machine()]
    kernel_packages_sum = f"kernel-packages-{arch}.sum"
    kernel_packages_tar = f"kernel-packages-{arch}.tar"

    # download kernel packages
    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_tar} -O {KMT_PACKAGES_DIR}/{kernel_packages_tar}",
        warn=True,
    )
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx)
        raise Exit("Failed to download kernel pacakges")

    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_sum} -O {KMT_PACKAGES_DIR}/{kernel_packages_sum}",
        warn=True,
    )
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx)
        raise Exit("Failed to download kernel pacakges checksum")

    # extract pacakges
    res = ctx.run(f"cd {KMT_PACKAGES_DIR} && tar xvf {kernel_packages_tar} | xargs -i tar xzf {{}}")
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx)
        raise Exit("Failed to extract kernel packages")

    # set permissions
    packages = glob(f"{KMT_PACKAGES_DIR}/kernel-v*")
    for pkg in packages:
        if not os.path.isdir(pkg):
            continue
        # set package dir as rwx for all
        os.chmod(pkg, 0o766)
        files = glob(f"{pkg}/*")
        for f in files:
            if not os.path.isdir(f):
                # set all files to rw for all
                os.chmod(f, 0o666)

    # copy headers
    res = ctx.run(
        f"find {KMT_PACKAGES_DIR} -name 'linux-image-*' -type f | xargs -i cp {{}} {KMT_KHEADERS_DIR} && find {KMT_PACKAGES_DIR} -name 'linux-headers-*' -type f | xargs -i cp {{}} {KMT_KHEADERS_DIR}"
    )
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx)
        raise Exit(f"failed to copy kernel headers to shared dir {KMT_KHEADERS_DIR}")


def gen_ssh_key(ctx):
    with open(f"{KMT_DIR}/ddvm_rsa", "w") as f:
        f.write(priv_key)

    ctx.run(f"chmod 400 {KMT_DIR}/ddvm_rsa")



def init_kernel_matrix_testing_system(ctx):
    sudo = "sudo" if not is_root() else ""
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_PACKAGES_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_BACKUP_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_STACKS_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_LIBVIRT_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_ROOTFS_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_SHARED_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_KHEADERS_DIR}")

    ## fix libvirt conf
    user = getpass.getuser()
    ctx.run(
        f"{sudo} sed --in-place 's/#security_driver = \"selinux\"/security_driver = \"none\"/' /etc/libvirt/qemu.conf"
    )
    ctx.run(f"{sudo} sed --in-place 's/#user = \"root\"/user = \"{user}\"/' /etc/libvirt/qemu.conf")
    ctx.run(f"{sudo} sed --in-place 's/#group = \"root\"/group = \"kvm\"/' /etc/libvirt/qemu.conf")
    ctx.run(f"{sudo} systemctl restart libvirtd.service")

    # download dependencies
    download_kernel_packages(ctx)
    download_rootfs(ctx)
    gen_ssh_key(ctx)



import getpass
import os
from pathlib import Path

from tasks.kernel_matrix_testing.compiler import build_compiler
from tasks.kernel_matrix_testing.download import download_rootfs
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import info, is_root

VMCONFIG = "vmconfig.json"


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


def gen_ssh_key(ctx, kmt_dir):
    ctx.run(f"cp tasks/kernel_matrix_testing/ddvm_rsa {kmt_dir}")
    ctx.run(f"chmod 600 {kmt_dir}/ddvm_rsa")


def init_kernel_matrix_testing_system(ctx, lite):
    kmt_os = get_kmt_os()

    if not lite:
        kmt_os.init_local(ctx)

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
        download_rootfs(ctx, kmt_os.rootfs_dir)
        gen_ssh_key(ctx, kmt_os.kmt_dir)

    # build docker compile image
    kmt_os.assert_user_in_docker_group(ctx)
    info(f"[+] User '{os.getlogin()}' in group 'docker'")

    build_compiler(ctx)

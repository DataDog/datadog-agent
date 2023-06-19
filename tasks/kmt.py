import os
import filecmp
import libvirt
import platform
from invoke import task
from invoke.exceptions import Exit
from glob import glob
from .kernel_matrix_testing import vmconfig
from .kernel_matrix_testing.init_kmt import check_and_get_stack
from .kernel_matrix_testing import stacks


@task
def create_stack(ctx, stack=None, branch=False):
    stacks.create_stack(ctx, stack, branch)


@task(
    help={
        "vms": "Comma seperated List of VMs to setup. Each definition must contain the following elemets (recipe, architecture, version).",
        "stack": "Name of the stack within which to generate the configuration file",
        "vcpu": "Comma seperated list of CPUs, to launch each VM with",
        "memory": "Comma seperated list of memory to launch each VM with. Automatically rounded up to power of 2",
        "new": "Generate new configuration file instead of appending to existing one within the provided stack",
    }
)
def gen_config(ctx, stack=None, branch=False, vms="", init_stack=False, vcpu="4", memory="8192", new=False):
    vmconfig.gen_config(ctx, stack, branch, vms, init_stack, vcpu, memory, new)
    


def update_kernel_packages(ctx):
    arch = archs_mapping[platform.machine()]
    kernel_packages_sum = f"kernel-packages-{arch}.sum"
    kernel_packages_tar = f"kernel-packages-{arch}.tar"

    ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_sum} -O /tmp/{kernel_packages_sum}"
    )

    current_sum_file = f"{KMT_PACKAGES_DIR}/{kernel_packages_sum}"
    if filecmp.cmp(current_sum_file, f"/tmp/{kernel_packages_sum}"):
        print("[-] No update required for custom kernel packages")

    # backup kernel-packges
    karch = karch_mapping[archs_mapping[platform.machine()]]
    ctx.run(
        f"find {KMT_PACKAGES_DIR} -name \"kernel-*.{karch}.pkg.tar.gz\" -type f | rev | cut -d '/' -f 1  | rev > /tmp/package.ls"
    )
    ctx.run(f"cd {KMT_PACKAGES_DIR} && tar -cf {kernel_packages_tar} -T /tmp/package.ls")
    ctx.run(f"cp {KMT_PACKAGES_DIR}/{kernel_packages_tar} {KMT_BACKUP_DIR}")
    ctx.run(f"cp {current_sum_file} {KMT_BACKUP_DIR}")
    print("[+] Backed up current packages")

    # clean kernel packages directory
    ctx.run(f"rm -f {KMT_PACKAGES_DIR}/*")

    download_kernel_packages(ctx, revert=True)

    print("[+] Kernel packages successfully updated")



def update_rootfs(ctx):
    arch = archs_mapping[platform.machine()]
    if arch == "x86_64":
        rootfs = "rootfs-amd64"
    elif arch == "arm64":
        rootfs = "rootfs-arm64"
    else:
        Exit(f"Unsupported arch detected {arch}")

    ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{rootfs}.sum -O /tmp/{rootfs}.sum"
    )

    current_sum_file = f"{KMT_ROOTFS_DIR}/{rootfs}.sum"
    if filecmp.cmp(current_sum_file, "/tmp/{rootfs}.sum"):
        print("[-] No update required for root filesystems and bootable images")

    # backup rootfs
    ctx.run("cp {KMT_ROOTFS_DIR}/{rootfs}.tar.gz {KMT_BACKUP_DIR}")
    ctx.run("cp {KMT_ROOTFS_DIR}/{rootfs}.sum {KMT_BACKUP_DIR}")
    print("[+] Backed up rootfs")

    # clean rootfs directory
    ctx.run(f"rm -f {KMT_ROOTFS_DIR}/*")

    download_rootfs(ctx, revert=True)

    print("[+] Root filesystem and bootables images updated")


@task
def update_resources(ctx):
    print("Updating resource dependencies will delete all running stacks.")
    if input("are you sure you want to continue? (y/n)") != "y":
        print("[-] Update aborted")
        return

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack)

    update_kernel_packages(ctx)
    update_rootfs(ctx)


@task
def revert_resources(ctx):
    print("Reverting resource dependencies will delete all running stacks.")
    if input("are you sure you want to revert to backups? (y/n)") != "y":
        print("[-] Revert aborted")
        return

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack)

    revert_kernel_packages(ctx)
    revert_rootfs(ctx)

    print("[+] Reverted successfully")



@task
def launch_stack(
    ctx, stack=None, branch=False, ssh_key="", x86_ami="ami-0584a00dd384af6ab", arm_ami="ami-0b7cd13521845570c"
):
    stacks.launch_stack(ctx, stack, branch, ssh_key, x86_ami, arm_ami)

@task
def destroy_stack(ctx, stack=None, branch=False, force=False, ssh_key=""):
    stacks.destroy_stack(ctx, stack, branch, force, ssh_key)

@task
def init_stack(ctx):
    init_kmt.init_kernel_matrix_testing_system(ctx)

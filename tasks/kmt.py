from invoke import task
from glob import glob
from .kernel_matrix_testing.init_kmt import (
    KMT_STACKS_DIR,
    update_kernel_packages,
    update_rootfs,
    revert_rootfs,
    init_kernel_matrix_testing_system,
    revert_kernel_packages,
)
from .kernel_matrix_testing import vmconfig
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


@task
def update_resources(ctx):
    print("Updating resource dependencies will delete all running stacks.")
    if input("are you sure you want to continue? (y/n)") != "y":
        print("[-] Update aborted")
        return

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack, force=True)

    update_kernel_packages(ctx)
    update_rootfs(ctx)


@task
def revert_resources(ctx):
    print("Reverting resource dependencies will delete all running stacks.")
    if input("are you sure you want to revert to backups? (y/n)") != "y":
        print("[-] Revert aborted")
        return

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack, force=True)

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
def init(ctx):
    init_kernel_matrix_testing_system(ctx)

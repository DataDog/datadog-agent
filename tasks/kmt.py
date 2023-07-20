import re
from glob import glob

from invoke import task

from .kernel_matrix_testing import stacks, vmconfig
from .kernel_matrix_testing.download import revert_kernel_packages, revert_rootfs, update_kernel_packages, update_rootfs
from .kernel_matrix_testing.init_kmt import (
    KMT_BACKUP_DIR,
    KMT_DIR,
    KMT_KHEADERS_DIR,
    KMT_PACKAGES_DIR,
    KMT_ROOTFS_DIR,
    KMT_STACKS_DIR,
    check_and_get_stack,
    init_kernel_matrix_testing_system,
)
from .kernel_matrix_testing.tool import Exit, ask, info, warn

try:
    from tabulate import tabulate
except ImportError:
    tabulate = None

X86_AMI_ID_SANDBOX = "ami-0d1f81cfdbd5b0188"
ARM_AMI_ID_SANDBOX = "ami-02cb18e91afb3777c"


@task
def create_stack(ctx, stack=None):
    stacks.create_stack(ctx, stack)


@task(
    help={
        "vms": "Comma separated List of VMs to setup. Each definition must contain the following elemets (recipe, architecture, version).",
        "stack": "Name of the stack within which to generate the configuration file",
        "vcpu": "Comma separated list of CPUs, to launch each VM with",
        "memory": "Comma separated list of memory to launch each VM with. Automatically rounded up to power of 2",
        "new": "Generate new configuration file instead of appending to existing one within the provided stack",
        "init-stack": "Automatically initialize stack if not present. Equivalent to calling 'inv -e kmt.create-stack [--stack=<stack>]'",
    }
)
def gen_config(ctx, stack=None, vms="", init_stack=False, vcpu="4", memory="8192", new=False):
    vmconfig.gen_config(ctx, stack, vms, init_stack, vcpu, memory, new)


@task
def launch_stack(ctx, stack=None, ssh_key="", x86_ami=X86_AMI_ID_SANDBOX, arm_ami=ARM_AMI_ID_SANDBOX):
    stacks.launch_stack(ctx, stack, ssh_key, x86_ami, arm_ami)


@task
def destroy_stack(ctx, stack=None, force=False, ssh_key=""):
    stacks.destroy_stack(ctx, stack, force, ssh_key)


@task
def pause_stack(stack=None):
    stacks.pause_stack(stack)


@task
def resume_stack(stack=None):
    stacks.resume_stack(stack)


@task
def stack(ctx, stack=None):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    ctx.run(f"cat {KMT_STACKS_DIR}/{stack}/stack.outputs")


@task
def ls(_, distro=False, custom=False):
    print(tabulate(vmconfig.get_image_list(distro, custom), headers='firstrow', tablefmt='fancy_grid'))


@task
def init(ctx, lite=False):
    init_kernel_matrix_testing_system(ctx, lite)


@task
def update_resources(ctx):
    warn("Updating resource dependencies will delete all running stacks.")
    if ask("are you sure you want to continue? (Y/n)") != "Y":
        raise Exit("[-] Update aborted")

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack, force=True)

    update_kernel_packages(ctx, KMT_PACKAGES_DIR, KMT_KHEADERS_DIR, KMT_BACKUP_DIR)
    update_rootfs(ctx, KMT_ROOTFS_DIR, KMT_BACKUP_DIR)


@task
def revert_resources(ctx):
    warn("Reverting resource dependencies will delete all running stacks.")
    if ask("are you sure you want to revert to backups? (Y/n)") != "Y":
        raise Exit("[-] Revert aborted")

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack, force=True)

    revert_kernel_packages(ctx, KMT_PACKAGES_DIR, KMT_BACKUP_DIR)
    revert_rootfs(ctx, KMT_ROOTFS_DIR, KMT_BACKUP_DIR)

    info("[+] Reverted successfully")


def get_vm_ip(stack, version, arch):
    with open(f"{KMT_STACKS_DIR}/{stack}/stack.outputs", 'r') as f:
        entries = f.readlines()
        for entry in entries:
            match = re.search(f"^.+{arch}-{version}.+\\s+.+$", entry.strip('\n'))
            if match is None:
                continue

            return arch, match.group(0).split(' ')[0], match.group(0).split(' ')[1]


def get_instance_ip(stack, arch):
    with open(f"{KMT_STACKS_DIR}/{stack}/stack.outputs", 'r') as f:
        entries = f.readlines()
        for entry in entries:
            if f"{arch}-instance-ip" in entry.split(' ')[0]:
                return entry.split()[0], entry.split()[1].strip('\n')


@task
def sync(ctx, stack=None):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    vm_types = vms.split(',')
    if len(vm_types) == 0:
        raise Exit("No VMs to lookup")

    possible = vmconfig.list_possible()
    target_vms = list()
    for vm in vm_types:
        _, version, arch = vmconfig.normalize_vm_def(possible, vm)
        target = get_vm_ip(stack, version, arch)
        target_vms.append(target)
        if arch != "local" and ssh_key == "":
            raise Exit("`ssh_key` is required when syncing VMs on remote instance")

    info("[*] VMs to sync")
    for _, vm, ip in target_vms:
        info(f"    Syncing VM {vm} with ip {ip}")

    if ask("Do you want to sync? (y/n)") != "y":
        warn("[-] Sync aborted !")
        return

    info("[*] Beginning sync...")

    if ssh_key != "":
        ssh_key_path = f"~/.ssh/{ssh_key}.pem"

    for arch, _, ip in target_vms:
        vm_copy = f"rsync -e \\\"ssh -o StrictHostKeyChecking=no -i {KMT_DIR}/ddvm_rsa\\\" --chmod=F644 --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' ./ root@{ip}:/root/datadog-agent"
        if arch == "local":
            ctx.run(
                f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {KMT_DIR}/ddvm_rsa\" --chmod=F644 --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' ./ root@{ip}:/root/datadog-agent"
            )
        elif arch == "x86_64" or arch == "arm64":
            instance_name, instance_ip = get_instance_ip(stack, arch)
            info(f"[*] Instance {instance_name} has ip {instance_ip}")
            ctx.run(
                f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {ssh_key_path}\" --chmod=F644 --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' ./ ubuntu@{instance_ip}:/home/ubuntu/datadog-agent"
            )
            ctx.run(
                f"ssh -i {ssh_key_path} -o StrictHostKeyChecking=no ubuntu@{instance_ip} \"cd /home/ubuntu/datadog-agent && {vm_copy}\""
            )
        else:
            raise Exit(f"Unsupported arch {arch}")

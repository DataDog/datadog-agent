import re
import os
import platform
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
    KMT_SHARED_DIR,
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

DOCKER_EXEC = "docker exec -i kmt-compiler bash -c"

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

def build_target_set(stack, vms, ssh_key):
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

    return target_vms

def sync_source(ctx, vm_ls, source, target):
    for arch, _, ip in vm_ls:
        vm_copy = f"rsync -e \\\"ssh -o StrictHostKeyChecking=no -i {KMT_DIR}/ddvm_rsa\\\" --chmod=F644 --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' ./ root@{ip}:{target}"
        if arch == "local":
            ctx.run(
                f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {KMT_DIR}/ddvm_rsa\" --chmod=F644 --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' {source} root@{ip}:{target}"
            )
        elif arch == "x86_64" or arch == "arm64":
            instance_name, instance_ip = get_instance_ip(stack, arch)
            info(f"[*] Instance {instance_name} has ip {instance_ip}")
            ctx.run(
                f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {ssh_key_path}\" --chmod=F644 --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' {source} ubuntu@{instance_ip}:/home/ubuntu/datadog-agent"
            )
            ctx.run(
                f"ssh -i {ssh_key_path} -o StrictHostKeyChecking=no ubuntu@{instance_ip} \"cd /home/ubuntu/datadog-agent && {vm_copy}\""
            )
        else:
            raise Exit(f"Unsupported arch {arch}")


@task
def sync(ctx, stack=None, vms="", ssh_key=""):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    target_vms = build_target_set(stack, vms, ssh_key)

    info("[*] VMs to sync")
    for _, vm, ip in target_vms:
        info(f"    Syncing VM {vm} with ip {ip}")

    if ask("Do you want to sync? (y/n)") != "y":
        warn("[-] Sync aborted !")
        return

    info("[*] Beginning sync...")

    if ssh_key != "":
        ssh_key_path = f"~/.ssh/{ssh_key}.pem"

    sync_source(ctx, target_vms, "./", "/root/datadog-agent")

def compiler_built(ctx):
    res = ctx.run("docker images kmt:compile | grep -v REPOSITORY | grep kmt", warn=True)
    return res.ok

@task
def build_compiler(ctx):
    ctx.run("docker build -f ../datadog-agent-buildimages/system-probe_x64/Dockerfile -t kmt:compile .")

@task
def start_compiler(ctx):
    if not compiler_built(ctx):
        build_compiler(ctx)

    ctx.run("docker run -d --restart always --name kmt-compiler --mount type=bind,source=./,target=/root/datadog-agent kmt:compile sleep \"infinity\"")

def compiler_running(ctx):
    res = ctx.run("docker ps -aqf \"name=kmt-compiler\"")
    if res.ok:
        return res.stdout.rstrip() != ""
    return False

def download_gotestsum(ctx):
    if platform.machine() == "x86_64":
        url = "https://github.com/gotestyourself/gotestsum/releases/download/v1.10.0/gotestsum_1.10.0_linux_amd64.tar.gz"
    else:
        url = "https://github.com/gotestyourself/gotestsum/releases/download/v1.10.0/gotestsum_1.10.0_linux_arm64.tar.gz"
        
    fgotestsum = "./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/gotestsum"
    if os.path.isfile(fgotestsum):
        return 

    ctx.run(f"wget {url} -O /tmp/gotestsum.tar.gz")
    ctx.run("tar xzvf /tmp/gotestsum.tar.gz -C /tmp")
    ctx.run(f"cp /tmp/gotestsum {fgotestsum}")


@task
def prepare(ctx, stack=None, arch=None, vms="", ssh_key=""):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not arch:
        arch = platform.machine()

    if not compiler_running(ctx):
        start_compiler(ctx)

    download_gotestsum(ctx)

    ctx.run(f"{DOCKER_EXEC} \"cd /root/datadog-agent && git config --global --add safe.directory /root/datadog-agent && inv -e system-probe.kitchen-prepare --ci\"")
    if not os.path.isfile(f"kmt-deps/{stack}/dependencies-{arch}.tar.gz"):
        ctx.run(f"{DOCKER_EXEC} \"cd /root/datadog-agent && ./test/new-e2e/system-probe/test/setup-microvm-deps.sh {stack} {os.getuid()}\"")

    target_vms = build_target_set(stack, vms, ssh_key)
    sync_source(ctx, target_vms, "./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg", "/opt/system-probe-tests")

#    target_vms = build_target_set(stack, vms, ssh_key)

#@task
#def test(ctx, stack=None, packages=None, run=None):
#    stack = check_and_get_stack(stack)
#    if not stacks.stack_exists(stack):
#        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")
#
#    prepare(ctx, stack, packages)


@task
def clean(ctx, stack=None):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    # delete all leftover artifacts from docker build
    ctx.run(f"{DOCKER_EXEC} \"find /root/datadog-agent -type f -user root -exec rm -f {{}} \\; \"")
    ctx.run(f"{DOCKER_EXEC} \"find /root/datadog-agent -type d -user root -exec rm -rf {{}} \\; \"")

    ctx.run(f"rm -rf kmt-deps/{stack}")

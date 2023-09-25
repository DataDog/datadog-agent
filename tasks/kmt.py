import os
import platform
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
    KMT_SHARED_DIR,
    KMT_STACKS_DIR,
    check_and_get_stack,
    init_kernel_matrix_testing_system,
)
from .kernel_matrix_testing.tool import Exit, ask, info, warn
from .system_probe import EMBEDDED_SHARE_DIR

try:
    from tabulate import tabulate
except ImportError:
    tabulate = None

X86_AMI_ID_SANDBOX = "ami-0d1f81cfdbd5b0188"
ARM_AMI_ID_SANDBOX = "ami-02cb18e91afb3777c"
GOVERSION = 1.20


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
    clean(ctx, stack)
    stacks.destroy_stack(ctx, stack, force, ssh_key)


@task
def pause_stack(_, stack=None):
    stacks.pause_stack(stack)


@task
def resume_stack(_, stack=None):
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
def update_resources(ctx, no_backup=False):
    warn("Updating resource dependencies will delete all running stacks.")
    if ask("are you sure you want to continue? (Y/n)") != "Y":
        raise Exit("[-] Update aborted")

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=os.path.basename(stack), force=True)

    update_kernel_packages(ctx, KMT_PACKAGES_DIR, KMT_KHEADERS_DIR, KMT_BACKUP_DIR, no_backup)
    update_rootfs(ctx, KMT_ROOTFS_DIR, KMT_BACKUP_DIR, no_backup)


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


def get_instance_ip(stack, arch):
    with open(f"{KMT_STACKS_DIR}/{stack}/stack.outputs", 'r') as f:
        entries = f.readlines()
        for entry in entries:
            if f"{arch}-instance-ip" in entry.split(' ')[0]:
                return entry.split()[0], entry.split()[1].strip('\n')


def ssh_key_to_path(ssh_key):
    ssh_key_path = ""
    if ssh_key != "":
        ssh_key.rstrip(".pem")
        ssh_key_path = stacks.find_ssh_key(ssh_key)

    return ssh_key_path


def sync_source(ctx, vm_ls, source, target, ssh_key):
    ssh_key_path = ssh_key_to_path(ssh_key)

    for arch, _, ip in vm_ls:
        vm_copy = f"rsync -e \\\"ssh -o StrictHostKeyChecking=no -i {KMT_DIR}/ddvm_rsa\\\" --chmod=F644 --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' ./ root@{ip}:{target}"
        if arch == "local":
            ctx.run(
                f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {KMT_DIR}/ddvm_rsa\" -p --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' {source} root@{ip}:{target}"
            )
        elif arch == "x86_64" or arch == "arm64":
            instance_name, instance_ip = get_instance_ip(stack, arch)
            info(f"[*] Instance {instance_name} has ip {instance_ip}")
            ctx.run(
                f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {ssh_key_path}\" -p --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' {source} ubuntu@{instance_ip}:/home/ubuntu/datadog-agent"
            )
            ctx.run(
                f"ssh -i {ssh_key_path} -o StrictHostKeyChecking=no ubuntu@{instance_ip} \"cd /home/ubuntu/datadog-agent && {vm_copy}\""
            )
        else:
            raise Exit(f"Unsupported arch {arch}")


@task
def sync(ctx, vms, stack=None, ssh_key=""):
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

    sync_source(ctx, target_vms, "./", "/datadog-agent", ssh_key)


def compiler_built(ctx):
    res = ctx.run("docker images kmt:compile | grep -v REPOSITORY | grep kmt", warn=True)
    return res.ok


@task
def build_compiler(ctx):
    ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")", warn=True, hide=True)
    ctx.run("docker image rm kmt:compile", warn=True, hide=True)
    ctx.run("docker build -f ../datadog-agent-buildimages/system-probe_x64/Dockerfile -t kmt:compile .")


def docker_exec(ctx, cmd, user="root"):
    ctx.run(f"docker exec -u {user} -i kmt-compiler bash -c \"{cmd}\"")


@task
def start_compiler(ctx):
    if not compiler_built(ctx):
        build_compiler(ctx)

    if not compiler_running(ctx):
        ctx.run(
            "docker run -d --restart always --name kmt-compiler --mount type=bind,source=./,target=/datadog-agent kmt:compile sleep \"infinity\""
        )

    uid = ctx.run("getent passwd $USER | cut -d ':' -f 3").stdout.rstrip()
    gid = ctx.run("getent group $USER | cut -d ':' -f 3").stdout.rstrip()
    docker_exec(ctx, f"getent group {gid} || groupadd -f -g {gid} compiler")
    docker_exec(ctx, f"getent passwd {uid} || useradd -m -u {uid} -g {gid} compiler")
    docker_exec(ctx, f"chown {uid}:{gid} /datadog-agent && chown -R {uid}:{gid} /datadog-agent")
    docker_exec(ctx, "apt install sudo")
    docker_exec(ctx, "usermod -aG sudo compiler && echo 'compiler ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers")
    docker_exec(ctx, f"install -d -m 0777 -o {uid} -g {uid} /go")


def compiler_running(ctx):
    res = ctx.run("docker ps -aqf \"name=kmt-compiler\"")
    if res.ok:
        return res.stdout.rstrip() != ""
    return False


TOOLS_PATH = '/datadog-agent/internal/tools'
GOTESTSUM = "gotest.tools/gotestsum"


def download_gotestsum(ctx):
    fgotestsum = "./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/gotestsum"
    if os.path.isfile(fgotestsum):
        return

    if not os.path.exists("kmt-deps/tools"):
        ctx.run("mkdir -p kmt-deps/tools")

    docker_exec(
        ctx,
        f"cd {TOOLS_PATH} && go install {GOTESTSUM} && cp /go/bin/gotestsum /datadog-agent/kmt-deps/tools/",
        user="compiler",
    )

    ctx.run(f"cp kmt-deps/tools/gotestsum {fgotestsum}")


def run_cmd_vms(ctx, stack, cmd, vms, ssh_key, allow_fail=False):
    ssh_key_path = ssh_key_to_path(ssh_key)

    for arch, _, ip in vms:
        if arch != "local" and ssh_key == "":
            raise Exit("`ssh_key` is required when syncing VMs on remote instance")
        if arch == "local":
            run_cmd_local(ctx, cmd, ip, allow_fail)
        else:
            run_cmd_remote(ctx, stack, cmd, arch, ip, ssh_key_path)


def run_cmd_local(ctx, cmd, ip, allow_fail):
    ctx.run(
        f"ssh -o StrictHostKeyChecking=no -i /home/kernel-version-testing/ddvm_rsa root@{ip} '{cmd}'", warn=allow_fail
    )


def run_cmd_remote(ctx, stack, cmd, arch, ip, ssh_key):
    _, remote_ip = get_instance_ip(stack, arch)
    ctx.run(
        f"ssh -o StrictHostKeyChecking=no -i {ssh_key} ubuntu@{remote_ip} \"ssh -o StrictHostKeyChecking=no -i /home/kernel-version-testing/ddvm_rsa root@{ip} '{cmd}'\""
    )


def copy_dependencies(ctx, stack, vms, ssh_key):
    local_cp = False
    ssh_key_path = ssh_key_to_path(ssh_key)

    for arch, _, _ in vms:
        if arch == "local":
            local_cp = True
            continue
        if ssh_key == "":
            raise Exit("`ssh_key` is required when syncing VMs on remote instance")

        _, remote_ip = get_instance_ip(stack, arch)
        ctx.run(
            f"scp -o StrictHostKeyChecking=no -i {ssh_key_path} kmt-deps/{stack}/dependencies-{arch}.tar.gz ubuntu@{remote_ip}:/opt/kernel-version-testing/"
        )

    if local_cp:
        ctx.run(f"cp kmt-deps/{stack}/dependencies-{platform.machine()}.tar.gz /opt/kernel-version-testing")


@task
def prepare(ctx, vms, stack=None, arch=None, ssh_key="", rebuild_deps=False, packages=""):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if vms == "":
        raise Exit("No vms specified to sync with")

    if not arch:
        arch = platform.machine()

    if not compiler_running(ctx):
        start_compiler(ctx)

    download_gotestsum(ctx)

    target_vms = build_target_set(stack, vms, ssh_key)

    docker_exec(
        ctx,
        f"cd /datadog-agent && git config --global --add safe.directory /datadog-agent && inv -e system-probe.kitchen-prepare --ci --packages={packages}",
        user="compiler",
    )
    if rebuild_deps or not os.path.isfile(f"kmt-deps/{stack}/dependencies-{arch}.tar.gz"):
        docker_exec(
            ctx,
            f"cd /datadog-agent && ./test/new-e2e/system-probe/test/setup-microvm-deps.sh {stack} {os.getuid()} {os.getgid()} {platform.machine()}",
            user="compiler",
        )
        copy_dependencies(ctx, stack, target_vms, ssh_key)
        run_cmd_vms(
            ctx, stack, f"/root/fetch_dependencies.sh {platform.machine()}", target_vms, ssh_key, allow_fail=True
        )

    sync_source(
        ctx,
        target_vms,
        "./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg",
        "/opt/system-probe-tests",
        ssh_key,
    )


@task
def test(ctx, vms, stack=None, packages="", run=None, retry=2, rebuild_deps=False, ssh_key="", go_version=GOVERSION):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    prepare(ctx, stack=stack, vms=vms, ssh_key=ssh_key, rebuild_deps=rebuild_deps, packages=packages)

    target_vms = build_target_set(stack, vms, ssh_key)
    args = [
        f"-include-packages {packages}" if packages else "",
        f"-run-tests {run}" if run else "",
    ]
    run_cmd_vms(
        ctx,
        stack,
        f"bash /micro-vm-init.sh {go_version} {retry} {platform.machine()} {' '.join(args)}",
        target_vms,
        "",
        allow_fail=True,
    )


@task
def build(ctx, vms, stack=None, ssh_key="", rebuild_deps=False):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not os.path.exists(f"kmt-deps/{stack}"):
        ctx.run(f"mkdir -p kmt-deps/{stack}")

    target_vms = build_target_set(stack, vms, ssh_key)
    if rebuild_deps or not os.path.isfile(f"kmt-deps/{stack}/dependencies-{platform.machine()}.tar.gz"):
        docker_exec(
            ctx,
            f"cd /datadog-agent && ./test/new-e2e/system-probe/test/setup-microvm-deps.sh {stack} {os.getuid()} {os.getgid()} {platform.machine()}",
            user="compiler",
        )
        copy_dependencies(ctx, stack, target_vms, ssh_key)
        run_cmd_vms(
            ctx, stack, f"/root/fetch_dependencies.sh {platform.machine()}", target_vms, ssh_key, allow_fail=True
        )

    docker_exec(
        ctx, "cd /datadog-agent && git config --global --add safe.directory /datadog-agent && inv -e system-probe.build"
    )
    docker_exec(ctx, f"tar cf /datadog-agent/kmt-deps/{stack}/shared.tar {EMBEDDED_SHARE_DIR}")
    sync_source(ctx, target_vms, "./bin/system-probe", "/root", ssh_key)
    sync_source(ctx, target_vms, f"kmt-deps/{stack}/shared.tar", "/", ssh_key)
    run_cmd_vms(ctx, stack, "tar xf /shared.tar -C /", target_vms, ssh_key)


@task
def clean(ctx, stack=None, container=False, image=False):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    ctx.run("rm -rf ./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg")
    ctx.run(f"rm -rf kmt-deps/{stack}", warn=True)
    ctx.run(f"rm {KMT_SHARED_DIR}/*.tar.gz", warn=True)

    if container:
        ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")")
    if image:
        ctx.run("docker image rm kmt:compile")

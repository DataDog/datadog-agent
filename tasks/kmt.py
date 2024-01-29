import json
import os
import platform
import re
import tempfile
from glob import glob
from pathlib import Path

from invoke import task

from tasks.kernel_matrix_testing import stacks, vmconfig
from tasks.kernel_matrix_testing.compiler import build_compiler as build_cc
from tasks.kernel_matrix_testing.compiler import compiler_running, docker_exec
from tasks.kernel_matrix_testing.compiler import start_compiler as start_cc
from tasks.kernel_matrix_testing.download import arch_mapping, update_rootfs
from tasks.kernel_matrix_testing.infra import build_infrastructure
from tasks.kernel_matrix_testing.init_kmt import check_and_get_stack, init_kernel_matrix_testing_system
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import Exit, ask, info, warn
from tasks.libs.common.gitlab import Gitlab, get_gitlab_token
from tasks.system_probe import EMBEDDED_SHARE_DIR

try:
    from tabulate import tabulate
except ImportError:
    tabulate = None

X86_AMI_ID_SANDBOX = "ami-0d1f81cfdbd5b0188"
ARM_AMI_ID_SANDBOX = "ami-02cb18e91afb3777c"
DEFAULT_VCPU = "4"
DEFAULT_MEMORY = "8192"


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
        "from-ci-pipeline": "Generate a vmconfig.json file with the VMs that failed jobs in pipeline with the given ID.",
        "use-local-if-possible": "(Only when --from-ci-pipeline is used) If the VM is for the same architecture as the host, use the local VM instead of the remote one.",
    }
)
def gen_config(
    ctx,
    stack=None,
    vms="",
    sets="",
    init_stack=False,
    vcpu=None,
    memory=None,
    new=False,
    ci=False,
    arch="",
    output_file="vmconfig.json",
    from_ci_pipeline=None,
    use_local_if_possible=False,
):
    """
    Generate a vmconfig.json file with the given VMs.
    """
    if from_ci_pipeline is not None:
        return gen_config_from_ci_pipeline(
            ctx,
            stack=stack,
            pipeline=from_ci_pipeline,
            init_stack=init_stack,
            vcpu=vcpu,
            memory=memory,
            new=new,
            ci=ci,
            arch=arch,
            output_file=output_file,
            use_local_if_possible=use_local_if_possible,
        )
    else:
        vcpu = DEFAULT_VCPU if vcpu is None else vcpu
        memory = DEFAULT_MEMORY if memory is None else memory
        vmconfig.gen_config(ctx, stack, vms, sets, init_stack, vcpu, memory, new, ci, arch, output_file)


def gen_config_from_ci_pipeline(
    ctx,
    stack=None,
    pipeline=None,
    init_stack=False,
    vcpu=None,
    memory=None,
    new=False,
    ci=False,
    use_local_if_possible=False,
    arch="",
    output_file="vmconfig.json",
):
    """
    Generate a vmconfig.json file with the VMs that failed jobs in the given pipeline.
    """
    gitlab = Gitlab("DataDog/datadog-agent", get_gitlab_token())
    vms = set()
    local_arch = full_arch("local")

    if pipeline is None:
        raise Exit("Pipeline ID must be provided")

    info(f"[+] retrieving all CI jobs for pipeline {pipeline}")
    for job in gitlab.all_jobs(pipeline):
        name = job.get("name", "")

        if (
            (vcpu is None or memory is None)
            and name.startswith("kernel_matrix_testing_setup_env")
            and job["status"] == "success"
        ):
            arch = "x86_64" if "x64" in name else "arm64"
            vmconfig_name = f"vmconfig-{pipeline}-{arch}.json"
            info(f"[+] retrieving {vmconfig_name} for {arch} from job {name}")

            try:
                req = gitlab.artifact(job["id"], vmconfig_name)
                req.raise_for_status()
            except Exception as e:
                warn(f"[-] failed to retrieve artifact {vmconfig_name}: {e}")
                continue

            data = json.loads(req.content)

            for vmset in data.get("vmsets", []):
                memory_list = vmset.get("memory", [])
                if memory is None and len(memory_list) > 0:
                    memory = str(memory_list[0])
                    info(f"[+] setting memory to {memory}")

                vcpu_list = vmset.get("vcpu", [])
                if vcpu is None and len(vcpu_list) > 0:
                    vcpu = str(vcpu_list[0])
                    info(f"[+] setting vcpu to {vcpu}")
        elif name.startswith("kernel_matrix_testing_run") and job["status"] == "failed":
            arch = "x86" if "x64" in name else "arm64"
            match = re.search(r"\[(.*)\]", name)

            if match is None:
                warn(f"Cannot extract variables from job {name}, skipping")
                continue

            vars = match.group(1).split(",")
            distro = vars[0]

            if use_local_if_possible and arch == local_arch:
                arch = "local"

            vms.add(f"{arch}-{distro}-distro")

    info(f"[+] generating vmconfig.json file for VMs {vms}")
    vcpu = DEFAULT_VCPU if vcpu is None else vcpu
    memory = DEFAULT_MEMORY if memory is None else memory
    return vmconfig.gen_config(ctx, stack, ",".join(vms), "", init_stack, vcpu, memory, new, ci, arch, output_file)


@task
def launch_stack(ctx, stack=None, ssh_key="", x86_ami=X86_AMI_ID_SANDBOX, arm_ami=ARM_AMI_ID_SANDBOX):
    stacks.launch_stack(ctx, stack, ssh_key, x86_ami, arm_ami)


@task
def destroy_stack(ctx, stack=None, pulumi=False, ssh_key=""):
    clean(ctx, stack)
    stacks.destroy_stack(ctx, stack, pulumi, ssh_key)


@task
def pause_stack(_, stack=None):
    stacks.pause_stack(stack)


@task
def resume_stack(_, stack=None):
    stacks.resume_stack(stack)


@task
def stack(_, stack=None):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    infrastructure = build_infrastructure(stack)
    for instance in infrastructure.values():
        print(instance)
        for vm in instance.microvms:
            print(f"  {vm}")


@task
def ls(_, distro=False, custom=False):
    print(tabulate(vmconfig.get_image_list(distro, custom), headers='firstrow', tablefmt='fancy_grid'))


@task
def init(ctx, lite=False):
    init_kernel_matrix_testing_system(ctx, lite)


@task
def update_resources(ctx):
    kmt_os = get_kmt_os()

    warn("Updating resource dependencies will delete all running stacks.")
    if ask("are you sure you want to continue? (y/n)").lower() != "y":
        raise Exit("[-] Update aborted")

    for stack in glob(f"{kmt_os.stacks_dir}/*"):
        destroy_stack(ctx, stack=os.path.basename(stack))

    update_rootfs(ctx, kmt_os.rootfs_dir)


@task
def build_compiler(ctx):
    build_cc(ctx)


@task
def start_compiler(ctx):
    start_cc(ctx)


def filter_target_domains(vms, infra):
    vmsets = vmconfig.build_vmsets(vmconfig.build_normalized_vm_def_set(vms), [])
    domains = list()
    for vmset in vmsets:
        for vm in vmset.vms:
            for domain in infra[vmset.arch].microvms:
                if domain.tag == vm.version:
                    domains.append(domain)

    return domains


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
    )

    ctx.run(f"cp kmt-deps/tools/gotestsum {fgotestsum}")


def full_arch(arch):
    if arch == "local":
        return arch_mapping[platform.machine()]
    return arch


@task
def prepare(ctx, vms, stack=None, arch=None, ssh_key=None, rebuild_deps=False, packages="", verbose=True):
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

    infra = build_infrastructure(stack, ssh_key)
    domains = filter_target_domains(vms, infra)

    constrain_pkgs = ""
    if not rebuild_deps:
        constrain_pkgs = f"--packages={packages}"

    docker_exec(
        ctx,
        f"git config --global --add safe.directory /datadog-agent && inv -e system-probe.kitchen-prepare --ci {constrain_pkgs}",
        run_dir="/datadog-agent",
    )
    if rebuild_deps:
        docker_exec(
            ctx,
            f"./test/new-e2e/system-probe/test/setup-microvm-deps.sh {stack} {os.getuid()} {os.getgid()} {platform.machine()}",
            run_dir="/datadog-agent",
        )
        target_instances = list()
        for d in domains:
            target_instances.append(d.instance)

        for instance in target_instances:
            instance.copy_to_all_vms(ctx, f"kmt-deps/{stack}/dependencies-{full_arch(instance.arch)}.tar.gz")

        for d in domains:
            d.run_cmd(ctx, f"/root/fetch_dependencies.sh {platform.machine()}", allow_fail=True, verbose=verbose)
            d.copy(
                ctx,
                "./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg",
                "/opt/system-probe-tests",
            )


def build_run_config(run, packages):
    c = dict()

    if len(packages) == 0:
        return {"*": {"exclude": False}}

    for p in packages:
        if p[:2] == "./":
            p = p[2:]
        if run is not None:
            c[p] = {"run-only": [run]}
        else:
            c[p] = {"exclude": False}

    return c


@task
def test(ctx, vms, stack=None, packages="", run=None, retry=2, rebuild_deps=False, ssh_key=None, verbose=True):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    prepare(ctx, stack=stack, vms=vms, ssh_key=ssh_key, rebuild_deps=rebuild_deps, packages=packages)

    infra = build_infrastructure(stack, ssh_key)
    domains = filter_target_domains(vms, infra)
    if run is not None and packages is None:
        raise Exit("Package must be provided when specifying test")
    pkgs = packages.split(",")
    if run is not None and len(pkgs) > 1:
        raise Exit("Only a single package can be specified when running specific tests")

    run_config = build_run_config(run, pkgs)
    with tempfile.NamedTemporaryFile(mode='w') as tmp:
        json.dump(run_config, tmp)
        tmp.flush()

        for d in domains:
            d.copy(ctx, f"{tmp.name}", "/tmp")
            d.run_cmd(ctx, f"bash /micro-vm-init.sh {retry} {tmp.name}", verbose=verbose)


@task
def build(ctx, vms, stack=None, ssh_key=None, rebuild_deps=False, verbose=True):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not os.path.exists(f"kmt-deps/{stack}"):
        ctx.run(f"mkdir -p kmt-deps/{stack}")

    infra = build_infrastructure(stack, ssh_key)
    domains = filter_target_domains(vms, infra)
    if rebuild_deps:
        docker_exec(
            ctx,
            f"./test/new-e2e/system-probe/test/setup-microvm-deps.sh {stack} {os.getuid()} {os.getgid()} {platform.machine()}",
            run_dir="/datadog-agent",
        )

        target_instances = list()
        for d in domains:
            target_instances.append(d.instance)

        for instance in target_instances:
            instance.copy_to_all_vms(ctx, f"kmt-deps/{stack}/dependencies-{full_arch(instance.arch)}.tar.gz")

        for d in domains:
            d.run_cmd(ctx, f"/root/fetch_dependencies.sh {arch_mapping[platform.machine()]}")

    docker_exec(
        ctx, "cd /datadog-agent && git config --global --add safe.directory /datadog-agent && inv -e system-probe.build"
    )
    docker_exec(ctx, f"tar cf /datadog-agent/kmt-deps/{stack}/shared.tar {EMBEDDED_SHARE_DIR}")
    for d in domains:
        d.copy(ctx, "./bin/system-probe", "/root")
        d.copy(ctx, f"kmt-deps/{stack}/shared.tar", "/")
        d.run_cmd(ctx, "tar xf /shared.tar -C /", verbose=verbose)
        info(f"[+] system-probe built for {d.name} @ /root")


@task
def clean(ctx, stack=None, container=False, image=False):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    docker_exec(ctx, "inv -e system-probe.clean", run_dir="/datadog-agent")
    ctx.run("rm -rf ./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg")
    ctx.run(f"rm -rf kmt-deps/{stack}", warn=True)
    ctx.run(f"rm {get_kmt_os().shared_dir}/*.tar.gz", warn=True)

    if container:
        ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")")
    if image:
        ctx.run("docker image rm kmt:compile")


@task(
    help={
        "stacks": "Comma separated list of stacks to generate ssh config for. 'all' to generate for all stacks.",
        "ddvm_rsa": "Path to the ddvm_rsa file to use for connecting to the VMs. Defaults to the path in the ami-builder repo",
    }
)
def ssh_config(_, stacks=None, ddvm_rsa="~/dd/ami-builder/scripts/kernel-version-testing/files/ddvm_rsa"):
    """
    Print the SSH config for the given stacks.

    Recommended usage: inv kmt.ssh-config --stacks=all > ~/.ssh/config-kmt.
    Then add the following to your ~/.ssh/config:
            Include ~/.ssh/config-kmt

    This makes it easy to use the SSH config for all stacks whenever you change anything,
    without worrying about overriding existing configs.
    """
    stacks_dir = Path(get_kmt_os().stacks_dir)
    stacks_to_print = None

    if stacks is not None and stacks != 'all':
        stacks_to_print = set(stacks.split(','))

    for stack in stacks_dir.iterdir():
        if not stack.is_dir():
            continue

        output = stack / "stack.output"
        if not output.exists():
            continue  # Invalid/removed stack, ignore it

        stack_name = stack.name.replace('-ddvm', '')
        if (
            stacks_to_print is not None
            and 'all' not in stacks_to_print
            and stack_name not in stacks_to_print
            and stack.name not in stacks_to_print
        ):
            continue

        for _, instance in build_infrastructure(stack, remote_ssh_key="").items():
            print(f"Host kmt-{stack_name}-{instance.arch}")
            print(f"    HostName {instance.ip}")
            print("    User ubuntu")
            print("")
            for domain in instance.microvms:
                print(f"Host kmt-{stack_name}-{instance.arch}-{domain.tag}")
                print(f"    HostName {domain.ip}")
                print(f"    ProxyJump kmt-{stack_name}-{instance.arch}")
                print(f"    IdentityFile {ddvm_rsa}")
                print("    User root")
                # Disable host key checking, the IPs of the QEMU machines are reused and we don't want constant
                # warnings about changed host keys. We need the combination of both options, if we just set
                # StrictHostKeyChecking to no, it will still check the known hosts file and disable some options
                # and print out scary warnings if the key doesn't match.
                print("    UserKnownHostsFile /dev/null")
                print("    StrictHostKeyChecking accept-new")
                print("")

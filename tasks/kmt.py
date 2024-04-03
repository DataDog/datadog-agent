from __future__ import annotations

import json
import os
import platform
import re
import shutil
import tempfile
from collections import defaultdict
from glob import glob
from pathlib import Path
from typing import TYPE_CHECKING, Any, Dict, Iterable, List, Optional, Set, Tuple, cast

from invoke.context import Context
from invoke.tasks import task

from tasks.kernel_matrix_testing import stacks, vmconfig
from tasks.kernel_matrix_testing.compiler import CONTAINER_AGENT_PATH, all_compilers, get_compiler
from tasks.kernel_matrix_testing.download import arch_mapping, update_rootfs
from tasks.kernel_matrix_testing.infra import SSH_OPTIONS, HostInstance, LibvirtDomain, build_infrastructure
from tasks.kernel_matrix_testing.init_kmt import init_kernel_matrix_testing_system
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.stacks import check_and_get_stack, ec2_instance_ids
from tasks.kernel_matrix_testing.tool import Exit, ask, get_binary_target_arch, info, warn
from tasks.libs.common.gitlab import Gitlab, get_gitlab_token
from tasks.system_probe import EMBEDDED_SHARE_DIR

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import (  # noqa: F401
        Arch,
        ArchOrLocal,
        Component,
        DependenciesLayout,
        PathOrStr,
    )

try:
    from tabulate import tabulate
except ImportError:
    tabulate = None


try:
    from termcolor import colored
except ImportError:

    def colored(text: str, color: Optional[str]) -> str:  # noqa: U100
        return text


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
        "vmconfig_template": "Template to use for the generated vmconfig.json file. Defaults to 'system-probe'. A file named 'vmconfig-<vmconfig_template>.json' must exist in 'tasks/new-e2e/system-probe/config/'",
    }
)
def gen_config(
    ctx: Context,
    stack: Optional[str] = None,
    vms: str = "",
    sets: str = "",
    init_stack=False,
    vcpu: Optional[str] = None,
    memory: Optional[str] = None,
    new=False,
    ci=False,
    arch: str = "",
    output_file: str = "vmconfig.json",
    from_ci_pipeline: Optional[str] = None,
    use_local_if_possible=False,
    vmconfig_template: Component = "system-probe",
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
            vmconfig_template=vmconfig_template,
        )
    else:
        vcpu = DEFAULT_VCPU if vcpu is None else vcpu
        memory = DEFAULT_MEMORY if memory is None else memory
        vmconfig.gen_config(
            ctx, stack, vms, sets, init_stack, vcpu, memory, new, ci, arch, output_file, vmconfig_template
        )


def gen_config_from_ci_pipeline(
    ctx: Context,
    stack: Optional[str] = None,
    pipeline: Optional[str] = None,
    init_stack=False,
    vcpu: Optional[str] = None,
    memory: Optional[str] = None,
    new=False,
    ci=False,
    use_local_if_possible=False,
    arch: str = "",
    output_file="vmconfig.json",
    vmconfig_template="system-probe",
    test_job_prefix="sysprobe",
):
    """
    Generate a vmconfig.json file with the VMs that failed jobs in the given pipeline.
    """
    gitlab = Gitlab("DataDog/datadog-agent", str(get_gitlab_token()))
    vms = set()
    local_arch = full_arch("local")

    if pipeline is None:
        raise Exit("Pipeline ID must be provided")

    info(f"[+] retrieving all CI jobs for pipeline {pipeline}")
    for job in gitlab.all_jobs(pipeline):
        name = job.get("name", "")

        if (vcpu is None or memory is None) and name.startswith("kmt_setup_env") and job["status"] == "success":
            arch = "x86_64" if "x64" in name else "arm64"
            vmconfig_name = f"vmconfig-{pipeline}-{arch}.json"
            info(f"[+] retrieving {vmconfig_name} for {arch} from job {name}")

            try:
                req = gitlab.artifact(job["id"], vmconfig_name)
                if req is None:
                    raise Exit(f"[-] failed to retrieve artifact {vmconfig_name}")
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
        elif name.startswith(f"kmt_run_{test_job_prefix}") and job["status"] == "failed":
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
    return vmconfig.gen_config(
        ctx, stack, ",".join(vms), "", init_stack, vcpu, memory, new, ci, arch, output_file, vmconfig_template
    )


@task
def launch_stack(
    ctx: Context,
    stack: Optional[str] = None,
    ssh_key: str = "",
    x86_ami: str = X86_AMI_ID_SANDBOX,
    arm_ami: str = ARM_AMI_ID_SANDBOX,
    provision_microvms: bool = True,
):
    stacks.launch_stack(ctx, stack, ssh_key, x86_ami, arm_ami, provision_microvms)


@task
def destroy_stack(ctx: Context, stack: Optional[str] = None, pulumi=False, ssh_key=""):
    clean(ctx, stack)
    stacks.destroy_stack(ctx, stack, pulumi, ssh_key)


@task
def pause_stack(_, stack: Optional[str] = None):
    stacks.pause_stack(stack)


@task
def resume_stack(_, stack: Optional[str] = None):
    stacks.resume_stack(stack)


@task
def ls(_, distro=False, custom=False):
    if tabulate is None:
        raise Exit("tabulate module is not installed, please install it to continue")

    print(tabulate(vmconfig.get_image_list(distro, custom), headers='firstrow', tablefmt='fancy_grid'))


@task
def init(ctx: Context, lite=False):
    init_kernel_matrix_testing_system(ctx, lite)


@task
def update_resources(ctx: Context, vmconfig_template="system-probe"):
    kmt_os = get_kmt_os()

    warn("Updating resource dependencies will delete all running stacks.")
    if ask("are you sure you want to continue? (y/n)").lower() != "y":
        raise Exit("[-] Update aborted")

    for stack in glob(f"{kmt_os.stacks_dir}/*"):
        destroy_stack(ctx, stack=os.path.basename(stack))

    update_rootfs(ctx, kmt_os.rootfs_dir, vmconfig_template)


@task
def build_compiler(ctx: Context):
    for cc in all_compilers(ctx):
        cc.build()


@task
def start_compiler(ctx: Context):
    for cc in all_compilers(ctx):
        cc.start()


def filter_target_domains(vms: str, infra: Dict[ArchOrLocal, HostInstance], arch: Optional[ArchOrLocal] = None):
    vmsets = vmconfig.build_vmsets(vmconfig.build_normalized_vm_def_set(vms), [])
    domains: List[LibvirtDomain] = list()
    for vmset in vmsets:
        if arch is not None and full_arch(vmset.arch) != full_arch(arch):
            warn(f"Ignoring VM {vmset} as it is not of the expected architecture {arch}")
            continue
        for vm in vmset.vms:
            for domain in infra[vmset.arch].microvms:
                if domain.tag == vm.version:
                    domains.append(domain)

    return domains


def get_archs_in_domains(domains: Iterable[LibvirtDomain]) -> Set[Arch]:
    archs: Set[Arch] = set()
    for d in domains:
        archs.add(full_arch(d.instance.arch))
    return archs


TOOLS_PATH = f"{CONTAINER_AGENT_PATH}/internal/tools"
GOTESTSUM = "gotest.tools/gotestsum"


def download_gotestsum(ctx: Context, arch: Arch):
    fgotestsum = "./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/gotestsum"

    if os.path.isfile(fgotestsum):
        file_arch = get_binary_target_arch(ctx, fgotestsum)
        if file_arch == arch:
            return

    paths = KMTPaths(None, arch)
    paths.tools.mkdir(parents=True, exist_ok=True)

    cc = get_compiler(ctx, arch)
    target_path = CONTAINER_AGENT_PATH / paths.tools.relative_to(paths.repo_root)
    cc.exec(
        f"cd {TOOLS_PATH} && go install {GOTESTSUM} && cp /go/bin/gotestsum {target_path}",
    )

    ctx.run(f"cp {paths.tools}/gotestsum {fgotestsum}")


def full_arch(arch: ArchOrLocal) -> Arch:
    if arch == "local":
        return arch_mapping[platform.machine()]
    return arch


class KMTPaths:
    def __init__(self, stack: Optional[str], arch: Arch):
        self.stack = stack
        self.arch = arch

    @property
    def repo_root(self):
        # this file is tasks/kmt.py, so two parents is the agent folder
        return Path(__file__).parent.parent

    @property
    def root(self):
        return self.repo_root / "kmt-deps"

    @property
    def arch_dir(self):
        return self.stack_dir / self.arch

    @property
    def stack_dir(self):
        if self.stack is None:
            raise Exit("no stack name provided, cannot use stack-specific paths")

        return self.root / self.stack

    @property
    def dependencies(self):
        return self.arch_dir / "dependencies"

    @property
    def dependencies_archive(self):
        return self.arch_dir / f"dependencies-{self.arch}.tar.gz"

    @property
    def tests_archive(self):
        return self.arch_dir / f"tests-{self.arch}.tar.gz"

    @property
    def tools(self):
        return self.root / self.arch / "tools"


def build_tests_package(ctx: Context, source_dir: str, stack: str, arch: Arch, ci: bool, verbose=True):
    paths = KMTPaths(stack, arch)
    tests_archive = paths.tests_archive
    if not ci:
        system_probe_tests = tests_archive.parent / "opt/system-probe-tests"
        test_pkgs = os.path.join(
            source_dir, "test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg"
        )
        ctx.run(f"rm -rf {system_probe_tests} && mkdir -p {system_probe_tests}", hide=(not verbose))
        ctx.run(f"cp -R {test_pkgs} {system_probe_tests}", hide=(not verbose))
        with ctx.cd(tests_archive.parent):
            ctx.run(f"tar czvf {tests_archive.name} opt", hide=(not verbose))


@task
def build_dependencies(
    ctx: Context,
    arch: Arch,
    layout_file: PathOrStr,
    source_dir: PathOrStr,
    ci=False,
    stack: Optional[str] = None,
    verbose=True,
) -> None:
    if stack is None:
        raise Exit("no stack name provided")
    info(f"[+] Building dependencies for {arch} in stack {stack}")
    paths = KMTPaths(stack, arch)
    source_dir = Path(source_dir)
    if not ci:
        # in the CI we can rely on gotestsum being present
        download_gotestsum(ctx, arch)

    if paths.dependencies.exists():
        shutil.rmtree(paths.dependencies)

    ctx.run(f"mkdir -p {paths.dependencies}")

    with open(layout_file) as f:
        deps_layout: DependenciesLayout = cast('DependenciesLayout', json.load(f))
    with ctx.cd(paths.dependencies):
        for new_dirs in deps_layout["layout"]:
            ctx.run(f"mkdir -p {new_dirs}", hide=(not verbose))

    for source in deps_layout["copy"]:
        target = deps_layout["copy"][source]
        ctx.run(f"cp {source_dir / source} {paths.dependencies / target}", hide=(not verbose))

    cc = get_compiler(ctx, arch)

    for build in deps_layout["build"]:
        directory = deps_layout["build"][build]["directory"]
        command = deps_layout["build"][build]["command"]
        artifact = source_dir / deps_layout["build"][build]["artifact"]
        if ci:
            ctx.run(f"cd {source_dir / directory} && {command}", hide=(not verbose))
        else:
            cc.exec(command, run_dir=os.path.join(CONTAINER_AGENT_PATH, directory), verbose=verbose)
        ctx.run(f"cp {artifact} {paths.dependencies}", hide=(not verbose))

    with ctx.cd(paths.dependencies.parent):
        ctx.run(f"tar czvf {paths.dependencies_archive.name} {paths.dependencies.name}", hide=(not verbose))


def is_root():
    return os.getuid() == 0


def vms_have_correct_deps(ctx: Context, domains: List[LibvirtDomain], depsfile: PathOrStr):
    deps_dir = os.path.dirname(depsfile)
    sha256sum = ctx.run(f"cd {deps_dir} && sha256sum {os.path.basename(depsfile)}", warn=True)
    if sha256sum is None or not sha256sum.ok:
        return False

    check = sha256sum.stdout.rstrip('\n')
    for d in domains:
        if not d.run_cmd(ctx, f"cd / && echo \"{check}\" | sha256sum --check", allow_fail=True):
            warn(f"[-] VM {d} does not have dependencies.")
            return False

    return True


def needs_build_from_scratch(ctx: Context, paths: KMTPaths, domains: "list[LibvirtDomain]", full_rebuild: bool):
    return (
        full_rebuild
        or (not paths.dependencies.exists())
        or (not vms_have_correct_deps(ctx, domains, paths.dependencies_archive))
    )


@task
def prepare(
    ctx: Context,
    vms: str,
    stack: Optional[str] = None,
    arch: Optional[Arch] = None,
    ssh_key: Optional[str] = None,
    full_rebuild=False,
    packages="",
    verbose=True,
):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if vms == "":
        raise Exit("No vms specified to sync with")
    if arch is None:
        arch = full_arch('local')

    info(f"[+] Preparing VMs {vms} in stack {stack} for {arch}")

    infra = build_infrastructure(stack, ssh_key)
    domains = filter_target_domains(vms, infra, arch)
    paths = KMTPaths(stack, arch)
    cc = get_compiler(ctx, arch)

    info("[+] Checking if we need a full rebuild...")
    build_from_scratch = needs_build_from_scratch(ctx, paths, domains, full_rebuild)

    constrain_pkgs = ""
    if not build_from_scratch and packages != "":
        info("[+] Dependencies already present in VMs")
        packages_with_ebpf = packages.split(",")
        packages_with_ebpf.append("./pkg/ebpf/bytecode")
        constrain_pkgs = f"--packages={','.join(set(packages_with_ebpf))}"
    else:
        warn("[!] Dependencies need to be rebuilt")

    info(f"[+] Compiling test binaries for {arch}")
    cc.exec(
        f"git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv -e system-probe.kitchen-prepare --ci {constrain_pkgs}",
        run_dir=CONTAINER_AGENT_PATH,
    )

    target_instances: List[HostInstance] = list()
    for d in domains:
        target_instances.append(d.instance)

    if build_from_scratch:
        info("[+] Building all dependencies from scratch")
        build_dependencies(
            ctx, arch, "test/new-e2e/system-probe/test-runner/files/system-probe-dependencies.json", "./", stack=stack
        )

        for instance in target_instances:
            instance.copy_to_all_vms(ctx, paths.dependencies_archive)

        for d in domains:
            if not d.run_cmd(ctx, f"/root/fetch_dependencies.sh {arch}", allow_fail=True, verbose=verbose):
                raise Exit(f"failed to fetch dependencies for domain {d}")

            info(f"[+] Dependencies shared with target VM {d}")

    info("[+] Building tests package")
    build_tests_package(ctx, "./", stack, arch, False)
    for d in domains:
        d.copy(ctx, paths.tests_archive, "/")
        d.run_cmd(ctx, f"cd / && tar xzf {paths.tests_archive.name}")
        info(f"[+] Tests packages setup in target VM {d}")


def build_run_config(run: Optional[str], packages: List[str]):
    c: Dict[str, Any] = dict()

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


@task(
    help={
        "vms": "Comma seperated list of vms to target when running tests. If None, run against all vms",
        "stack": "Stack in which the VMs exist. If not provided stack is autogenerated based on branch name",
        "packages": "Similar to 'system-probe.test'. Specify the package from which to run the tests",
        "run": "Similar to 'system-probe.test'. Specify the regex to match specific tests to run",
        "quick": "Assume no need to rebuild anything, and directly run the tests",
        "retry": "Number of times to retry a failing test",
        "run-count": "Number of times to run a tests regardless of status",
        "full-rebuild": "Do a full rebuild of all test dependencies to share with VMs, before running tests. Useful when changes are not being picked up correctly",
        "ssh-key": "SSH key to use for connecting to a remote EC2 instance hosting the target VM",
        "verbose": "Enable full output of all commands executed",
        "test-logs": "Set 'gotestsum' verbosity to 'standard-verbose' to print all test logs. Default is 'testname'",
        "test-extra-arguments": "Extra arguments to pass to the test runner, see `go help testflag` for more details",
    }
)
def test(
    ctx: Context,
    vms: Optional[str] = None,
    stack: Optional[str] = None,
    packages="",
    run: Optional[str] = None,
    quick=False,
    retry=2,
    run_count=1,
    full_rebuild=False,
    ssh_key: Optional[str] = None,
    verbose=True,
    test_logs=False,
    test_extra_arguments=None,
):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if vms is None:
        vms = ",".join(stacks.get_all_vms_in_stack(stack))
        info(f"[+] Running tests on all VMs in stack {stack}: vms={vms}")
    infra = build_infrastructure(stack, ssh_key)
    domains = filter_target_domains(vms, infra)
    used_archs = get_archs_in_domains(domains)

    info("[+] Detected architectures in target VMs: " + ", ".join(used_archs))

    if not quick:
        for arch in used_archs:
            prepare(ctx, stack=stack, vms=vms, ssh_key=ssh_key, full_rebuild=full_rebuild, packages=packages, arch=arch)

    if run is not None and packages is None:
        raise Exit("Package must be provided when specifying test")
    pkgs = packages.split(",")
    if run is not None and len(pkgs) > 1:
        raise Exit("Only a single package can be specified when running specific tests")

    run_config = build_run_config(run, pkgs)
    with tempfile.NamedTemporaryFile(mode='w') as tmp:
        json.dump(run_config, tmp)
        tmp.flush()

        args = [
            f"-packages-run-config /tmp/{os.path.basename(tmp.name)}",
            f"-retry {retry}",
            "-verbose" if test_logs else "",
            f"-run-count {run_count}",
            "-test-root /opt/system-probe-tests",
            f"-extra-params {test_extra_arguments}" if test_extra_arguments is not None else "",
        ]
        for d in domains:
            info(f"[+] Running tests on {d}")
            d.copy(ctx, f"{tmp.name}", "/tmp")
            d.run_cmd(ctx, f"bash /micro-vm-init.sh {' '.join(args)}", verbose=verbose)


@task(
    help={
        "vms": "Comma seperated list of vms to target when running tests",
        "stack": "Stack in which the VMs exist. If not provided stack is autogenerated based on branch name",
        "ssh-key": "SSH key to use for connecting to a remote EC2 instance hosting the target VM",
        "full-rebuild": "Do a full rebuild of all test dependencies to share with VMs, before running tests. Useful when changes are not being picked up correctly",
        "verbose": "Enable full output of all commands executed",
        "arch": "Architecture to build the system-probe for",
    }
)
def build(
    ctx: Context,
    vms: str,
    stack: Optional[str] = None,
    ssh_key: Optional[str] = None,
    full_rebuild=False,
    verbose=True,
    arch: Optional[ArchOrLocal] = None,
):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if arch is None:
        arch = "local"

    arch = full_arch(arch)
    paths = KMTPaths(stack, arch)
    paths.arch_dir.mkdir(parents=True, exist_ok=True)

    infra = build_infrastructure(stack, ssh_key)
    domains = filter_target_domains(vms, infra, arch)
    cc = get_compiler(ctx, arch)

    build_from_scratch = needs_build_from_scratch(ctx, paths, domains, full_rebuild)

    if build_from_scratch:
        build_dependencies(
            ctx, arch, "test/new-e2e/system-probe/test-runner/files/system-probe-dependencies.json", "./", stack=stack
        )

        target_instances: List[HostInstance] = list()
        for d in domains:
            target_instances.append(d.instance)

        for instance in target_instances:
            instance.copy_to_all_vms(ctx, f"kmt-deps/{stack}/dependencies-{full_arch(instance.arch)}.tar.gz")

        for d in domains:
            d.run_cmd(ctx, f"/root/fetch_dependencies.sh {arch_mapping[platform.machine()]}")
            info(f"[+] Dependencies shared with target VM {d}")

    cc.exec(
        f"cd {CONTAINER_AGENT_PATH} && git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv -e system-probe.build --no-bundle",
    )
    cc.exec(f"tar cf {CONTAINER_AGENT_PATH}/kmt-deps/{stack}/shared.tar {EMBEDDED_SHARE_DIR}")
    for d in domains:
        d.copy(ctx, "./bin/system-probe", "/root")
        d.copy(ctx, f"kmt-deps/{stack}/shared.tar", "/")
        d.run_cmd(ctx, "tar xf /shared.tar -C /", verbose=verbose)
        info(f"[+] system-probe built for {d.name} @ /root")


@task
def clean(ctx: Context, stack: Optional[str] = None, container=False, image=False):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    cc = get_compiler(ctx, full_arch("local"))
    cc.exec("inv -e system-probe.clean", run_dir=CONTAINER_AGENT_PATH)
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
def ssh_config(
    _, stacks: Optional[str] = None, ddvm_rsa="~/dd/ami-builder/scripts/kernel-version-testing/files/ddvm_rsa"
):
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

        for _, instance in build_infrastructure(stack.name, remote_ssh_key="").items():
            if instance.arch != "local":
                print(f"Host kmt-{stack_name}-{instance.arch}")
                print(f"    HostName {instance.ip}")
                print("    User ubuntu")
                print("")

            for domain in instance.microvms:
                print(f"Host kmt-{stack_name}-{instance.arch}-{domain.tag}")
                print(f"    HostName {domain.ip}")
                if instance.arch != "local":
                    print(f"    ProxyJump kmt-{stack_name}-{instance.arch}")
                print(f"    IdentityFile {ddvm_rsa}")
                print("    User root")

                for key, value in SSH_OPTIONS.items():
                    print(f"    {key} {value}")
                print("")


@task(
    help={
        "stack": "Name of the stack to get the status of. If None, show status of stack associated with current branch",
        "all": "Show status of all stacks. --stack parameter will be ignored",
    }
)
def status(ctx: Context, stack: Optional[str] = None, all=False, ssh_key: Optional[str] = None):
    stacks: List[str]

    if all:
        stacks = [stack.name for stack in Path(get_kmt_os().stacks_dir).iterdir() if stack.is_dir()]
    else:
        stacks = [check_and_get_stack(stack)]

    # Dict of status lines for each stack
    status: Dict[str, List[str]] = defaultdict(list)
    stack_status: Dict[str, Tuple[int, int, int, int]] = {}
    info("[+] Getting status...")

    for stack in stacks:
        try:
            infrastructure = build_infrastructure(stack, remote_ssh_key=ssh_key)
        except Exception:
            warn(f"Failed to get status for stack {stack}. stacks.output file might be corrupt.")
            print("")
            continue

        instances_down = 0
        instances_up = 0
        vms_down = 0
        vms_up = 0

        for arch, instance in infrastructure.items():
            if arch == 'local':
                status[stack].append("· Local VMs")
                instances_up += 1
            else:
                instance_id = ec2_instance_ids(ctx, [instance.ip])
                if len(instance_id) == 0:
                    status[stack].append(f"· {arch} AWS instance {instance.ip} - {colored('not running', 'red')}")
                    instances_down += 1
                else:
                    status[stack].append(
                        f"· {arch} AWS instance {instance.ip} - {colored('running', 'green')} - ID {instance_id[0]}"
                    )
                    instances_up += 1

            for vm in instance.microvms:
                vm_id = f"{vm.tag:14} | IP {vm.ip}"
                if vm.check_reachable(ctx):
                    status[stack].append(f"  - {vm_id} - {colored('up', 'green')}")
                    vms_up += 1
                else:
                    status[stack].append(f"  - {vm_id} - {colored('down', 'red')}")
                    vms_down += 1

            stack_status[stack] = (instances_down, instances_up, vms_down, vms_up)

    info("[+] Tasks completed, printing status")

    for stack, lines in status.items():
        instances_down, instances_up, vms_down, vms_up = stack_status[stack]

        if instances_down == 0 and instances_up == 0:
            status_str = colored("Empty", "grey")
        elif instances_up == 0:
            status_str = colored("Hosts down", "red")
        elif instances_down == 0:
            status_str = colored("Hosts active", "green")
        else:
            status_str = colored("Hosts partially active", "yellow")

        if vms_down == 0 and vms_up == 0:
            vm_status_str = colored("No VMs defined", "grey")
        elif vms_up == 0:
            vm_status_str = colored("All VMs down", "red")
        elif vms_down == 0:
            vm_status_str = colored("All VMs up", "green")
        else:
            vm_status_str = colored("Some VMs down", "yellow")

        print(f"Stack {stack} - {status_str} - {vm_status_str}")
        for line in lines:
            print(line)
        print("")

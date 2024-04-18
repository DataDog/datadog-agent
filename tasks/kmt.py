from __future__ import annotations

import itertools
import json
import os
import platform
import re
import tempfile
from collections import defaultdict
from glob import glob
from pathlib import Path
from typing import TYPE_CHECKING, Any, Dict, Iterable, List, Optional, Set, Tuple, cast

from invoke.context import Context
from invoke.tasks import task

from tasks.libs.build.ninja import NinjaWriter
from tasks.kernel_matrix_testing import stacks, vmconfig
from tasks.kernel_matrix_testing.ci import KMTTestRunJob, get_all_jobs_for_pipeline
from tasks.kernel_matrix_testing.compiler import CONTAINER_AGENT_PATH, all_compilers, get_compiler
from tasks.kernel_matrix_testing.config import ConfigManager
from tasks.kernel_matrix_testing.download import arch_mapping, update_rootfs
from tasks.kernel_matrix_testing.infra import (
    SSH_OPTIONS,
    HostInstance,
    LibvirtDomain,
    build_infrastructure,
    ensure_key_in_ec2,
    get_ssh_agent_key_names,
    get_ssh_key_name,
    try_get_ssh_key,
)
from tasks.kernel_matrix_testing.init_kmt import init_kernel_matrix_testing_system
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.stacks import check_and_get_stack, ec2_instance_ids
from tasks.kernel_matrix_testing.tool import Exit, ask, error, get_binary_target_arch, info, warn
from tasks.system_probe import (
    EMBEDDED_SHARE_DIR,
    TEST_PACKAGES_LIST,
    check_for_ninja,
    go_package_dirs,
    NPM_TAG,
    BPF_TAG,
    get_sysprobe_buildtags,
    get_test_timeout,
    ninja_generate,
)
from tasks.libs.common.utils import get_build_flags

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import (  # noqa: F401
        Arch,
        ArchOrLocal,
        Component,
        DependenciesLayout,
        PathOrStr,
        SSHKey,
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
    vmconfig_template: Component = "system-probe",
):
    """
    Generate a vmconfig.json file with the VMs that failed jobs in the given pipeline.
    """
    vms = set()
    local_arch = full_arch("local")

    if pipeline is None:
        raise Exit("Pipeline ID must be provided")

    info(f"[+] retrieving all CI jobs for pipeline {pipeline}")
    setup_jobs, test_jobs = get_all_jobs_for_pipeline(pipeline)

    for job in setup_jobs:
        if (vcpu is None or memory is None) and job.status == "success":
            info(f"[+] retrieving vmconfig from job {job.name}")
            for vmset in job.vmconfig["vmsets"]:
                memory_list = vmset.get("memory", [])
                if memory is None and len(memory_list) > 0:
                    memory = str(memory_list[0])
                    info(f"[+] setting memory to {memory}")

                vcpu_list = vmset.get("vcpu", [])
                if vcpu is None and len(vcpu_list) > 0:
                    vcpu = str(vcpu_list[0])
                    info(f"[+] setting vcpu to {vcpu}")

    failed_packages: Set[str] = set()
    for job in test_jobs:
        if job.status == "failed" and job.component == vmconfig_template:
            vm_arch = job.arch
            if use_local_if_possible and vm_arch == local_arch:
                vm_arch = local_arch

            failed_tests = job.get_test_results()
            failed_packages.update({test.split(':')[0] for test in failed_tests.keys()})
            vms.add(f"{vm_arch}-{job.distro}-distro")

    info(f"[+] generating {output_file} file for VMs {vms}")
    vcpu = DEFAULT_VCPU if vcpu is None else vcpu
    memory = DEFAULT_MEMORY if memory is None else memory
    vmconfig.gen_config(
        ctx, stack, ",".join(vms), "", init_stack, vcpu, memory, new, ci, arch, output_file, vmconfig_template
    )
    info("[+] You can run the following command to execute only packages with failed tests")
    print(f"inv kmt.test --packages=\"{' '.join(failed_packages)}\"")


@task
def launch_stack(
    ctx: Context,
    stack: Optional[str] = None,
    ssh_key: Optional[str] = None,
    x86_ami: str = X86_AMI_ID_SANDBOX,
    arm_ami: str = ARM_AMI_ID_SANDBOX,
    provision_microvms: bool = True,
):
    stacks.launch_stack(ctx, stack, ssh_key, x86_ami, arm_ami, provision_microvms)


@task
def destroy_stack(ctx: Context, stack: Optional[str] = None, pulumi=False, ssh_key: Optional[str] = None):
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
    config_ssh_key(ctx)


@task
def config_ssh_key(ctx: Context):
    """Automatically configure the default SSH key to use"""
    agent_choices = [
        ("ssh", "Keys located in ~/.ssh"),
        ("1password", "1Password SSH agent (valid for any other SSH agent too)"),
        ("manual", "Manual input"),
    ]
    choices = "\n".join([f" - [{i + 1}] {short}: {name}" for i, (short, name) in enumerate(agent_choices)])
    opts_numbers = [str(i + 1) for i in range(len(agent_choices))]
    opts_words = [name for name, _ in agent_choices]
    result = ask(
        f"[?] Choose your SSH key storage method\n{choices}\nChoose a number ({','.join(opts_numbers)}) or option name ({','.join(opts_words)}): "
    ).strip()
    method = None
    if result in opts_numbers:
        method = agent_choices[int(result) - 1][0]
    elif result in opts_words:
        method = result
    else:
        raise Exit(
            f"Invalid choice {result}, must be a number between 1 and {len(agent_choices)} or option name ({opts_words})"
        )

    ssh_key: SSHKey
    if method == "manual":
        warn("[!] The manual method does not do any validation. Ensure the key is valid and loaded in AWS.")
        ssh_key_path = ask("Enter the path to the SSH key (can be left blank): ")
        name = ask("Enter the key name: ")
        aws_config_name = ask("Enter the AWS key name (leave blank to set the same as the key name): ")
        if ssh_key_path.strip() == "":
            ssh_key_path = None
        if aws_config_name.strip() == "":
            aws_config_name = name

        ssh_key = {'path': ssh_key_path, 'name': name, 'aws_key_name': aws_config_name}
    else:
        info("[+] Finding SSH keys to use...")
        ssh_keys: List[SSHKey]
        if method == "1password":
            agent_keys = get_ssh_agent_key_names(ctx)
            ssh_keys = [{'path': None, 'name': key, 'aws_key_name': key} for key in agent_keys]
        else:
            ssh_key_files = [Path(f[: -len(".pub")]) for f in glob(os.path.expanduser("~/.ssh/*.pub"))]
            ssh_keys = []

            for f in ssh_key_files:
                key_comment = get_ssh_key_name(f.with_suffix(".pub"))
                if key_comment is None:
                    warn(f"[x] {f} does not have a valid key name, cannot be used")
                else:
                    ssh_keys.append({'path': os.fspath(f), 'name': key_comment, 'aws_key_name': ''})

        keys_str = "\n".join([f" - [{i + 1}] {key['name']} (path: {key['path']})" for i, key in enumerate(ssh_keys)])
        result = ask(f"[?] Found these valid key files:\n{keys_str}\nChoose one of these files (1-{len(ssh_keys)}): ")
        try:
            ssh_key = ssh_keys[int(result.strip()) - 1]
        except ValueError:
            raise Exit(f"Choice {result} is not a valid number")
        except IndexError:  # out of range
            raise Exit(f"Invalid choice {result}, must be a number between 1 and {len(ssh_keys)} (inclusive)")

        aws_key_name = ask(
            f"Enter the key name configured in AWS for this key (leave blank to set the same as the local key name '{ssh_key['name']}'): "
        )
        if aws_key_name.strip() != "":
            ssh_key['aws_key_name'] = aws_key_name.strip()
        else:
            ssh_key['aws_key_name'] = ssh_key['name']

        ensure_key_in_ec2(ctx, ssh_key)

    cm = ConfigManager()
    cm.config["ssh"] = ssh_key
    cm.save()

    info(
        f"[+] Saved for use: SSH key '{ssh_key}'. You can run this command later or edit the file manually in ~/kernel-version-testing/config.json"
    )


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


def download_gotestsum(ctx: Context, arch: Arch, fgotestsum: PathOrStr):
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
        return self.arch_dir / "testing-tools"

    @property
    def sysprobe_tests(self):
        return self.arch_dir / "opt/system-probe-tests"

    @property
    def tools(self):
        return self.root / self.arch / "tools"


def is_root():
    return os.getuid() == 0


@task
def prepare(
    ctx: Context,
    vms: str,
    stack: Optional[str] = None,
    arch: Optional[Arch] = None,
    ssh_key: Optional[str] = None,
    packages=None,
    verbose=True,
    ci=False,
):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    go_root = os.getenv("GOROOT")
    if not ci:
        download_gotestsum(ctx, arch, f"{go_root}/bin/gotestsum")

    if vms == "":
        raise Exit("No vms specified to sync with")
    if arch is None:
        arch = full_arch('local')

    info(f"[+] Preparing VMs {vms} in stack {stack} for {arch}")

    ssh_key_obj = try_get_ssh_key(ctx, ssh_key)
    infra = build_infrastructure(stack, ssh_key_obj)
    domains = filter_target_domains(vms, infra, arch)
    paths = KMTPaths(stack, arch)
    cc = get_compiler(ctx, arch)

    info(f"[+] Compiling test binaries for {arch}")

    pkgs = ""
    if packages:
        pkgs = f"--packages {packages}"
    cc.exec(
        f"git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv -e kmt.kmt-prepare --stack={stack} {pkgs}",
        run_dir=CONTAINER_AGENT_PATH,
    )

    copy_executables = {
        f"{go_root}/bin/gotestsum": f"{paths.dependencies}/go/bin/gotestsum",
        "/opt/datadog-agent/embedded/bin/clang-bpf": f"{paths.arch_dir}/opt/datadog-agent/embedded/bin/clang-bpf",
        "/opt/datadog-agent/embedded/bin/llc-bpf": f"{paths.arch_dir}/opt/datadog-agent/embedded/bin/llc-bpf",
        f"{os.getcwd()}/test/new-e2e/system-probe/test/micro-vm-init.sh": f"{paths.arch_dir}/opt/micro-vm-init.sh",
    }

    for sf, df in copy_executables.items():
        if os.path.exists(sf) and not os.path.exists(df):
            ctx.run(f"install -D {sf} {df}")

    target_instances: List[HostInstance] = list()
    for d in domains:
        target_instances.append(d.instance)

    for d in domains:
        d.copy(ctx, paths.dependencies, "/opt/", verbose=verbose)
        d.copy(ctx, f"{paths.arch_dir}/opt/*", "/opt/", exclude="*.ninja", verbose=verbose)
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


def build_target_packages(filter_packages):
    all_packages = go_package_dirs(TEST_PACKAGES_LIST, [NPM_TAG, BPF_TAG])
    if filter_packages == []:
        return all_packages

    filtered = list()
    for pkg in all_packages:
        if os.path.relpath(pkg) in filter_packages:
            filtered.append(pkg)

    return filtered


@task
def kmt_prepare(
    ctx: Context,
    stack: Optional[str] = None,
    kernel_release: Optional[str] = None,
    packages=None,
    arch: Optional[ArchOrLocal] = None,
    extra_arguments: Optional[str] = None,
):
    if stack is None:
        raise Exit("A stack name must be provided")

    if arch is None:
        arch = full_arch("local")

    check_for_ninja(ctx)

    filter_pkgs = []
    if packages:
        filter_pkgs = [os.path.relpath(p) for p in packages.split(",")]

    target_packages = build_target_packages(filter_pkgs)
    kmt_paths = KMTPaths(stack, arch)
    nf_path = os.path.join(kmt_paths.arch_dir, "kmt.ninja")
    object_files_nf_path = os.path.join(kmt_paths.arch_dir, "kmt-object-files.ninja")

    kmt_paths.arch_dir.mkdir(exist_ok=True, parents=True)
    kmt_paths.dependencies.mkdir(exist_ok=True, parents=True)

    go_path = "go"
    go_root = os.getenv("GOROOT")
    if go_root:
        go_path = os.path.join(go_root, "bin", "go")

    ninja_generate(ctx, object_files_nf_path)
    ctx.run(f"ninja -d explain -f {object_files_nf_path}")

    with open(nf_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file)

        # go build does not seem to be designed to run concurrently on the same
        # source files. To make go build work with ninja we create a pool to force
        # only a single instance of go to be running.
        nw.pool(name="gobuild", depth=1)

        nw.rule(
            name="gotestsuite",
            command="$env $go test -mod=mod -v $timeout -tags \"$build_tags\" $extra_arguments -c -o $out $in",
        )
        nw.rule(name="copyextra", command="cp -r $in $out")
        nw.rule(
            name="gobin",
            command="$chdir && $env $go build -o $out $tags $ldflags $in $tool",
        )
        nw.rule(name="copyfiles", command="install -D $in $out $mode")

        _, _, env = get_build_flags(ctx)
        env["DD_SYSTEM_PROBE_BPF_DIR"] = EMBEDDED_SHARE_DIR

        env_str = ""
        for key, val in env.items():
            new_val = val.replace('\n', ' ')
            env_str += f"{key}='{new_val}' "
        env_str.rstrip()

        nw.build(
            rule="gobin",
            pool="gobuild",
            outputs=[os.path.join(kmt_paths.dependencies, "test-runner")],
            variables={
                "go": go_path,
                "chdir": "cd test/new-e2e/system-probe/test-runner",
            },
        )

        nw.build(
            rule="gobin",
            pool="gobuild",
            outputs=[os.path.join(kmt_paths.dependencies, "test-json-review")],
            variables={
                "go": go_path,
                "chdir": "cd test/new-e2e/system-probe/test-json-review/",
            },
        )

        nw.build(
            outputs=[f"{kmt_paths.dependencies}/go/bin/test2json"],
            rule="gobin",
            pool="gobuild",
            variables={
                "go": go_path,
                "ldflags": "-ldflags=\"-s -w\"",
                "chdir": "true",
                "tool": "cmd/test2json",
                "env": "CGO_ENABLED=0",
            },
        )

        # copy object files
        object_files = [
            os.path.abspath(i) for i in glob("**/*.o", recursive=True) if i.split('/')[0] == "pkg" and "build" in i
        ]
        for file in object_files:
            out = f"{kmt_paths.sysprobe_tests}/{os.path.relpath(file)}"
            nw.build(inputs=[file], outputs=[out], rule="copyfiles", variables={"mode": "-m744"})

        print(f"ALl packages: {target_packages}")
        for pkg in target_packages:
            target_path = os.path.join(kmt_paths.sysprobe_tests, os.path.relpath(pkg, os.getcwd()))
            output_path = os.path.join(target_path, "testsuite")
            variables = {
                "env": env_str,
                "go": go_path,
                "build_tags": get_sysprobe_buildtags(False, False),
            }
            timeout = get_test_timeout(os.path.relpath(pkg, os.getcwd()))
            if timeout:
                variables["timeout"] = f"-timeout {timeout}"
            if extra_arguments:
                variables["extra_arguments"] = extra_arguments

            go_files = [os.path.abspath(i) for i in glob(f"{pkg}/*.go", recursive=True)]
            nw.build(
                inputs=[pkg],
                outputs=[output_path],
                implicit=go_files,
                rule="gotestsuite",
                pool="gobuild",
                variables=variables,
            )

            testdata = os.path.join(pkg, "testdata")
            if os.path.exists(testdata):
                nw.build(inputs=[testdata], outputs=[os.path.join(target_path, "testdata")], rule="copyextra")

            if pkg.endswith("java"):
                nw.build(
                    inputs=[os.path.join(pkg, "agent-usm.jar")],
                    outputs=[os.path.join(target_path, "agent-usm.jar")],
                    rule="copyfiles",
                )

            for gobin in [
                "gotls_client",
                "grpc_external_server",
                "external_unix_proxy_server",
                "fmapper",
                "prefetch_file",
            ]:
                src_file_path = os.path.join(pkg, f"{gobin}.go")
                if os.path.isdir(pkg) and os.path.isfile(src_file_path):
                    binary_path = os.path.join(target_path, gobin)
                    nw.build(
                        inputs=[f"{pkg}/{gobin}.go"],
                        outputs=[binary_path],
                        rule="gobin",
                        pool="gobuild",
                        variables={
                            "go": go_path,
                            "chdir": "true",
                            "tags": "-tags=\"test\"",
                            "ldflags": "-ldflags=\"-extldflags '-static'\"",
                        },
                    )

    ctx.run(f"ninja -d explain -v -f {nf_path}")


@task(
    help={
        "vms": "Comma seperated list of vms to target when running tests. If None, run against all vms",
        "stack": "Stack in which the VMs exist. If not provided stack is autogenerated based on branch name",
        "packages": "Similar to 'system-probe.test'. Specify the package from which to run the tests",
        "run": "Similar to 'system-probe.test'. Specify the regex to match specific tests to run",
        "quick": "Assume no need to rebuild anything, and directly run the tests",
        "retry": "Number of times to retry a failing test",
        "run-count": "Number of times to run a tests regardless of status",
        "ssh-key": "SSH key to use for connecting to a remote EC2 instance hosting the target VM. Can be either a name of a file in ~/.ssh, a key name (the comment in the public key) or a full path",
        "verbose": "Enable full output of all commands executed",
        "test-logs": "Set 'gotestsum' verbosity to 'standard-verbose' to print all test logs. Default is 'testname'",
        "test-extra-arguments": "Extra arguments to pass to the test runner, see `go help testflag` for more details",
    }
)
def test(
    ctx: Context,
    vms: Optional[str] = None,
    stack: Optional[str] = None,
    packages=None,
    run: Optional[str] = None,
    quick=False,
    retry=2,
    run_count=1,
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
    ssh_key_obj = try_get_ssh_key(ctx, ssh_key)
    infra = build_infrastructure(stack, ssh_key_obj)
    domains = filter_target_domains(vms, infra)
    used_archs = get_archs_in_domains(domains)

    if len(domains) == 0:
        raise Exit(f"no vms found from list {vms}. Run `inv -e kmt.status` to see all VMs in current stack")

    info("[+] Detected architectures in target VMs: " + ", ".join(used_archs))

    if not quick:
        for arch in used_archs:
            prepare(ctx, stack=stack, vms=vms, packages=packages, ssh_key=ssh_key, arch=arch)

    if run is not None and packages is None:
        raise Exit("Package must be provided when specifying test")
    pkgs = packages.split(",")
    if run is not None and len(pkgs) > 1:
        raise Exit("Only a single package can be specified when running specific tests")

    run_config = build_run_config(run, pkgs)
    with tempfile.NamedTemporaryFile(mode='w') as tmp:
        json.dump(run_config, tmp)
        tmp.flush()
        remote_tmp = "/tmp"
        remote_run_config = os.path.join(remote_tmp, os.path.basename(tmp.name))

        args = [
            f"-packages-run-config {remote_run_config}",
            f"-retry {retry}",
            "-verbose" if test_logs else "",
            f"-run-count {run_count}",
            "-test-root /opt/system-probe-tests",
            f"-extra-params {test_extra_arguments}" if test_extra_arguments is not None else "",
        ]
        for d in domains:
            info(f"[+] Running tests on {d}")
            d.copy(ctx, f"{tmp.name}", remote_tmp)
            d.run_cmd(ctx, f"/opt/micro-vm-init.sh {' '.join(args)}", verbose=verbose)


def build_layout(ctx, domains, layout: str, verbose: bool):
    with open(layout, 'r') as lf:
        todo: DependenciesLayout = cast('DependenciesLayout', json.load(lf))

    for d in domains:
        mkdir = list()
        for dirs in todo["layout"]:
            mkdir.append(f"mkdir -p {dirs} &&")

        cmd = ' '.join(mkdir)
        d.run_cmd(ctx, cmd.rstrip(" &&"), verbose)

        for src, dst in todo["copy"].items():
            if not os.path.exists(src):
                raise Exit(f"File {src} specified in {layout} does not exist")

            d.copy(ctx, src, dst)

        for cmd in todo["run"]:
            d.run_cmd(ctx, cmd, verbose)


@task(
    help={
        "vms": "Comma seperated list of vms to target when running tests",
        "stack": "Stack in which the VMs exist. If not provided stack is autogenerated based on branch name",
        "ssh-key": "SSH key to use for connecting to a remote EC2 instance hosting the target VM. Can be either a name of a file in ~/.ssh, a key name (the comment in the public key) or a full path",
        "verbose": "Enable full output of all commands executed",
        "arch": "Architecture to build the system-probe for",
        "layout": "Path to file specifying the expected layout on the target VMs",
    }
)
def build(
    ctx: Context,
    vms: str,
    stack: Optional[str] = None,
    ssh_key: Optional[str] = None,
    verbose=True,
    arch: Optional[ArchOrLocal] = None,
    layout: Optional[str] = "tasks/kernel_matrix_testing/build-layout.json",
):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if arch is None:
        arch = "local"

    if not os.path.exists(layout):
        raise Exit(f"File {layout} does not exist")

    arch = full_arch(arch)
    paths = KMTPaths(stack, arch)
    paths.arch_dir.mkdir(parents=True, exist_ok=True)

    ssh_key_obj = try_get_ssh_key(ctx, ssh_key)
    infra = build_infrastructure(stack, ssh_key_obj)
    domains = filter_target_domains(vms, infra, arch)
    cc = get_compiler(ctx, arch)

    if len(domains) == 0:
        raise Exit(f"no vms found from list {vms}. Run `inv -e kmt.status` to see all VMs in current stack")

    cc.exec(
        f"cd {CONTAINER_AGENT_PATH} && git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv -e system-probe.build --no-bundle",
    )
    cc.exec(f"tar cf {CONTAINER_AGENT_PATH}/kmt-deps/{stack}/build-embedded-dir.tar {EMBEDDED_SHARE_DIR}")

    with open(layout, 'r') as lf:
        todo: DependenciesLayout = cast('DependenciesLayout', json.load(lf))

    build_layout(ctx, domains, layout, verbose)
    for d in domains:
        d.copy(ctx, "./bin/system-probe", "/root/")
        d.copy(ctx, f"kmt-deps/{stack}/build-embedded-dir.tar", "/")
        d.run_cmd(ctx, "tar xf /build-embedded-dir.tar -C /", verbose=verbose)
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
    ctx: Context,
    stacks: Optional[str] = None,
    ddvm_rsa="tasks/kernel_matrix_testing/ddvm_rsa",
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

        for _, instance in build_infrastructure(stack.name, try_get_ssh_key(ctx, None)).items():
            if instance.arch != "local":
                print(f"Host kmt-{stack_name}-{instance.arch}")
                print(f"    HostName {instance.ip}")
                print("    User ubuntu")
                if instance.ssh_key_path is not None:
                    print(f"    IdentityFile {instance.ssh_key_path}")
                    print("    IdentitiesOnly yes")
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
    ssh_key_obj = try_get_ssh_key(ctx, ssh_key)

    for stack in stacks:
        try:
            infrastructure = build_infrastructure(stack, ssh_key_obj)
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


@task
def explain_ci_failure(_, pipeline: str):
    """Show a summary of KMT failures in the given pipeline."""
    if tabulate is None:
        raise Exit("tabulate module is not installed, please install it to continue")

    info(f"[+] retrieving all CI jobs for pipeline {pipeline}")
    setup_jobs, test_jobs = get_all_jobs_for_pipeline(pipeline)

    failed_setup_jobs = [j for j in setup_jobs if j.status == "failed"]
    failed_jobs = [j for j in test_jobs if j.status == "failed"]
    failreasons: Dict[str, str] = {}
    ok = "✅"
    testfail = "❌"
    infrafail = "⚙️"
    result_to_emoji = {
        True: ok,
        False: testfail,
        None: " ",
    }

    if len(failed_jobs) == 0 and len(failed_setup_jobs) == 0:
        info("[+] No KMT tests failed")
        return

    # Compute a reason for failure for each test run job
    for job in failed_jobs:
        if job.failure_reason == "script_failure":
            failreason = testfail  # By default, we assume it's a test failure

            # Now check the artifacts, we'll guess why the job failed based on the size
            for artifact in job.job_data.get("artifacts", []):
                if artifact.get("filename") == "artifacts.zip":
                    fsize = artifact.get("size", 0)
                    if fsize < 1500:
                        # This means we don't have the junit test results, assuming an infra
                        # failure because tests didn't even run
                        failreason = infrafail
                        break
        else:
            failreason = job.failure_reason

        failreasons[job.name] = failreason

    # Check setup-env jobs that failed, they are infra failures for all related test jobs
    for job in failed_setup_jobs:
        for test_job in job.associated_test_jobs:
            failreasons[test_job.name] = infrafail
            failed_jobs.append(test_job)

    warn(f"[!] Found {len(failed_jobs)} failed jobs. Showing only distros with failures")

    print(f"Legend: OK {ok} | Test failure {testfail} | Infra failure {infrafail} | Skip ' ' (empty cell)")

    def groupby_comp_vmset(job: KMTTestRunJob) -> Tuple[str, str]:
        return (job.component, job.vmset)

    # Show first a matrix of failed distros and archs for each tuple of component and vmset
    jobs_by_comp_and_vmset = itertools.groupby(sorted(failed_jobs, key=groupby_comp_vmset), groupby_comp_vmset)
    for (component, vmset), group_jobs in jobs_by_comp_and_vmset:
        group_jobs = list(group_jobs)  # Consume the iterator, make a copy
        distros: Dict[str, Dict[Arch, str]] = defaultdict(lambda: {"x86_64": " ", "arm64": " "})
        distro_arch_with_test_failures: List[Tuple[str, Arch]] = []

        # Build the distro table with all jobs for this component and vmset, to correctly
        # differentiate between skipped and ok jobs
        for job in test_jobs:
            if job.component != component or job.vmset != vmset:
                continue

            failreason = failreasons.get(job.name, ok)
            distros[job.distro][job.arch] = failreason
            if failreason == testfail:
                distro_arch_with_test_failures.append((job.distro, job.arch))

        # Filter out distros with no failures
        distros = {d: v for d, v in distros.items() if any(r == testfail or r == infrafail for r in v.values())}

        print(f"\n=== Job failures for {component} - {vmset}")
        table = [[d, v["x86_64"], v["arm64"]] for d, v in distros.items()]
        print(tabulate(sorted(table, key=lambda x: x[0]), headers=["Distro", "x86_64", "arm64"]))

        ## Show a table summary with failed tests
        jobs_with_failed_tests = [j for j in group_jobs if failreasons[j.name] == testfail]
        test_results_by_distro_arch = {(j.distro, j.arch): j.get_test_results() for j in jobs_with_failed_tests}
        # Get the names of all tests
        all_tests = set(itertools.chain.from_iterable(d.keys() for d in test_results_by_distro_arch.values()))
        test_failure_table: List[List[str]] = []

        for testname in sorted(all_tests):
            test_row = [testname]
            for distro, arch in distro_arch_with_test_failures:
                test_result = test_results_by_distro_arch.get((distro, arch), {}).get(testname)
                test_row.append(result_to_emoji[test_result])

            # Only show tests with at least one failure:
            if any(r == testfail for r in test_row[1:]):
                test_failure_table.append(test_row)

        if len(test_failure_table) > 0:
            print(
                f"\n=== Test failures for {component} - {vmset} (show only tests and distros with at least one fail, empty means skipped)"
            )
            print(
                tabulate(
                    test_failure_table,
                    headers=["Test name"] + [f"{d} {a}" for d, a in distro_arch_with_test_failures],
                    tablefmt="simple_grid",
                )
            )

    def groupby_arch_comp(job: KMTTestRunJob) -> Tuple[str, str]:
        return (job.arch, job.component)

    # Now get the exact infra failure for each VM
    failed_infra_jobs = [j for j in failed_jobs if failreasons[j.name] == infrafail]
    jobs_by_arch_comp = itertools.groupby(sorted(failed_infra_jobs, key=groupby_arch_comp), groupby_arch_comp)
    for (arch, component), group_jobs in jobs_by_arch_comp:
        info(f"\n[+] Analyzing {component} {arch} infra failures...")
        group_jobs = list(group_jobs)  # Iteration consumes the value, we have to store it

        setup_job = next((x.setup_job for x in group_jobs if x.setup_job is not None), None)
        if setup_job is None:
            error("[x] No corresponding setup job found")
            continue

        infra_fail_table: List[List[str]] = []
        for failed_job in group_jobs:
            try:
                boot_log = setup_job.get_vm_boot_log(failed_job.distro, failed_job.vmset)
            except Exception as e:
                error(f"[x] error getting boot log for {failed_job.distro}: {e}")
                continue

            if boot_log is None:
                error(f"[x] no boot log present for {failed_job.distro}")
                continue

            vmdata = setup_job.get_vm(failed_job.distro, failed_job.vmset)
            if vmdata is None:
                error("[x] could not find VM in stack.output")
                continue
            microvm_ip = vmdata[1]

            # Some distros do not show the systemd service status in the boot log, which means
            # that we cannot infer the state of services from that boot log. Filter only non-kernel
            # lines in the output (kernel logs always are prefaced by [ seconds-since-boot ] so
            # they're easy to filter out) to see if there we can find clues that tell us whether
            # we have status logs or not.
            non_kernel_boot_log_lines = [
                l for l in boot_log.splitlines() if re.match(r"\[[0-9 \.]+\]", l) is None
            ]  # reminder: match only searches pattern at the beginning of string
            non_kernel_boot_log = "\n".join(non_kernel_boot_log_lines)
            # systemd will always show the journal service starting in the boot log if it's outputting there
            have_service_status_logs = re.search("Journal Service", non_kernel_boot_log, re.IGNORECASE) is not None

            # From the boot log we can get clues about the state of the VM
            booted = re.search(r"(ddvm|pool[0-9\-]+) login: ", boot_log) is not None
            setup_ddvm = (
                re.search("(Finished|Started) ([^\\n]+)?Setup ddvm", non_kernel_boot_log) is not None
                if have_service_status_logs
                else None
            )
            ip_assigned = microvm_ip in setup_job.seen_ips

            boot_log_savepath = (
                Path("/tmp")
                / f"kmt-pipeline-{pipeline}"
                / f"{arch}-{component}-{failed_job.distro}-{failed_job.vmset}.boot.log"
            )
            boot_log_savepath.parent.mkdir(parents=True, exist_ok=True)
            boot_log_savepath.write_text(boot_log)

            infra_fail_table.append(
                [
                    failed_job.distro,
                    result_to_emoji[booted],
                    result_to_emoji[setup_ddvm],
                    result_to_emoji[ip_assigned],
                    os.fspath(boot_log_savepath),
                ]
            )

        print(
            tabulate(
                infra_fail_table,
                headers=["Distro", "Login prompt found", "setup-ddvm ok", "Assigned IP", "Downloaded boot log"],
            )
        )

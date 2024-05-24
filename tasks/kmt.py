from __future__ import annotations

import itertools
import json
import os
import re
import tempfile
from collections import defaultdict
from collections.abc import Callable, Iterable
from glob import glob
from pathlib import Path
from typing import TYPE_CHECKING, Any, cast

from invoke.context import Context
from invoke.tasks import task

from tasks.kernel_matrix_testing import selftest as selftests
from tasks.kernel_matrix_testing import stacks, vmconfig
from tasks.kernel_matrix_testing.ci import KMTTestRunJob, get_all_jobs_for_pipeline
from tasks.kernel_matrix_testing.compiler import CONTAINER_AGENT_PATH, get_compiler
from tasks.kernel_matrix_testing.config import ConfigManager
from tasks.kernel_matrix_testing.download import update_rootfs
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
from tasks.kernel_matrix_testing.platforms import get_platforms, platforms_file
from tasks.kernel_matrix_testing.stacks import check_and_get_stack, ec2_instance_ids
from tasks.kernel_matrix_testing.tool import Exit, ask, error, get_binary_target_arch, info, warn
from tasks.kernel_matrix_testing.vars import KMT_SUPPORTED_ARCHS, KMTPaths
from tasks.libs.build.ninja import NinjaWriter
from tasks.libs.common.utils import get_build_flags
from tasks.libs.types.arch import ARCH_AMD64, ARCH_ARM64, Arch, KMTArchName
from tasks.security_agent import build_functional_tests, build_stress_tests
from tasks.system_probe import (
    BPF_TAG,
    EMBEDDED_SHARE_DIR,
    NPM_TAG,
    TEST_PACKAGES_LIST,
    check_for_ninja,
    get_ebpf_build_dir,
    get_ebpf_runtime_dir,
    get_sysprobe_buildtags,
    get_test_timeout,
    go_package_dirs,
    ninja_generate,
    setup_runtime_clang,
)

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import (  # noqa: F401
        Component,
        DependenciesLayout,
        KMTArchNameOrLocal,
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

    def colored(text: str, color: str | None) -> str:  # noqa: U100
        return text


X86_AMI_ID_SANDBOX = "ami-0d1f81cfdbd5b0188"
ARM_AMI_ID_SANDBOX = "ami-02cb18e91afb3777c"
DEFAULT_VCPU = "4"
DEFAULT_MEMORY = "8192"

CLANG_PATH_CI = Path("/tmp/clang-bpf")
LLC_PATH_CI = Path("/tmp/llc-bpf")


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
        "yes": "Do not ask for confirmation",
    }
)
def gen_config(
    ctx: Context,
    stack: str | None = None,
    vms: str = "",
    sets: str = "",
    init_stack=False,
    vcpu: str | None = None,
    memory: str | None = None,
    new=False,
    ci=False,
    arch: str = "",
    output_file: str = "vmconfig.json",
    from_ci_pipeline: str | None = None,
    use_local_if_possible=False,
    vmconfig_template: Component = "system-probe",
    yes=False,
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
            yes=yes,
        )
    else:
        vcpu = DEFAULT_VCPU if vcpu is None else vcpu
        memory = DEFAULT_MEMORY if memory is None else memory
        vmconfig.gen_config(
            ctx, stack, vms, sets, init_stack, vcpu, memory, new, ci, arch, output_file, vmconfig_template, yes=yes
        )


def gen_config_from_ci_pipeline(
    ctx: Context,
    stack: str | None = None,
    pipeline: str | None = None,
    init_stack=False,
    vcpu: str | None = None,
    memory: str | None = None,
    new=False,
    ci=False,
    use_local_if_possible=False,
    arch: str = "",
    output_file="vmconfig.json",
    vmconfig_template: Component = "system-probe",
    yes=False,
):
    """
    Generate a vmconfig.json file with the VMs that failed jobs in the given pipeline.
    """
    vms = set()
    local_arch = Arch.local().kmt_arch

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

    failed_packages: set[str] = set()
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
        ctx, stack, ",".join(vms), "", init_stack, vcpu, memory, new, ci, arch, output_file, vmconfig_template, yes=yes
    )
    info("[+] You can run the following command to execute only packages with failed tests")
    print(f"inv kmt.test --packages=\"{' '.join(failed_packages)}\"")


@task
def launch_stack(
    ctx: Context,
    stack: str | None = None,
    ssh_key: str | None = None,
    x86_ami: str = X86_AMI_ID_SANDBOX,
    arm_ami: str = ARM_AMI_ID_SANDBOX,
    provision_microvms: bool = True,
):
    stacks.launch_stack(ctx, stack, ssh_key, x86_ami, arm_ami, provision_microvms)


@task
def destroy_stack(ctx: Context, stack: str | None = None, pulumi=False, ssh_key: str | None = None):
    clean(ctx, stack)
    stacks.destroy_stack(ctx, stack, pulumi, ssh_key)


@task
def pause_stack(_, stack: str | None = None):
    stacks.pause_stack(stack)


@task
def resume_stack(_, stack: str | None = None):
    stacks.resume_stack(stack)


@task
def ls(_, distro=True, custom=False):
    if tabulate is None:
        raise Exit("tabulate module is not installed, please install it to continue")

    print(tabulate(vmconfig.get_image_list(distro, custom), headers='firstrow', tablefmt='fancy_grid'))


@task(
    help={
        "lite": "If set, then do not download any VM images locally",
        "images": "Comma separated list of images to update, instead of everything. The format of each image is '<os_id>-<os_version>'. Refer to platforms.json for the appropriate values for <os_id> and <os_version>.",
    }
)
def init(ctx: Context, lite=False, images: str | None = None):
    init_kernel_matrix_testing_system(ctx, lite, images)
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
        ssh_keys: list[SSHKey]
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


@task(
    help={
        "vmconfig-template": "template to use for the target component",
        "all_archs": "Download images for all supported architectures. By default only images for the host architecture are downloaded",
        "images": "Comma separated list of images to update, instead of everything. The format of each image is '<os_id>-<os_version>'. Refer to platforms.json for the appropriate values for <os_id> and <os_version>.",
    }
)
def update_resources(
    ctx: Context, vmconfig_template="system-probe", all_archs: bool = False, images: str | None = None
):
    kmt_os = get_kmt_os()

    warn("Updating resource dependencies will delete all running stacks.")
    if ask("are you sure you want to continue? (y/n)").lower() != "y":
        raise Exit("[-] Update aborted")

    for stack in glob(f"{kmt_os.stacks_dir}/*"):
        destroy_stack(ctx, stack=os.path.basename(stack))

    update_rootfs(ctx, kmt_os.rootfs_dir, vmconfig_template, all_archs=all_archs, images=images)


@task
def start_compiler(ctx: Context):
    cc = get_compiler(ctx)
    info(f"[+] Starting compiler {cc.name}")
    cc.start()


def filter_target_domains(vms: str, infra: dict[KMTArchNameOrLocal, HostInstance], arch: Arch | None = None):
    vmsets = vmconfig.build_vmsets(vmconfig.build_normalized_vm_def_set(vms), [])
    domains: list[LibvirtDomain] = list()
    for vmset in vmsets:
        if arch is not None and Arch.from_str(vmset.arch) != arch:
            warn(f"Ignoring VM {vmset} as it is not of the expected architecture {arch}")
            continue
        for vm in vmset.vms:
            for domain in infra[vmset.arch].microvms:
                if domain.tag == vm.version:
                    domains.append(domain)

    return domains


def get_archs_in_domains(domains: Iterable[LibvirtDomain]) -> set[Arch]:
    archs: set[Arch] = set()
    for d in domains:
        archs.add(Arch.from_str(d.instance.arch))
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

    cc = get_compiler(ctx)
    target_path = CONTAINER_AGENT_PATH / paths.tools.relative_to(paths.repo_root) / "gotestsum"
    cc.exec(
        f"cd {TOOLS_PATH} && GOARCH={arch.go_arch} go build -o {target_path} {GOTESTSUM}",
    )

    ctx.run(f"cp {paths.tools}/gotestsum {fgotestsum}")


def is_root():
    return os.getuid() == 0


def ninja_define_rules(nw: NinjaWriter):
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
    nw.rule(name="copyfiles", command="mkdir -p $$(dirname $out) && install $in $out $mode")


def ninja_build_dependencies(ctx: Context, nw: NinjaWriter, kmt_paths: KMTPaths, go_path: str, arch: Arch):
    _, _, env = get_build_flags(ctx, arch=arch)
    env_str = " ".join([f"{k}=\"{v.strip()}\"" for k, v in env.items()])

    test_runner_files = glob("test/new-e2e/system-probe/test-runner/*.go")
    nw.build(
        rule="gobin",
        pool="gobuild",
        outputs=[os.path.join(kmt_paths.dependencies, "test-runner")],
        implicit=test_runner_files,
        variables={
            "go": go_path,
            "chdir": "cd test/new-e2e/system-probe/test-runner",
            "env": env_str,
        },
    )
    test_runner_config = glob("test/new-e2e/system-probe/test-runner/files/*.json")
    for f in test_runner_config:
        nw.build(
            rule="copyfiles",
            outputs=[f"{kmt_paths.arch_dir}/opt/{os.path.basename(f)}"],
            inputs=[os.path.abspath(f)],
        )

    test_json_files = glob("test/new-e2e/system-probe/test-json-review/*.go")
    nw.build(
        rule="gobin",
        pool="gobuild",
        outputs=[os.path.join(kmt_paths.dependencies, "test-json-review")],
        implicit=test_json_files,
        variables={
            "go": go_path,
            "chdir": "cd test/new-e2e/system-probe/test-json-review/",
            "env": env_str,
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
            "env": f"{env_str} CGO_ENABLED=0",
        },
    )

    nw.build(
        rule="copyfiles",
        outputs=[f"{kmt_paths.arch_dir}/opt/micro-vm-init.sh"],
        inputs=[f"{os.getcwd()}/test/new-e2e/system-probe/test/micro-vm-init.sh"],
        variables={"mode": "-m744"},
    )


def ninja_copy_ebpf_files(
    nw: NinjaWriter,
    component: Component,
    kmt_paths: KMTPaths,
    arch: Arch,
    filter_fn: Callable[[Path], bool] = lambda _: True,
):
    # copy ebpf files from build and runtime dirs
    build_dir = get_ebpf_build_dir(arch).absolute()
    runtime_dir = get_ebpf_runtime_dir().absolute()

    # Copy to the target directory, retaining the directory structure
    root = kmt_paths.secagent_tests if component == "security-agent" else kmt_paths.sysprobe_tests
    output = root / build_dir.relative_to(Path.cwd().absolute())

    def filter(x: Path):
        return filter_fn(x) and x.is_file()

    to_copy = [(p, output / p.relative_to(build_dir)) for p in build_dir.glob("**/*.o") if filter(p)]
    to_copy += [(p, output / "runtime" / p.relative_to(runtime_dir)) for p in runtime_dir.glob("**/*.c") if filter(p)]

    for source, target in to_copy:
        nw.build(inputs=[os.fspath(source)], outputs=[os.fspath(target)], rule="copyfiles", variables={"mode": "-m744"})


@task
def kmt_secagent_prepare(
    ctx: Context,
    vms: str | None = None,
    stack: str | None = None,
    arch: Arch | str | None = None,
    ssh_key: str | None = None,
    packages: str | None = None,
    verbose: bool = True,
    ci: bool = True,
    compile_only: bool = False,
):
    if arch is None:
        arch = "local"
    arch = Arch.from_str(arch)
    kmt_paths = KMTPaths(stack, arch)
    kmt_paths.secagent_tests.mkdir(exist_ok=True, parents=True)

    build_object_files(ctx, f"{kmt_paths.arch_dir}/kmt-secagent-obj-files.ninja", arch)
    build_functional_tests(
        ctx,
        bundle_ebpf=False,
        race=True,
        debug=True,
        output=f"{kmt_paths.secagent_tests}/pkg/security/testsuite",
        skip_linters=True,
        skip_object_files=True,
        arch=arch,
    )
    build_stress_tests(ctx, output=f"{kmt_paths.secagent_tests}/pkg/security/stresssuite", skip_linters=True)

    go_path = "go"
    go_root = os.getenv("GOROOT")
    if go_root:
        go_path = os.path.join(go_root, "bin", "go")

    nf_path = kmt_paths.arch_dir / "kmt-secagent.ninja"
    with open(nf_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file)

        ninja_define_rules(nw)
        ninja_build_dependencies(ctx, nw, kmt_paths, go_path, arch)
        ninja_copy_ebpf_files(
            nw,
            "security-agent",
            kmt_paths,
            arch,
            filter_fn=lambda x: os.path.basename(x).startswith("runtime-security"),
        )

    ctx.run(f"ninja -d explain -v -f {nf_path}")


@task
def prepare(
    ctx: Context,
    component: Component,
    vms: str | None = None,
    stack: str | None = None,
    arch: str | Arch | None = None,
    ssh_key: str | None = None,
    packages=None,
    verbose=True,
    ci=False,
    compile_only=False,
):
    if not ci:
        stack = check_and_get_stack(stack)
        assert stacks.stack_exists(
            stack
        ), f"Stack {stack} does not exist. Please create with 'inv kmt.create-stack --stack=<name>'"
    else:
        stack = "ci"

    if arch is None:
        arch = "local"
    arch_obj = Arch.from_str(arch)
    if arch_obj not in {ARCH_AMD64, ARCH_ARM64}:
        raise Exit(
            f"Architecture {arch} (inferred {arch_obj}) is not supported. Supported architectures are amd64 and arm64"
        )
    cc = get_compiler(ctx)

    if arch_obj.is_cross_compiling():
        cc.ensure_ready_for_cross_compile()

    pkgs = ""
    if packages:
        pkgs = f"--packages {packages}"

    info(f"[+] Compiling artifacts for {arch_obj}, component = {component}")
    if component == "security-agent":
        if ci:
            kmt_secagent_prepare(ctx, vms, stack, arch_obj, ssh_key, packages, verbose, ci)
        else:
            cc.exec(
                f"git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv -e kmt.kmt-secagent-prepare --stack={stack} {pkgs} --arch={arch_obj.name}",
                run_dir=CONTAINER_AGENT_PATH,
            )
    elif component == "system-probe":
        if ci:
            kmt_sysprobe_prepare(ctx, arch_obj, ci=True)
        else:
            cc.exec(
                f"git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv -e kmt.kmt-sysprobe-prepare --stack={stack} {pkgs} --arch={arch_obj.name}",
                run_dir=CONTAINER_AGENT_PATH,
            )
    else:
        raise Exit(f"Component can only be 'system-probe' or 'security-agent'. {component} not supported.")

    info(f"[+] Preparing helper binaries for {arch_obj}")

    paths = KMTPaths(stack, arch_obj)

    if ci:
        # In CI, these binaries are always present
        llc_path = LLC_PATH_CI
        clang_path = CLANG_PATH_CI
        gotestsum_path = Path(f"{os.getenv('GOPATH')}/bin/gotestsum")

        # Copy the binaries to the target directory, CI will take them from those
        # paths as artifacts
        copy_executables = {
            gotestsum_path: paths.dependencies / "go/bin/gotestsum",
            clang_path: paths.arch_dir / "opt/datadog-agent/embedded/bin/clang-bpf",
            llc_path: paths.arch_dir / "opt/datadog-agent/embedded/bin/llc-bpf",
        }

        for src, dst in copy_executables.items():
            ctx.run(f"install -D {src} {dst}")
    else:
        gotestsum_path = paths.dependencies / "go/bin/gotestsum"
        download_gotestsum(ctx, arch_obj, gotestsum_path)

        # We cannot use the pre-built local clang and llc-bpf binaries, as they
        # might not be built for the target architecture.
        llc_path = paths.tools / "llc-bpf"
        clang_path = paths.tools / "clang-bpf"
        setup_runtime_clang(ctx, arch_obj, paths.tools)

        # Later on, we will copy these binaries to the target VMs
        # We do not copy them to the same locations we do in CI, as here we want arch-specific paths

    if ci or compile_only:
        return

    if vms is None or vms == "":
        raise Exit("No vms specified to sync with")

    ssh_key_obj = try_get_ssh_key(ctx, ssh_key)
    infra = build_infrastructure(stack, ssh_key_obj)
    domains = filter_target_domains(vms, infra, arch_obj)

    info(f"[+] Preparing VMs {vms} in stack {stack} for {arch}")

    target_instances: list[HostInstance] = list()
    for d in domains:
        target_instances.append(d.instance)

    for d in domains:
        # Copy all test-specific dependencies to the target VM
        d.copy(ctx, paths.dependencies, "/opt/", verbose=verbose)

        # Copy embedded tools, make them
        embedded_remote_path = Path("/opt/datadog-agent/embedded/bin")
        d.copy(ctx, llc_path, embedded_remote_path / llc_path.name, verbose=verbose)
        d.copy(ctx, clang_path, embedded_remote_path / clang_path.name, verbose=verbose)

        # Copy all test files
        d.copy(ctx, paths.arch_dir / "opt/*", "/opt/", exclude="*.ninja", verbose=verbose)

        info(f"[+] Tests packages and dependencies setup in target VM {d}")


def build_run_config(run: str | None, packages: list[str]):
    c: dict[str, Any] = dict()

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

    return [pkg for pkg in all_packages if os.path.relpath(pkg) in filter_packages]


def build_object_files(ctx, fp, arch: Arch):
    info("[+] Generating eBPF object files...")
    ninja_generate(ctx, fp, arch=arch)
    ctx.run(f"ninja -d explain -f {fp}")


@task
def kmt_sysprobe_prepare(
    ctx: Context,
    arch: str | Arch,
    stack: str | None = None,
    kernel_release: str | None = None,
    packages=None,
    extra_arguments: str | None = None,
    ci: bool = False,
):
    if ci:
        stack = "ci"

    assert stack is not None, "A stack name must be provided"

    assert arch is not None and arch != "local", "No architecture provided"

    arch = Arch.from_str(arch)
    check_for_ninja(ctx)

    filter_pkgs = []
    if packages:
        filter_pkgs = [os.path.relpath(p) for p in packages.split(",")]

    target_packages = build_target_packages(filter_pkgs)
    kmt_paths = KMTPaths(stack, arch)
    nf_path = os.path.join(kmt_paths.arch_dir, "kmt-sysprobe.ninja")

    kmt_paths.arch_dir.mkdir(exist_ok=True, parents=True)
    kmt_paths.dependencies.mkdir(exist_ok=True, parents=True)

    go_path = "go"
    go_root = os.getenv("GOROOT")
    if go_root:
        go_path = os.path.join(go_root, "bin", "go")

    build_object_files(ctx, f"{kmt_paths.arch_dir}/kmt-object-files.ninja", arch)
    with open(nf_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file)

        _, _, env = get_build_flags(ctx, arch=arch)
        env["DD_SYSTEM_PROBE_BPF_DIR"] = EMBEDDED_SHARE_DIR

        env_str = ""
        for key, val in env.items():
            new_val = val.replace('\n', ' ')
            env_str += f"{key}='{new_val}' "
        env_str = env_str.rstrip()

        ninja_define_rules(nw)
        ninja_build_dependencies(ctx, nw, kmt_paths, go_path, arch)
        ninja_copy_ebpf_files(nw, "system-probe", kmt_paths, arch)

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

            go_files = [os.path.abspath(i) for i in glob(f"{pkg}/*.go")]

            # We delete the output file to force ninja to rebuild the testsuite everytime
            # because it cannot track go dependencies correctly.
            ctx.run(f"rm -f {output_path}")
            nw.build(
                inputs=[pkg],
                outputs=[output_path],
                implicit=go_files,
                rule="gotestsuite",
                pool="gobuild",
                variables=variables,
            )

            if pkg.endswith("java"):
                nw.build(
                    inputs=[os.path.join(pkg, "agent-usm.jar")],
                    outputs=[os.path.join(target_path, "agent-usm.jar")],
                    rule="copyfiles",
                )

        # handle testutils and testdata seperately since they are
        # shared across packages
        target_pkgs = build_target_packages([])
        for pkg in target_pkgs:
            target_path = os.path.join(kmt_paths.sysprobe_tests, os.path.relpath(pkg, os.getcwd()))

            testdata = os.path.join(pkg, "testdata")
            if os.path.exists(testdata):
                nw.build(inputs=[testdata], outputs=[os.path.join(target_path, "testdata")], rule="copyextra")

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
                            "env": env_str
                        },
                    )

    ctx.run(f"ninja -d explain -v -f {nf_path}")


def images_matching_ci(ctx, domains):
    platforms = get_platforms()
    arch = Arch.local().kmt_arch
    kmt_os = get_kmt_os()

    not_matches = list()
    for tag in platforms[arch]:
        platinfo = platforms[arch][tag]
        vmid = f"{platinfo['os_id']}_{platinfo['os_version']}"

        check_tag = False
        for d in domains:
            if vmid in d.name:
                check_tag = True
                break

        if not check_tag:
            continue

        manifest_file = '.'.join(platinfo["image"].split('.')[:-2]) + ".manifest"

        if not (kmt_os.rootfs_dir / manifest_file).exists():
            not_matches.append(platinfo["image"])
            continue

        with open(kmt_os.rootfs_dir / manifest_file) as f:
            for line in f:
                key, value = line.strip().split('=', 1)
                if key != "IMAGE_VERSION":
                    continue

                value = value.replace('"', '')
                if value != platinfo["image_version"]:
                    not_matches.append(platinfo["image"])

    for name in not_matches:
        warn(f"[-] {name} does not match version in CI")

    if len(not_matches) > 0:
        return False

    return True


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
    component: str = "system-probe",
    vms: str | None = None,
    stack: str | None = None,
    packages=None,
    run: str | None = None,
    quick=False,
    retry=2,
    run_count=1,
    ssh_key: str | None = None,
    verbose=True,
    test_logs=False,
    test_extra_arguments=None,
):
    stack = check_and_get_stack(stack)
    assert stacks.stack_exists(
        stack
    ), f"Stack {stack} does not exist. Please create with 'inv kmt.create-stack --stack=<name>'"

    if vms is None:
        vms = ",".join(stacks.get_all_vms_in_stack(stack))
        info(f"[+] Running tests on all VMs in stack {stack}: vms={vms}")

    ssh_key_obj = try_get_ssh_key(ctx, ssh_key)
    infra = build_infrastructure(stack, ssh_key_obj)
    domains = filter_target_domains(vms, infra)
    used_archs = get_archs_in_domains(domains)

    if not images_matching_ci(ctx, domains):
        if ask("Some VMs do not match version in CI. Continue anyway [y/N]") != "y":
            return

    assert len(domains) > 0, f"no vms found from list {vms}. Run `inv -e kmt.status` to see all VMs in current stack"

    info("[+] Detected architectures in target VMs: " + ", ".join(map(str, used_archs)))

    if not quick:
        for arch in used_archs:
            info(f"[+] Preparing {component} for {arch}")
            prepare(ctx, component, stack=stack, vms=vms, packages=packages, ssh_key=ssh_key, arch=arch)

    if run is not None and packages is None:
        raise Exit("Package must be provided when specifying test")

    pkgs = []
    if packages is not None:
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
            f"-test-root /opt/{component}-tests",
            f"-extra-params {test_extra_arguments}" if test_extra_arguments is not None else "",
            "-test-tools /opt/testing-tools",
        ]
        for d in domains:
            info(f"[+] Running tests on {d}")
            d.copy(ctx, f"{tmp.name}", remote_tmp)
            d.run_cmd(ctx, f"/opt/micro-vm-init.sh {' '.join(args)}", verbose=verbose)


def build_layout(ctx, domains, layout: str, verbose: bool):
    with open(layout) as lf:
        todo: DependenciesLayout = cast('DependenciesLayout', json.load(lf))

    for d in domains:
        mkdir = list()
        for dirs in todo["layout"]:
            mkdir.append(f"mkdir -p {dirs} &&")

        cmd = ' '.join(mkdir)
        d.run_cmd(ctx, cmd.rstrip('&'), verbose)

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
    vms: str | None = None,
    stack: str | None = None,
    ssh_key: str | None = None,
    verbose=True,
    arch: str | None = None,
    component: Component = "system-probe",
    layout: str = "tasks/kernel_matrix_testing/build-layout.json",
):
    stack = check_and_get_stack(stack)
    assert stacks.stack_exists(
        stack
    ), f"Stack {stack} does not exist. Please create with 'inv kmt.create-stack --stack=<name>'"

    if arch is None:
        arch = "local"

    if vms is None:
        vms = ",".join(stacks.get_all_vms_in_stack(stack))

    assert os.path.exists(layout), f"File {layout} does not exist"

    arch_obj = Arch.from_str(arch)
    paths = KMTPaths(stack, arch_obj)
    paths.arch_dir.mkdir(parents=True, exist_ok=True)

    ssh_key_obj = try_get_ssh_key(ctx, ssh_key)
    infra = build_infrastructure(stack, ssh_key_obj)
    domains = filter_target_domains(vms, infra, arch_obj)
    cc = get_compiler(ctx)

    if not images_matching_ci(ctx, domains):
        if ask("Some VMs do not match version in CI. Continue anyway [y/N]") != "y":
            return

    assert len(domains) > 0, f"no vms found from list {vms}. Run `inv -e kmt.status` to see all VMs in current stack"

    cc.exec(f"cd {CONTAINER_AGENT_PATH} && inv -e system-probe.object-files")

    build_task = "build-sysprobe-binary" if component == "system-probe" else "build"
    cc.exec(
        f"cd {CONTAINER_AGENT_PATH} && git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv -e {component}.{build_task} --no-bundle",
    )

    cc.exec(f"tar cf {CONTAINER_AGENT_PATH}/kmt-deps/{stack}/build-embedded-dir.tar {EMBEDDED_SHARE_DIR}")

    build_layout(ctx, domains, layout, verbose)
    for d in domains:
        d.copy(ctx, f"./bin/{component}", "/root/")
        d.copy(ctx, f"kmt-deps/{stack}/build-embedded-dir.tar", "/")
        d.run_cmd(ctx, "tar xf /build-embedded-dir.tar -C /", verbose=verbose)
        info(f"[+] {component} built for {d.name} @ /root")


@task
def clean(ctx: Context, stack: str | None = None, container=False, image=False):
    stack = check_and_get_stack(stack)
    assert stacks.stack_exists(
        stack
    ), f"Stack {stack} does not exist. Please create with 'inv kmt.create-stack --stack=<name>'"

    cc = get_compiler(ctx)
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
    stacks: str | None = None,
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
                for key, value in SSH_OPTIONS.items():
                    print(f"    {key} {value}")
                print("")

            multiple_instances_with_same_tag = len({i.tag for i in instance.microvms}) != len(instance.microvms)

            for domain in instance.microvms:
                domain_name = domain.tag
                if multiple_instances_with_same_tag:
                    id_parts = domain.name.split('-')
                    mem = id_parts[-1]
                    cpu = id_parts[-2]
                    domain_name += f"-mem{mem}-cpu{cpu}"

                print(f"Host kmt-{stack_name}-{instance.arch}-{domain_name}")
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
def status(ctx: Context, stack: str | None = None, all=False, ssh_key: str | None = None):
    stacks: list[str]

    if all:
        stacks = [stack.name for stack in Path(get_kmt_os().stacks_dir).iterdir() if stack.is_dir()]
    else:
        stacks = [check_and_get_stack(stack)]

    # Dict of status lines for each stack
    status: dict[str, list[str]] = defaultdict(list)
    stack_status: dict[str, tuple[int, int, int, int]] = {}
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


@task(
    help={
        "version": "The version to update the images to. If not provided, version will not be changed. If 'latest' is provided, the latest version will be used.",
        "update-only-matching": "Only update the platform info for images that match the given regex",
        "exclude-matching": "Exclude images that match the given regex",
    }
)
def update_platform_info(
    ctx: Context,
    version: str | None = None,
    update_only_matching: str | None = None,
    exclude_matching: str | None = None,
):
    """Generate a JSON file with platform information for all the images
    found in the KMT S3 bucket.
    """
    res = ctx.run(
        "aws-vault exec sso-staging-engineering -- aws s3 ls --recursive s3://dd-agent-omnibus/kernel-version-testing/rootfs",
        warn=True,
    )
    if res is None or not res.ok:
        raise Exit("Cannot list bucket contents")

    objects = [line.split()[-1] for line in res.stdout.splitlines()]
    objects_by_version: dict[str, list[str]] = defaultdict(list)

    for obj in objects:
        v = "/".join(obj.split("/")[2:-1])
        if v != "":
            objects_by_version[v].append(obj)

    if version is None:
        master_versions = [v for v in objects_by_version if re.match(r"^20[0-9]{6}_[0-9a-f]+$", v)]
        if len(master_versions) == 0:
            raise Exit("No master versions available")

        version = sorted(master_versions)[-1]
        info(f"[+] detected {version} as latest version from master branch")

    if version not in objects_by_version:
        raise Exit(f"Version {version} not found in S3 bucket, cannot update")

    manifests = [obj for obj in objects_by_version[version] if obj.endswith(".manifest")]
    platforms = get_platforms()

    with tempfile.TemporaryDirectory() as tmpdir:
        for manifest in manifests:
            info(f"[+] Processing manifest {manifest}")
            ctx.run(f"aws-vault exec sso-staging-engineering -- aws s3 cp s3://dd-agent-omnibus/{manifest} {tmpdir}")
            with open(f"{tmpdir}/{os.path.basename(manifest)}") as f:
                options = f.readlines()
                keyvals = {line.split("=")[0]: line.split("=")[1].strip().strip('"') for line in options}

            try:
                arch = Arch.from_str(keyvals['ARCH'])
                image_name = keyvals['IMAGE_NAME']
                image_filename = keyvals['IMAGE_FILENAME']
            except KeyError:
                raise Exit(f"[!] Invalid manifest {manifest}")

            if arch not in platforms:
                warn(f"[!] Unsupported architecture {arch}, skipping")
                continue

            if update_only_matching is not None and re.search(update_only_matching, image_name) is None:
                warn(f"[!] Image {image_name} does not match the filter, skipping")
                continue

            if exclude_matching is not None and re.search(exclude_matching, image_name) is not None:
                warn(f"[!] Image {image_name} matches the exclude filter, skipping")
                continue

            manifest_to_platinfo_keys = {
                'NAME': 'os_name',
                'ID': 'os_id',
                'KERNEL_VERSION': 'kernel',
                'VERSION_ID': 'os_version',
            }

            if image_name not in platforms[arch.kmt_arch]:
                platforms[arch.kmt_arch][image_name] = {}
            img_data = platforms[arch.kmt_arch][image_name]

            for mkey, pkey in manifest_to_platinfo_keys.items():
                if mkey in keyvals:
                    img_data[pkey] = keyvals[mkey]

            img_data['image'] = image_filename + ".xz"
            img_data['image_version'] = version

            if 'VERSION_CODENAME' in keyvals:
                altname = keyvals['VERSION_CODENAME']
                # Do not modify existing altnames
                altnames = img_data.get('alt_version_names', [])
                if altname not in altnames:
                    altnames.append(altname)

                img_data['alt_version_names'] = altnames

    info(f"[+] Writing output to {platforms_file}...")

    # Do validation of the platforms dict, check that there are no outdated versions
    for kmt_arch in KMT_SUPPORTED_ARCHS:
        for image_name, platinfo in platforms[kmt_arch].items():
            if update_only_matching is not None and re.search(update_only_matching, image_name) is None:
                continue  # Only validate those that match

            if exclude_matching is not None and re.search(exclude_matching, image_name) is not None:
                continue

            version_from_file = platinfo.get('image_version')
            if version_from_file != version:
                warn(
                    f"[!] Image {image_name} ({kmt_arch}) has version {version_from_file} but we are updating to {version}, manifest file may be missing?"
                )

    with open(platforms_file, "w") as f:
        json.dump(platforms, f, indent=2)


@task
def validate_platform_info(ctx: Context):
    """Validate the platform info file for correctness, ensuring that all images can be found"""
    platforms = get_platforms()
    errors: set[str] = set()

    for arch in KMT_SUPPORTED_ARCHS:
        for image_name, platinfo in platforms[arch].items():
            image = platinfo.get('image')
            if image is None:
                warn(f"[!] {image_name} does not have an image filename")
                errors.add(image_name)
                continue

            version = platinfo.get('image_version')
            if version is None:
                warn(f"[!] {image_name} does not have an image version")
                errors.add(image_name)
                continue

            remote = f"{platforms['url_base']}/{version}/{image}"
            res = ctx.run(f"curl -s -I {remote}")
            if res is None or not res.ok:
                warn(f"[!] {image_name}: {image} for version {version} not found at {remote}")
                errors.add(image_name)
            else:
                info(f"[+] {image_name}: {image} for version {version} found at {remote}")

    if len(errors) == 0:
        info("[+] Platform info file is valid")
    else:
        raise Exit(f"[!] Found {len(errors)} errors in the platform info file. Images failed: {', '.join(errors)}")


@task
def explain_ci_failure(_, pipeline: str):
    """Show a summary of KMT failures in the given pipeline."""
    if tabulate is None:
        raise Exit("tabulate module is not installed, please install it to continue")

    info(f"[+] retrieving all CI jobs for pipeline {pipeline}")
    setup_jobs, test_jobs = get_all_jobs_for_pipeline(pipeline)

    failed_setup_jobs = [j for j in setup_jobs if j.status == "failed"]
    failed_jobs = [j for j in test_jobs if j.status == "failed"]
    failreasons: dict[str, str] = {}
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
            for artifact in job.job.artifacts:
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

    def groupby_comp_vmset(job: KMTTestRunJob) -> tuple[str, str]:
        return (job.component, job.vmset)

    # Show first a matrix of failed distros and archs for each tuple of component and vmset
    jobs_by_comp_and_vmset = itertools.groupby(sorted(failed_jobs, key=groupby_comp_vmset), groupby_comp_vmset)
    for (component, vmset), group_jobs in jobs_by_comp_and_vmset:
        group_jobs = list(group_jobs)  # Consume the iterator, make a copy
        distros: dict[str, dict[KMTArchName, str]] = defaultdict(lambda: {"x86_64": " ", "arm64": " "})
        distro_arch_with_test_failures: list[tuple[str, KMTArchName]] = []

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
        test_failure_table: list[list[str]] = []

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

    def groupby_arch_comp(job: KMTTestRunJob) -> tuple[str, str]:
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

        infra_fail_table: list[list[str]] = []
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


@task()
def tmux(ctx: Context, stack: str | None = None):
    """Create a tmux session with panes for each VM in the stack.

    Note that this task requires the tmux command to be available on the system, and the SSH
    config to have been generated with the kmt.ssh-config task.
    """
    stack = check_and_get_stack(stack)
    stack_name = stack.replace('-ddvm', '')

    ctx.run(f"tmux kill-session -t kmt-{stack_name} || true")
    ctx.run(f"tmux new-session -d -s kmt-{stack_name}")

    for i, (_, instance) in enumerate(build_infrastructure(stack, try_get_ssh_key(ctx, None)).items()):
        window_name = instance.arch
        if i == 0:
            ctx.run(f"tmux rename-window -t kmt-{stack_name} {window_name}")
        else:
            ctx.run(f"tmux new-window -t kmt-{stack_name} -n {window_name}")

        multiple_instances_with_same_tag = len({i.tag for i in instance.microvms}) != len(instance.microvms)

        needs_split = False
        for domain in instance.microvms:
            domain_name = domain.tag
            if multiple_instances_with_same_tag:
                id_parts = domain.name.split('-')
                mem = id_parts[-1]
                cpu = id_parts[-2]
                domain_name += f"-mem{mem}-cpu{cpu}"
            ssh_name = f"kmt-{stack_name}-{instance.arch}-{domain_name}"

            if needs_split:
                ctx.run(f"tmux split-window -h -t kmt-{stack_name}:{i}")
            needs_split = True

            ctx.run(f"tmux send-keys -t kmt-{stack_name}:{i} 'ssh {ssh_name}' Enter")
            ctx.run(f"tmux select-layout -t kmt-{stack_name}:{i} tiled")

    info(f"[+] Tmux session kmt-{stack_name} created. Attach with 'tmux attach -t kmt-{stack_name}'")


@task(
    help={
        "allow_infra_changes": "Allow infrastructure changes to be made during the selftest",
        "filter": "Filter to run only tests matching the given regex",
    }
)
def selftest(ctx: Context, allow_infra_changes=False, filter: str | None = None):
    """Run all KMT selftests, reporting status at the end. Can be used for debugging in KMT development
    or for troubleshooting.
    """
    selftests.selftest(ctx, allow_infra_changes=allow_infra_changes, filter=filter)

from __future__ import annotations

import io
import itertools
import json
import os
import re
import shutil
import sys
import tarfile
import tempfile
import xml.etree.ElementTree as ET
from collections import defaultdict
from collections.abc import Callable, Iterable
from glob import glob
from pathlib import Path
from typing import TYPE_CHECKING, Any, cast

import yaml
from invoke.context import Context
from invoke.tasks import task

from tasks.kernel_matrix_testing import selftest as selftests
from tasks.kernel_matrix_testing import stacks, vmconfig
from tasks.kernel_matrix_testing.ci import KMTTestRunJob, get_all_jobs_for_pipeline, get_test_results_from_tarfile
from tasks.kernel_matrix_testing.compiler import CONTAINER_AGENT_PATH, get_compiler
from tasks.kernel_matrix_testing.config import ConfigManager
from tasks.kernel_matrix_testing.download import update_rootfs
from tasks.kernel_matrix_testing.infra import (
    SSH_OPTIONS,
    HostInstance,
    LibvirtDomain,
    build_alien_infrastructure,
    build_infrastructure,
    ensure_key_in_ec2,
    get_ssh_agent_key_names,
    get_ssh_key_name,
    try_get_ssh_key,
)
from tasks.kernel_matrix_testing.init_kmt import init_kernel_matrix_testing_system
from tasks.kernel_matrix_testing.kmt_os import flare as flare_kmt_os
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.platforms import get_platforms, platforms_file
from tasks.kernel_matrix_testing.stacks import check_and_get_stack, ec2_instance_ids
from tasks.kernel_matrix_testing.tool import Exit, ask, error, get_binary_target_arch, info, warn
from tasks.kernel_matrix_testing.vars import KMT_SUPPORTED_ARCHS, KMTPaths
from tasks.libs.build.ninja import NinjaWriter
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.git import get_current_branch
from tasks.libs.common.utils import get_build_flags
from tasks.libs.pipeline.tools import loop_status
from tasks.libs.releasing.version import VERSION_RE, check_version
from tasks.libs.types.arch import Arch, KMTArchName
from tasks.security_agent import build_functional_tests, build_stress_tests
from tasks.system_probe import (
    BPF_TAG,
    EMBEDDED_SHARE_DIR,
    NPM_TAG,
    TEST_HELPER_CBINS,
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
    from tasks.kernel_matrix_testing.types import (
        Component,  # noqa: F401
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
        "ci": "Generate a vmconfig.json file for the KMT CI, that is, with all available VMs for the given architecture.",
        "arch": "(Only when --ci is used) Architecture to select when generating the vmconfig for all posible VMs.",
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
    if not ci and arch != "":
        # The argument is not used later on, so better notify the user early to avoid confusion
        raise Exit(
            "Error: Architecture (--arch argument) can only be specified when generating from a CI pipeline (--ci argument). "
            "To specify the architecture of the VMs, use the VM specifier (e.g., x64-ubuntu_22-distro or local-ubuntu_22-distro for local VMs)"
        )

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

        if use_local_if_possible:
            raise Exit(
                "--use-local-if-possible can only be used with --from-ci-pipeline. If you want to set up local VMs, use the local specifier in the VM list (e.g., ubuntu_22-distro-local instead of ubuntu_22-distro-arm64)"
            )

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
    failed_tests: set[str] = set()
    successful_tests: set[str] = set()
    for test_job in test_jobs:
        if test_job.status == "failed" and job.component == vmconfig_template:
            vm_arch = test_job.arch
            if use_local_if_possible and vm_arch == local_arch:
                vm_arch = local_arch

            results = test_job.get_test_results()
            for test, result in results.items():
                if result is False:
                    package, test = test.split(":", maxsplit=1)
                    failed_tests.add(test)
                    failed_packages.add(
                        f"./{package}"
                    )  # Use relative path to the package so the suggestions for kmt.test work correctly
                elif result is True:  # It can also be None if the test was skipped
                    successful_tests.add(test)

            vm_name = f"{vm_arch}-{test_job.distro}-distro"
            info(f"[+] Adding {vm_name} from failed job {test_job.name}")
            vms.add(vm_name)

    # Simplify the failed tests so that we show only the parent tests with all failures below
    # and not all child tests that failed
    # Not at all the most efficient way to do this, but it works for the amount of data we have
    # and is simple enough
    successful_tests = successful_tests.difference(failed_tests)
    coalesced_failed_tests: set[str] = set()
    non_coalesced_failed_tests: set[str] = set()
    for test in sorted(failed_tests):  # Sort to have parent tests first
        is_included = False

        # Check if this test is already included in some parent test
        for already_coalesced in coalesced_failed_tests:
            if test.startswith(already_coalesced):
                is_included = True
                break
        else:
            # If not, check if there is a subtest that succeeded. If there is not,
            # we assume all children tests of this one failed and we can coalesce them
            # into a single one
            for succesful_test in successful_tests:
                if succesful_test.startswith(test):
                    # There was a subtest of this one that succeeded, we cannot coalesce
                    # Add it to the non-coalesced list so that it's not checked as a parent
                    # and its children will be checked again
                    non_coalesced_failed_tests.add(test)
                    is_included = True
                    break

        if not is_included:
            coalesced_failed_tests.add(test)

    failed_tests = non_coalesced_failed_tests | {f"{t}/.*" for t in coalesced_failed_tests}

    if len(vms) == 0:
        raise Exit(f"No failed jobs found in pipeline {pipeline}")

    info(f"[+] generating {output_file} file for VMs {vms}")
    vcpu = DEFAULT_VCPU if vcpu is None else vcpu
    memory = DEFAULT_MEMORY if memory is None else memory
    vmconfig.gen_config(
        ctx, stack, ",".join(vms), "", init_stack, vcpu, memory, new, ci, arch, output_file, vmconfig_template, yes=yes
    )
    info("[+] You can run the following command to execute only packages with failed tests")
    print(f"inv kmt.test --packages=\"{','.join(failed_packages)}\" --run='^{'|'.join(failed_tests)}$'")


@task
def launch_stack(
    ctx: Context,
    stack: str | None = None,
    ssh_key: str | None = None,
    x86_ami: str = X86_AMI_ID_SANDBOX,
    arm_ami: str = ARM_AMI_ID_SANDBOX,
    provision_microvms: bool = True,
    provision_script: str | None = None,
):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.create-stack --stack=<name>'")

    stacks.launch_stack(ctx, stack, ssh_key, x86_ami, arm_ami, provision_microvms)
    if provision_script is not None:
        provision_stack(ctx, provision_script, stack, ssh_key)


@task
def provision_stack(
    ctx: Context,
    provision_script: str,
    stack: str | None = None,
    ssh_key: str | None = None,
):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.create-stack --stack=<name>'")

    ssh_key_obj = try_get_ssh_key(ctx, ssh_key)
    infra = build_infrastructure(stack, ssh_key_obj)
    for arch in infra:
        for domain in infra[arch].microvms:
            domain.copy(ctx, provision_script, "/tmp/provision.sh")
            domain.run_cmd(ctx, "chmod +x /tmp/provision.sh && /tmp/provision.sh", verbose=True)


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
    try:
        init_kernel_matrix_testing_system(ctx, lite, images)
    except Exception as e:
        error(f"[-] Error initializing kernel matrix testing system: {e}")
        raise e

    info("[+] Kernel matrix testing system initialized successfully")
    config_ssh_key(ctx)


@task
def config_ssh_key(ctx: Context):
    """Automatically configure the default SSH key to use"""
    info("[+] Configuring SSH key for use with the KMT AWS instances")
    info(
        "[+] Ensure your desired SSH key is set up in the AWS sandbox account (not agent-sandbox) so we can check its existence"
    )
    info("[+] Reminder that key pairs for AWS are configured in AWS > EC2 > Key Pairs")
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
                key_name = get_ssh_key_name(f.with_suffix(".pub")) or f.name
                ssh_keys.append({'path': os.fspath(f), 'name': key_name, 'aws_key_name': ''})

        keys_str = "\n".join([f" - [{i + 1}] {key['name']} (path: {key['path']})" for i, key in enumerate(ssh_keys)])
        result = ask(f"[?] Found these valid key files:\n{keys_str}\nChoose one of these files (1-{len(ssh_keys)}): ")
        try:
            ssh_key = ssh_keys[int(result.strip()) - 1]
        except ValueError as e:
            raise Exit(f"Choice {result} is not a valid number") from e
        except IndexError as e:  # out of range
            raise Exit(f"Invalid choice {result}, must be a number between 1 and {len(ssh_keys)} (inclusive)") from e

        info("[+] KMT needs this SSH key to be loaded in AWS so that it can be used to access the instances")
        info(
            "[+] If you haven't loaded it yet, go to https://dtdg.co/aws-sso-prod -> DataDog Sandbox -> EC2 -> Network & Security -> Key Pairs"
        )
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
        "images": "Comma separated list of images to update, instead of everything. The format of each image can be 'image_name', 'OSId-OSVersion', or 'Alternative name' (resp. examples, debian_11, amzn-2023, mantic). Refer to the output of kmt.ls for the appropriate values",
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
    try:
        cc.start()
    except Exception as e:
        error(f"[-] Error starting compiler {cc.name}: {e}")


def filter_target_domains(vms: str, infra: dict[KMTArchNameOrLocal, HostInstance], arch: Arch | None = None):
    vmsets = vmconfig.build_vmsets(vmconfig.build_normalized_vm_def_by_set(vms, []))
    domains: list[LibvirtDomain] = []
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
        archs.add(Arch.from_str(d.arch))
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

    nw.rule(
        name="cbin",
        command="$cc $cflags -o $out $in $ldflags",
    )


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

    vm_metrics_files = glob("test/new-e2e/system-probe/vm-metrics/*.go")
    nw.build(
        rule="gobin",
        pool="gobuild",
        outputs=[os.path.join(kmt_paths.dependencies, "vm-metrics")],
        implicit=vm_metrics_files,
        variables={
            "go": go_path,
            "chdir": "cd test/new-e2e/system-probe/vm-metrics",
            "env": env_str,
        },
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

    verifier_files = glob("pkg/ebpf/verifier/**/*.go")
    nw.build(
        rule="gobin",
        pool="gobuild",
        inputs=["./pkg/ebpf/verifier/calculator/main.go"],
        outputs=[os.fspath(kmt_paths.dependencies / "verifier-calculator")],
        implicit=verifier_files,
        variables={
            "go": go_path,
            "chdir": "true",
            "env": env_str,
            "tags": f"-tags=\"{','.join(get_sysprobe_buildtags(False, False))}\"",
        },
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
    stack: str | None = None,
    arch: Arch | str = "local",
    packages: str | None = None,
    verbose: bool = True,
    ci: bool = True,
    compile_only: bool = False,
):
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
    alien_vms: str | None = None,
    stack: str | None = None,
    arch: str | Arch = "local",
    ssh_key: str | None = None,
    packages=None,
    verbose=True,
    ci=False,
    compile_only=False,
):
    arch_obj = Arch.from_str(arch)
    if arch_obj.kmt_arch not in KMT_SUPPORTED_ARCHS:
        raise Exit(
            f"Architecture {arch} (inferred {arch_obj}) is not supported. Supported architectures are amd64 and arm64"
        )

    if ci:
        domains = None
        stack = "ci"
        return _prepare(ctx, stack, component, arch_obj, packages, verbose, ci, compile_only)

    if alien_vms is not None:
        err_msg = f"no alient VMs discovered from provided profile {alien_vms}."
    else:
        err_msg = f"no vms found from list {vms}. Run `inv -e kmt.status` to see all VMs in current stack"
    stack = get_kmt_or_alien_stack(ctx, stack, vms, alien_vms)
    domains = get_target_domains(ctx, stack, ssh_key, arch_obj, vms, alien_vms)
    assert len(domains) > 0, err_msg

    _prepare(ctx, stack, component, arch_obj, packages, verbose, ci, compile_only, domains=domains)


def _prepare(
    ctx: Context,
    stack: str,
    component: Component,
    arch_obj: Arch,
    packages=None,
    verbose=True,
    ci=False,
    compile_only=False,
    domains: list[LibvirtDomain] | None = None,
):
    if not ci:
        cc = get_compiler(ctx)

        if arch_obj.is_cross_compiling():
            cc.ensure_ready_for_cross_compile()

    pkgs = ""
    if packages:
        pkgs = f"--packages {packages}"

    inv_echo = "-e" if ctx.config.run["echo"] else ""

    info(f"[+] Compiling artifacts for {arch_obj}, component = {component}")
    if component == "security-agent":
        if ci:
            kmt_secagent_prepare(ctx, stack, arch_obj, packages, verbose, ci)
        else:
            cc.exec(
                f"git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv {inv_echo} kmt.kmt-secagent-prepare --stack={stack} {pkgs} --arch={arch_obj.name}",
                run_dir=CONTAINER_AGENT_PATH,
            )
    elif component == "system-probe":
        if ci:
            kmt_sysprobe_prepare(ctx, arch_obj, ci=True)
        else:
            cc.exec(
                f"git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv {inv_echo} kmt.kmt-sysprobe-prepare --stack={stack} {pkgs} --arch={arch_obj.name}",
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
        copy_static_files = {
            gotestsum_path: paths.dependencies / "go/bin/gotestsum",
            clang_path: paths.arch_dir / "opt/datadog-agent/embedded/bin/clang-bpf",
            llc_path: paths.arch_dir / "opt/datadog-agent/embedded/bin/llc-bpf",
            "flakes.yaml": paths.dependencies / "flakes.yaml",
            ".github/CODEOWNERS": paths.dependencies / "CODEOWNERS",
        }

        for src, dst in copy_static_files.items():
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

    info(f"[+] Preparing VMs in stack {stack} for {arch_obj}")

    target_instances: list[HostInstance] = []
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

        # Copy BTF files
        btf_dir = Path(f"/opt/{component}-tests") / get_ebpf_build_dir(arch_obj) / "co-re/btf"
        d.run_cmd(
            ctx,
            f"[ -f /sys/kernel/btf/vmlinux ] \
                || [ -f {btf_dir}/minimized-btfs.tar.xz ] \
                || ([ -d /opt/btf ] \
                && cd /opt/btf/ \
                && tar cJf minimized-btfs.tar.xz * \
                && mkdir -p {btf_dir} \
                && mv /opt/btf/minimized-btfs.tar.xz {btf_dir}/)",
            verbose=verbose,
        )

        info(f"[+] Tests packages and dependencies setup in target VM {d}")


def build_run_config(run: str | None, packages: list[str]):
    c: dict[str, Any] = {}

    if len(packages) == 0:
        return {"filters": {"*": {"exclude": False}}}

    for p in packages:
        if p[:2] == "./":
            p = p[2:]
        if run is not None:
            c["filters"] = {p: {"run-only": [run]}}
        else:
            c["filters"] = {p: {"exclude": False}}

    return c


def build_target_packages(filter_packages: list[str], build_tags: list[str]):
    all_packages = go_package_dirs(TEST_PACKAGES_LIST, build_tags)
    if not filter_packages:
        return all_packages

    filter_packages = [os.path.relpath(p) for p in go_package_dirs(filter_packages, build_tags)]
    return [pkg for pkg in all_packages if os.path.relpath(pkg) in filter_packages]


def build_object_files(ctx, fp, arch: Arch):
    info("[+] Generating eBPF object files...")
    ninja_generate(ctx, fp, arch=arch)
    ctx.run(f"ninja -d explain -f {fp}")


def compute_package_dependencies(ctx: Context, packages: list[str], build_tags: list[str]) -> dict[str, set[str]]:
    dd_pkg_name = "github.com/DataDog/datadog-agent/"
    pkg_deps: dict[str, set[str]] = defaultdict(set)

    packages_list = " ".join(packages)
    list_format = "{{ .ImportPath }}: {{ join .Deps \" \" }}"
    res = ctx.run(f"go list -test -f '{list_format}' -tags \"{build_tags}\" {packages_list}", hide=True)
    if res is None or not res.ok:
        raise Exit("Failed to get dependencies for system-probe")

    for line in res.stdout.split("\n"):
        if ":" not in line:
            continue

        pkg, deps = line.split(":", 1)
        deps = [d.strip() for d in deps.split(" ")]
        dd_deps = [d[len(dd_pkg_name) :] for d in deps if d.startswith(dd_pkg_name)]

        # The import path printed by "go list" is usually path/to/pkg  (e.g., pkg/ebpf/verifier).
        # However, for test packages it might be either:
        # - path/to/pkg.test
        # - path/to/pkg [path/to/pkg.test]
        # In any case all variants refer to the same variant. This code controls for that
        # so that we keep the usual package name.
        pkg = pkg.split(" ")[0].removeprefix(dd_pkg_name).removesuffix(".test")
        pkg_deps[pkg].update(dd_deps)

    return pkg_deps


@task
def kmt_sysprobe_prepare(
    ctx: Context,
    arch: str | Arch,
    stack: str | None = None,
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
        filter_pkgs = packages.split(",")

    kmt_paths = KMTPaths(stack, arch)
    nf_path = os.path.join(kmt_paths.arch_dir, "kmt-sysprobe.ninja")

    kmt_paths.arch_dir.mkdir(exist_ok=True, parents=True)
    kmt_paths.dependencies.mkdir(exist_ok=True, parents=True)

    go_path = "go"
    go_root = os.getenv("GOROOT")
    if go_root:
        go_path = os.path.join(go_root, "bin", "go")

    build_object_files(ctx, f"{kmt_paths.arch_dir}/kmt-object-files.ninja", arch)

    info("[+] Computing Go dependencies for test packages...")
    build_tags = get_sysprobe_buildtags(False, False)
    target_packages = build_target_packages(filter_pkgs, build_tags)
    pkg_deps = compute_package_dependencies(ctx, target_packages, build_tags)

    info("[+] Generating build instructions..")
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

        build_tags = get_sysprobe_buildtags(False, False)
        for pkg in target_packages:
            pkg_name = os.path.relpath(pkg, os.getcwd())
            target_path = os.path.join(kmt_paths.sysprobe_tests, pkg_name)
            output_path = os.path.join(target_path, "testsuite")
            variables = {
                "env": env_str,
                "go": go_path,
                "build_tags": build_tags,
            }
            timeout = get_test_timeout(os.path.relpath(pkg, os.getcwd()))
            if timeout:
                variables["timeout"] = f"-timeout {timeout}"
            if extra_arguments:
                variables["extra_arguments"] = extra_arguments

            go_files = set(glob(f"{pkg}/*.go"))
            has_test_files = any(x.lower().endswith("_test.go") for x in go_files)

            # skip packages without test files
            if has_test_files:
                for deps in pkg_deps[pkg_name]:
                    go_files.update(os.path.abspath(p) for p in glob(f"{deps}/*.go"))

                nw.build(
                    inputs=[pkg],
                    outputs=[output_path],
                    implicit=list(go_files),
                    rule="gotestsuite",
                    pool="gobuild",
                    variables=variables,
                )

        # handle testutils and testdata separately since they are
        # shared across packages
        target_pkgs = build_target_packages([], build_tags)
        for pkg in target_pkgs:
            target_path = os.path.join(kmt_paths.sysprobe_tests, os.path.relpath(pkg, os.getcwd()))

            testdata = os.path.join(pkg, "testdata")
            if os.path.exists(testdata):
                nw.build(inputs=[testdata], outputs=[os.path.join(target_path, "testdata")], rule="copyextra")

            for gobin in [
                "gotls_client",
                "gotls_server",
                "grpc_external_server",
                "external_unix_proxy_server",
                "fmapper",
                "prefetch_file",
                "fake_server",
                "sample_service",
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
                            "env": env_str,
                        },
                    )

            for cbin in TEST_HELPER_CBINS:
                source = Path(pkg) / "testdata" / f"{cbin}.c"
                if source.is_file():
                    testdata_folder = os.path.join(target_path, "testdata")
                    binary_path = os.path.join(testdata_folder, cbin)
                    nw.build(
                        inputs=[os.fspath(source)],
                        outputs=[binary_path],
                        # Ensure that the testdata folder is created before the
                        # binary, to avoid races between this command and the
                        # copy command
                        implicit=[testdata_folder],
                        rule="cbin",
                        variables={
                            "cc": "clang",
                        },
                    )

    info("[+] Compiling tests...")
    ctx.run(f"ninja -d explain -v -f {nf_path}")


def images_matching_ci(_: Context, domains: list[LibvirtDomain]):
    platforms = get_platforms()
    arch = Arch.local().kmt_arch
    kmt_os = get_kmt_os()

    not_matches = []
    for tag in platforms[arch]:
        platinfo = platforms[arch][tag]
        vmid = f"{platinfo['os_id']}_{platinfo['os_version']}"

        check_tag = False
        for d in domains:
            if vmid in d.name and d.instance.arch == "local":
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

    return len(not_matches) == 0


def get_target_domains(ctx, stack, ssh_key, arch_obj, vms, alien_vms) -> list[LibvirtDomain]:
    def _get_infrastructure(ctx, stack, ssh_key, vms, alien_vms):
        if alien_vms:
            alien_vms_path = Path(alien_vms)
            if not alien_vms_path.exists():
                raise Exit(f"No alien VMs profile found @ {alien_vms_path}")
            return build_alien_infrastructure(alien_vms_path)

        ssh_key_obj = try_get_ssh_key(ctx, ssh_key)
        return build_infrastructure(stack, ssh_key_obj)

    if vms is None and alien_vms is None:
        vms = ",".join(stacks.get_all_vms_in_stack(stack))
        info(f"[+] running tests on all vms in stack {stack}: vms={vms}")

    infra = _get_infrastructure(ctx, stack, ssh_key, vms, alien_vms)
    if alien_vms is not None:
        return infra["local"].microvms

    domains = filter_target_domains(vms, infra, arch_obj)
    if not images_matching_ci(ctx, domains):
        if ask("Some VMs do not match version in CI. Continue anyway [y/N]") != "y":
            raise Exit("[-] Aborting due to version mismatch")

    return domains


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
        "test-extra-env": "Extra environment variables to pass to the test runner",
    }
)
def test(
    ctx: Context,
    component: str = "system-probe",
    vms: str | None = None,
    alien_vms: str | None = None,
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
    test_extra_env=None,
):
    stack = get_kmt_or_alien_stack(ctx, stack, vms, alien_vms)
    domains = get_target_domains(ctx, stack, ssh_key, None, vms, alien_vms)
    used_archs = get_archs_in_domains(domains)

    if alien_vms is not None:
        err_msg = f"no alient VMs discovered from provided profile {alien_vms}."
    else:
        err_msg = f"no vms found from list {vms}. Run `inv -e kmt.status` to see all VMs in current stack"

    assert len(domains) > 0, err_msg

    info("[+] Detected architectures in target VMs: " + ", ".join(map(str, used_archs)))

    if not quick:
        for arch in used_archs:
            info(f"[+] Preparing {component} for {arch}")
            _prepare(ctx, stack, component, arch, packages=packages, verbose=verbose, domains=domains)

    if run is not None and packages is None:
        raise Exit("Package must be provided when specifying test")

    pkgs = []
    if packages is not None:
        pkgs = [os.path.relpath(os.path.realpath(p)) for p in go_package_dirs(packages.split(","), [NPM_TAG, BPF_TAG])]

    if run is not None and len(pkgs) > 1:
        raise Exit("Only a single package can be specified when running specific tests")

    paths = KMTPaths(stack, Arch.local())  # Arch is not relevant to the test result paths, which is what we want now
    shutil.rmtree(paths.test_results, ignore_errors=True)  # Reset test-results folder

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
            f"-extra-env {test_extra_env}" if test_extra_env is not None else "",
            "-test-tools /opt/testing-tools",
        ]
        for d in domains:
            info(f"[+] Running tests on {d}")
            d.copy(ctx, f"{tmp.name}", remote_tmp)
            d.run_cmd(ctx, f"/opt/micro-vm-init.sh {' '.join(args)}", verbose=verbose, allow_fail=True)

            info(f"[+] Showing summary of results for {d}")
            d.run_cmd(ctx, "/opt/testing-tools/test-json-review", verbose=verbose, allow_fail=True)

            info(f"[+] Tests completed on {d}, downloading results...")
            target_folder = paths.vm_test_results(d.name)
            target_folder.mkdir(parents=True, exist_ok=True)
            d.download(ctx, "/ci-visibility/junit/", target_folder)

    info("[+] All domains finished, showing summary table of test results")
    show_last_test_results(ctx, stack)


def build_layout(ctx, domains, layout: str, verbose: bool):
    with open(layout) as lf:
        todo: DependenciesLayout = cast('DependenciesLayout', json.load(lf))

    for d in domains:
        info(f"[+] apply layout to vm {d.name}")

        cmd = ' && '.join(f'mkdir -p {dirs}' for dirs in todo["layout"])
        if len(cmd) > 0:
            d.run_cmd(ctx, cmd.rstrip('&'), verbose)

        for src, dst in todo["copy"].items():
            if not os.path.exists(src):
                raise Exit(f"File {src} specified in {layout} does not exist")

            d.copy(ctx, src, dst)

        for cmd in todo["run"]:
            d.run_cmd(ctx, cmd, verbose)


def get_kmt_or_alien_stack(ctx, stack, vms, alien_vms):
    assert not (vms is not None and alien_vms is not None), "target VMs can be either KMT VMs or alien VMs, not both"

    if alien_vms is not None and vms is None:
        stack = check_and_get_stack("alien-stack")
        if not stacks.stack_exists(stack):
            stacks.create_stack(ctx, stack)
        return stack

    stack = check_and_get_stack(stack)
    assert stacks.stack_exists(
        stack
    ), f"Stack {stack} does not exist. Please create with 'inv kmt.create-stack --stack=<name>'"
    return stack


@task(
    help={
        "vms": "Comma seperated list of vms to target when running tests",
        "stack": "Stack in which the VMs exist. If not provided stack is autogenerated based on branch name",
        "ssh-key": "SSH key to use for connecting to a remote EC2 instance hosting the target VM. Can be either a name of a file in ~/.ssh, a key name (the comment in the public key) or a full path",
        "verbose": "Enable full output of all commands executed",
        "arch": "Architecture to build the system-probe for",
        "layout": "Path to file specifying the expected layout on the target VMs",
        "override_agent": "Assume that the datadog-agent has been installed with `kmt.install-ddagent`, and replace the system-probe binary in its package with a local build. This also overrides the configuration files as defined in tasks/kernel-matrix-testing/build-layout.json",
    }
)
def build(
    ctx: Context,
    vms: str | None = None,
    alien_vms: str | None = None,
    stack: str | None = None,
    ssh_key: str | None = None,
    verbose=True,
    arch: str | None = None,
    component: Component = "system-probe",
    layout: str = "tasks/kernel_matrix_testing/build-layout.json",
    compile_only=False,
    override_agent=False,
):
    stack = get_kmt_or_alien_stack(ctx, stack, vms, alien_vms)

    if arch is None:
        arch = "local"

    arch_obj = Arch.from_str(arch)
    paths = KMTPaths(stack, arch_obj)
    paths.arch_dir.mkdir(parents=True, exist_ok=True)

    cc = get_compiler(ctx)

    inv_echo = "-e" if ctx.config.run["echo"] else ""
    cc.exec(f"cd {CONTAINER_AGENT_PATH} && inv {inv_echo} system-probe.object-files")

    build_task = "build-sysprobe-binary" if component == "system-probe" else "build"
    cc.exec(
        f"cd {CONTAINER_AGENT_PATH} && git config --global --add safe.directory {CONTAINER_AGENT_PATH} && inv {inv_echo} {component}.{build_task} --no-bundle --arch={arch_obj.name}",
    )

    cc.exec(f"tar cf {CONTAINER_AGENT_PATH}/kmt-deps/{stack}/build-embedded-dir.tar {EMBEDDED_SHARE_DIR}")

    if compile_only:
        return

    assert os.path.exists(layout), f"File {layout} does not exist"

    domains = get_target_domains(ctx, stack, ssh_key, arch_obj, vms, alien_vms)
    if alien_vms is not None:
        err_msg = f"no alient VMs discovered from provided profile {alien_vms}."
    else:
        err_msg = f"no vms found from list {vms}. Run `inv -e kmt.status` to see all VMs in current stack"

    assert len(domains) > 0, err_msg

    llc_path = paths.tools / "llc-bpf"
    clang_path = paths.tools / "clang-bpf"
    setup_runtime_clang(ctx, arch_obj, paths.tools)

    build_layout(ctx, domains, layout, verbose)
    for d in domains:
        # Copy embedded tools, make them
        embedded_remote_path = Path("/opt/datadog-agent/embedded/bin")
        d.copy(ctx, llc_path, embedded_remote_path / llc_path.name, verbose=verbose)
        d.copy(ctx, clang_path, embedded_remote_path / clang_path.name, verbose=verbose)

        if override_agent:
            d.run_cmd(ctx, f"[ -f /opt/datadog-agent/embedded/bin/{component} ]", verbose=False)
            d.copy(ctx, f"./bin/{component}/{component}", f"/opt/datadog-agent/embedded/bin/{component}")
        else:
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

    ctx.run("rm -rf ./test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg")
    ctx.run(f"rm -rf kmt-deps/{stack}", warn=True)
    ctx.run(f"rm {get_kmt_os().shared_dir}/*.tar.gz", warn=True)

    if container:
        ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")")
    if image:
        ctx.run("docker image rm kmt:compile")


@task(
    help={
        "stack": "List of stacks to generate ssh config for. 'all' to generate for all stacks.",
        "ddvm_rsa": "Path to the ddvm_rsa file to use for connecting to the VMs. Defaults to the path in the ami-builder repo",
    },
    iterable=["stack"],
)
def ssh_config(
    ctx: Context,
    stack: Iterable[str] | None = None,
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
    stack = set(stack or [])

    # Ensure correct permissions of the ddvm_rsa file if we're using
    # it to connect to VMs. This attribute change doesn't seem to be tracked
    # in git correctly
    ctx.run(f"chmod 600 {ddvm_rsa}", echo=False)

    for stack_dir in stacks_dir.iterdir():
        if not stack_dir.is_dir():
            continue

        output = stack_dir / "stack.output"
        if not output.exists():
            continue  # Invalid/removed stack, ignore it

        stack_name = stack_dir.name.replace('-ddvm', '')
        if len(stack) > 0 and 'all' not in stack and stack_name not in stack and stack_dir.name not in stack:
            continue

        for _, instance in build_infrastructure(stack_dir.name, try_get_ssh_key(ctx, None)).items():
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
                print("    IdentitiesOnly yes")
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
            except KeyError as e:
                raise Exit(f"[!] Invalid manifest {manifest}") from e

            if arch.kmt_arch not in platforms:
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
def explain_ci_failure(ctx: Context, pipeline: str | None = None):
    """Show a summary of KMT failures in the given pipeline."""
    if tabulate is None:
        raise Exit("tabulate module is not installed, please install it to continue")

    gitlab = get_gitlab_repo()

    if pipeline is None:
        branch = get_current_branch(ctx)
        info(f"[+] searching for the latest pipeline for this branch ({branch})")
        pipelines = cast(list[Any], gitlab.pipelines.list(ref=branch, per_page=1))
        if len(pipelines) != 1:
            raise Exit(f"[!] Could not find a pipeline for branch {branch}")
        pipeline = cast(str, pipelines[0].id)

    pipeline_data = gitlab.pipelines.get(pipeline)
    info(
        f"[+] retrieving all CI jobs for pipeline {pipeline} ({pipeline_data.web_url}), {pipeline_data.status}, created {pipeline_data.created_at} last updated {pipeline_data.updated_at}"
    )
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
    for failed_job in failed_jobs:
        if failed_job.failure_reason == "script_failure":
            failreason = testfail  # By default, we assume it's a test failure

            # Now check the artifacts, we'll guess why the job failed based on the size
            for artifact in failed_job.job.artifacts:  # type: ignore
                if artifact.get("filename") == "artifacts.zip":
                    fsize = artifact.get("size", 0)
                    if fsize < 1500:
                        # This means we don't have the junit test results, assuming an infra
                        # failure because tests didn't even run
                        failreason = infrafail
                        break
        else:
            failreason = failed_job.failure_reason

        failreasons[failed_job.name] = failreason

    # Check setup-env jobs that failed, they are infra failures for all related test jobs
    for setup_job in failed_setup_jobs:
        for test_job in setup_job.associated_test_jobs:
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
        for test_job in test_jobs:
            if test_job.component != component or test_job.vmset != vmset:
                continue

            failreason = failreasons.get(test_job.name, ok)
            distros[test_job.distro][test_job.arch] = failreason
            if failreason == testfail:
                distro_arch_with_test_failures.append((test_job.distro, test_job.arch))

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
            # they're easy to filter out) to see if we can find clues that tell us whether
            # we have status logs or not.
            non_kernel_boot_log_lines = [
                line for line in boot_log.splitlines() if re.match(r"\[[0-9 \.]+\]", line) is None
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


@task
def show_last_test_results(ctx: Context, stack: str | None = None):
    stack = check_and_get_stack(stack)
    assert stacks.stack_exists(
        stack
    ), f"Stack {stack} does not exist. Please create with 'inv kmt.create-stack --stack=<name>'"
    assert tabulate is not None, "tabulate module is not installed, please install it to continue"

    paths = KMTPaths(stack, Arch.local())
    results: dict[str, dict[str, tuple[int, int, int, int]]] = defaultdict(dict)
    vm_list: list[str] = []
    total_by_vm: dict[str, tuple[int, int, int, int]] = defaultdict(lambda: (0, 0, 0, 0))
    sum_failures = 0
    sum_tests = 0

    for vm_folder in paths.test_results.iterdir():
        if not vm_folder.is_dir():
            continue

        vm_name = "-".join(vm_folder.name.split('-')[:2])
        vm_list.append(vm_name)
        test_results: dict[str, dict[str, set[str]]] = defaultdict(lambda: defaultdict(set))

        for file in vm_folder.glob("*.xml"):
            xml = ET.parse(file)

            for testcase in xml.findall(".//testcase"):
                pkgname = testcase.get("classname")
                testname = testcase.get("name")
                if pkgname is None or testname is None:
                    continue
                failed = testcase.find("failure") is not None
                skipped = testcase.find("skipped") is not None

                if failed:
                    result = "failed"
                elif skipped:
                    result = "skipped"
                else:
                    result = "success"

                test_results[pkgname][testname].add(result)

        for pkgname, tests in test_results.items():
            failures, successes_on_retry, successes, skipped = 0, 0, 0, 0

            for testresults in tests.values():
                if len(testresults) == 1:
                    result = next(iter(testresults))
                    sum_tests += 1
                    if result == "failed":
                        failures += 1
                        sum_failures += 1
                    elif result == "success":
                        successes += 1
                    elif result == "skipped":
                        skipped += 1
                elif "failed" in testresults and "success" in testresults:
                    successes_on_retry += 1

            result_tuple = (successes, successes_on_retry, failures, skipped)

            results[pkgname][vm_name] = result_tuple
            total_by_vm[vm_name] = tuple(x + y for x, y in zip(result_tuple, total_by_vm[vm_name], strict=True))  # type: ignore

    def _color_result(result: tuple[int, int, int, int]) -> str:
        success = colored(str(result[0]), "green" if result[0] > 0 else None)
        success_on_retry = colored(str(result[1]), "blue" if result[1] > 0 else None)
        failures = colored(str(result[2]), "red" if result[2] > 0 else None)
        skipped = colored(str(result[3]), "yellow" if result[3] > 0 else None)

        return f"{success}/{success_on_retry}/{failures}/{skipped}"

    table: list[list[str]] = []
    for package, vm_results in sorted(results.items(), key=lambda x: x[0]):
        row = [package] + [_color_result(vm_results.get(vm, (0, 0, 0, 0))) for vm in vm_list]
        table.append(row)

    table.append(["Total"] + [_color_result(total_by_vm[vm]) for vm in vm_list])

    print(tabulate(table, headers=["Package"] + vm_list) + "\n")

    if sum_tests == 0:
        warn("WARN: No test runs")
    elif sum_failures > 0:
        error("ERROR: Found failed tests")
    else:
        info("SUCCESS: All tests passed")

    print("Legend: Successes/Successes on retry/Failures/Skipped")

    if sum_failures:
        sys.exit(1)


@task
def tag_ci_job(ctx: Context):
    """Add extra tags to the CI job"""
    tags: dict[str, str] = {}
    metrics: dict[str, str] = {}

    # Retrieve tags from environment variables, with a renaming for convenience
    environment_vars_to_tags = {
        "ARCH": "arch",
        "TEST_COMPONENT": "component",
        "TAG": "platform",
        "TEST_SET": "test_set",
        "MICRO_VM_IP": "microvm_ip",
    }

    for env_var, tag in environment_vars_to_tags.items():
        value = os.getenv(env_var)
        if value is not None:
            tags[tag] = value

    # Get the job type based on the job name
    job_name = os.environ["CI_JOB_NAME"]
    if "setup_env" in job_name or "upload" in job_name or "pull_test_dockers" in job_name:
        job_type = "setup"
    elif "cleanup" in job_name:
        job_type = "cleanup"
    elif "kmt_run" in job_name:
        job_type = "test"
    tags["job_type"] = job_type

    agent_testing_dir = Path(os.environ["DD_AGENT_TESTING_DIR"])
    ci_project_dir = Path(os.environ["CI_PROJECT_DIR"])

    if job_type == "test":
        # Retrieve all data for the tests to detect a failure reason
        test_jobs_executed, tests_failed = False, False
        for candidate in agent_testing_dir.glob("junit-*.tar.gz"):
            tar = tarfile.open(candidate)
            test_results = get_test_results_from_tarfile(tar)
            test_jobs_executed = test_jobs_executed or len(test_results) > 0
            # values can be none, we have to explicitly check for False
            tests_failed = tests_failed or any(r is False for r in test_results.values())

        tags["tests_executed"] = str(test_jobs_executed).lower()
        tags["tests_failed"] = str(tests_failed).lower()

        ssh_config_path = Path.home() / ".ssh" / "config"
        setup_ddvm_status_file = ci_project_dir / "setup-ddvm.status"

        if test_jobs_executed and not tests_failed:
            tags["failure_reason"] = "none"
        elif test_jobs_executed and tests_failed:  # The first condition is redundant but makes the logic clearer
            tags["failure_reason"] = "test"
        elif setup_ddvm_status_file.is_file() and "active" not in setup_ddvm_status_file.read_text():
            tags["failure_reason"] = "infra_setup-ddvm"
        elif not ssh_config_path.is_file():
            tags["failure_reason"] = "infra_ssh-config"
        else:
            tags["failure_reason"] = "infra-unknown"

        # Tag complexity results
        should_collect_complexity = os.getenv("COLLECT_COMPLEXITY") == "yes"
        collected_complexity = any(agent_testing_dir.glob("verifier-complexity-*.tar.gz"))

        if not should_collect_complexity:
            tags["complexity_collection"] = "skipped"
        elif collected_complexity:
            tags["complexity_collection"] = "success"
        else:
            tags["complexity_collection"] = "failure"
    elif job_type == "setup":
        if "kmt_setup_env" in job_name:
            tags["setup_stage"] = "infra-provision"
        elif "pull_test_dockers" in job_name:
            tags["setup_stage"] = "docker-images"
        elif "upload_dependencies" in job_name:
            tags["setup_stage"] = "dependencies"
        elif "btfs" in job_name:
            tags["setup_stage"] = "btfs"
        elif "upload_secagent_tests" in job_name or "upload_sysprobe_tests" in job_name:
            tags["setup_stage"] = "tests"

        instance_not_found_marker = ci_project_dir / "instance_not_found"
        e2e_fail_marker = ci_project_dir / "e2e-error-reason"
        if instance_not_found_marker.is_file():
            tags["failure_reason"] = "infra_instance-not-found"
        elif e2e_fail_marker.is_file():
            e2e_fail = e2e_fail_marker.read_text().strip()
            if e2e_fail != "":
                tags["failure_reason"] = f"infra_e2e_{e2e_fail}"

        e2e_retry_count = ci_project_dir / "e2e-retry-count"
        if e2e_retry_count.is_file():
            metrics["pulumi_retry_count"] = e2e_retry_count.read_text().strip()

    tag_prefix = "kmt."
    tags_str = " ".join(f"--tags '{tag_prefix}{k}:{v}'" for k, v in tags.items())

    ctx.run(f"datadog-ci tag --level job {tags_str}")

    if len(metrics) > 0:
        metrics_str = " ".join(f"--metrics '{tag_prefix}{k}:{v}'" for k, v in metrics.items())
        ctx.run(f"datadog-ci metric --level job {metrics_str}")


@task
def wait_for_setup_job(ctx: Context, pipeline_id: int, arch: str | Arch, component: Component, timeout_sec: int = 3600):
    """Wait for the setup job to finish corresponding to the given pipeline, arch and component"""
    arch = Arch.from_str(arch)
    setup_jobs, _ = get_all_jobs_for_pipeline(pipeline_id)
    matching_jobs = [j for j in setup_jobs if j.arch == arch.kmt_arch and j.component == component]
    if len(matching_jobs) != 1:
        raise Exit(f"Search for setup_job for {arch} {component} failed: result = {matching_jobs}")

    setup_job = matching_jobs[0]
    finished_status = {"failed", "success", "canceled", "skipped"}

    def _check_status(_):
        setup_job.refresh()
        info(f"[+] Status for job {setup_job.name}: {setup_job.status}")
        return setup_job.status.lower() in finished_status, None

    loop_status(_check_status, timeout_sec=timeout_sec)
    info(f"[+] Setup job {setup_job.name} finished with status {setup_job.status}")


# by default the PyYaml dumper does not indent lists correctly using to problem when
# starting the agent. The following solution is taken from
# https://stackoverflow.com/questions/25108581/python-yaml-dump-bad-indentation
class IndentedDumper(yaml.Dumper):
    def increase_indent(self, flow=False, indentless=False):
        return super().increase_indent(flow, indentless)


@task
def install_ddagent(
    ctx: Context,
    api_key: str,
    vms: str | None = None,
    alien_vms: str | None = None,
    stack: str | None = None,
    ssh_key: str | None = None,
    verbose=True,
    arch: str | None = None,
    version: str | None = None,
    datadog_yaml: str | None = None,
    layout: str | None = None,
):
    stack = get_kmt_or_alien_stack(ctx, stack, vms, alien_vms)

    if arch is None:
        arch = "local"

    arch_obj = Arch.from_str(arch)

    domains = get_target_domains(ctx, stack, ssh_key, arch_obj, vms, alien_vms)
    if alien_vms is not None:
        err_msg = f"no alient VMs discovered from provided profile {alien_vms}."
    else:
        err_msg = f"no vms found from list {vms}. Run `inv -e kmt.status` to see all VMs in current stack"

    assert len(domains) > 0, err_msg

    if version is not None:
        check_version(version)
    else:
        with open("release.json") as f:
            release = json.load(f)
        version = release["last_stable"]["7"]

    match = VERSION_RE.match(version)
    if not match:
        raise Exit(f"Version {version} not of expected pattern")

    groups = match.groups()
    major = groups[1]
    minor = groups[2]
    env = [
        f"DD_API_KEY={api_key}",
        f"DD_AGENT_MAJOR_VERSION={major}",
        f"DD_AGENT_MINOR_VERSION={minor}",
        "DD_INSTALL_ONLY=true",
    ]

    if datadog_yaml is not None:
        with open(datadog_yaml) as f:
            ddyaml = yaml.load(f, Loader=yaml.SafeLoader)

    for d in domains:
        d.run_cmd(
            ctx,
            f"curl -L https://install.datadoghq.com/scripts/install_script_agent{major}.sh > /tmp/install-script.sh",
            verbose=verbose,
        )
        d.run_cmd(ctx, f"{' '.join(env)} bash /tmp/install-script.sh", verbose=verbose)

        # setup datadog yaml
        if datadog_yaml is not None:
            # hostnames with '_' are not accepted according to RFC1123
            ddyaml["hostname"] = f"{os.getlogin()}_{d.tag}".replace("_", "-")
            ddyaml["api_key"] = api_key
            with tempfile.NamedTemporaryFile(mode='w') as tmp:
                yaml.dump(ddyaml, tmp, Dumper=IndentedDumper, default_flow_style=False)
                tmp.flush()
                d.copy(ctx, tmp.name, "/etc/datadog-agent/datadog.yaml")

    if layout is not None:
        build_layout(ctx, domains, layout, verbose)


@task(
    help={
        "commit": "The commit to download the complexity data for",
        "dest_path": "The path to save the complexity data to",
        "keep_compressed_archives": "Keep the compressed archives after extracting the data. Useful for testing, as it replicates the exact state of the artifacts in CI",
    }
)
def download_complexity_data(ctx: Context, commit: str, dest_path: str | Path, keep_compressed_archives: bool = False):
    gitlab = get_gitlab_repo()
    dest_path = Path(dest_path)

    # We can't get all the pipelines associated with a commit directly, so we get it by the commit statuses
    commit_statuses = gitlab.commits.get(commit, lazy=True).statuses.list(all=True)
    pipeline_ids = {status.pipeline_id for status in commit_statuses if 'pipeline_id' in status.asdict()}
    print(f"Found pipelines {pipeline_ids}")

    for pipeline_id in pipeline_ids:
        pipeline = gitlab.pipelines.get(pipeline_id)
        if pipeline.source != "push":
            print(f"Ignoring pipeline {pipeline_id}, only using push pipelines")
            continue

        _, test_jobs = get_all_jobs_for_pipeline(pipeline_id)
        for job in test_jobs:
            complexity_name = f"verifier-complexity-{job.arch}-{job.distro}-{job.component}"
            complexity_data_fname = f"test/kitchen/{complexity_name}.tar.gz"
            data = job.artifact_file_binary(complexity_data_fname, ignore_not_found=True)
            if data is None:
                print(f"Complexity data not found for {job.name} - filename {complexity_data_fname} not found")
                continue

            if keep_compressed_archives:
                with open(dest_path / f"{complexity_name}.tar.gz", "wb") as f:
                    f.write(data)

            tar = tarfile.open(fileobj=io.BytesIO(data))
            job_folder = dest_path / complexity_name
            job_folder.mkdir(parents=True, exist_ok=True)
            tar.extractall(dest_path / job_folder)
            print(f"Extracted complexity data for {job.name} successfully, filename {complexity_data_fname}")


@task
def flare(ctx: Context, dest_folder: Path | str | None = None, keep_uncompressed_files: bool = False):
    if dest_folder is None:
        dest_folder = "."
    dest_folder = Path(dest_folder)

    with tempfile.TemporaryDirectory() as tmpdir:
        flare_kmt_os(ctx, Path(tmpdir), dest_folder, keep_uncompressed_files)

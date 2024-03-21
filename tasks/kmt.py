import json
import os
import platform
import re
import shutil
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
from tasks.kernel_matrix_testing.init_kmt import init_kernel_matrix_testing_system
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.stacks import check_and_get_stack
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
        "vmconfig_template": "Template to use for the generated vmconfig.json file. Defaults to 'system-probe'. A file named 'vmconfig-<vmconfig_template>.json' must exist in 'tasks/new-e2e/system-probe/config/'",
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
    vmconfig_template="system-probe",
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
    vmconfig_template="system-probe",
):
    """
    Generate a vmconfig.json file with the VMs that failed jobs in the given pipeline.
    """
    gitlab = Gitlab(api_token=get_gitlab_token())
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
    return vmconfig.gen_config(
        ctx, stack, ",".join(vms), "", init_stack, vcpu, memory, new, ci, arch, output_file, vmconfig_template
    )


@task
def launch_stack(ctx, stack=None, ssh_key="", x86_ami=X86_AMI_ID_SANDBOX, arm_ami=ARM_AMI_ID_SANDBOX, provision_microvms=True):
    stacks.launch_stack(ctx, stack, ssh_key, x86_ami, arm_ami, provision_microvms)


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

    infrastructure = build_infrastructure(stack, remote_ssh_key="")
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
def update_resources(ctx, vmconfig_template="system-probe"):
    kmt_os = get_kmt_os()

    warn("Updating resource dependencies will delete all running stacks.")
    if ask("are you sure you want to continue? (y/n)").lower() != "y":
        raise Exit("[-] Update aborted")

    for stack in glob(f"{kmt_os.stacks_dir}/*"):
        destroy_stack(ctx, stack=os.path.basename(stack))

    update_rootfs(ctx, kmt_os.rootfs_dir, vmconfig_template)


@task
def build_compiler(ctx):
    build_cc(ctx)


@task
def start_compiler(ctx):
    start_cc(ctx)


def filter_target_domains(vms, infra, local_arch):
    vmsets = vmconfig.build_vmsets(vmconfig.build_normalized_vm_def_set(vms), [])
    domains = list()
    for vmset in vmsets:
        if vmset.arch != "local" and vmset.arch != local_arch:
            raise Exit(f"KMT does not support cross-arch ({local_arch} -> {vmset.arch}) build/test at the moment")
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


def build_tests_package(ctx, source_dir, stack, arch, ci, verbose=True):
    root = os.path.join(source_dir, "kmt-deps")
    test_archive = f"tests-{arch}.tar.gz"
    if not ci:
        system_probe_tests = os.path.join(root, stack, "opt/system-probe-tests")
        test_pkgs = os.path.join(
            source_dir, "test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg"
        )
        ctx.run(f"rm -rf {system_probe_tests} && mkdir -p {system_probe_tests}", hide=(not verbose))
        ctx.run(f"cp -R {test_pkgs} {system_probe_tests}", hide=(not verbose))
        with ctx.cd(os.path.join(root, stack)):
            ctx.run(f"tar czvf {test_archive} opt", hide=(not verbose))


@task
def build_dependencies(ctx, arch, layout_file, source_dir, ci=False, stack=None, verbose=True):
    root = os.path.join(source_dir, "kmt-deps")
    deps_dir = os.path.join(root, "dependencies")
    if not ci:
        if stack is None:
            raise Exit("no stack name provided")
        deps_dir = os.path.join(root, stack, "dependencies")
        # in the CI we can rely on gotestsum being present
        download_gotestsum(ctx)

    if os.path.exists(deps_dir):
        shutil.rmtree(deps_dir)

    ctx.run(f"mkdir -p {deps_dir}")

    with open(layout_file) as f:
        deps_layout = json.load(f)
    with ctx.cd(deps_dir):
        for new_dirs in deps_layout["layout"]:
            ctx.run(f"mkdir -p {new_dirs}", hide=(not verbose))

    for source in deps_layout["copy"]:
        target = deps_layout["copy"][source]
        ctx.run(f"cp {os.path.join(source_dir, source)} {os.path.join(deps_dir, target)}", hide=(not verbose))

    def _exec_context_ci(ctx, command, directory):
        ctx.run(f"cd {os.path.join(source_dir, directory)} && {command}", hide=(not verbose))

    def _exec_context(ctx, command, directory):
        docker_exec(ctx, command, run_dir=f"/datadog-agent/{directory}", verbose=verbose)

    exec_context = _exec_context
    if ci:
        exec_context = _exec_context_ci
    for build in deps_layout["build"]:
        directory = deps_layout["build"][build]["directory"]
        command = deps_layout["build"][build]["command"]
        artifact = os.path.join(source_dir, deps_layout["build"][build]["artifact"])
        exec_context(ctx, command, directory)
        ctx.run(f"cp {artifact} {deps_dir}", hide=(not verbose))

    archive_name = f"dependencies-{arch}.tar.gz"
    with ctx.cd(os.path.join(root, stack)):
        ctx.run(f"tar czvf {archive_name} dependencies", hide=(not verbose))


def is_root():
    return os.getuid() == 0


def vms_have_correct_deps(ctx, domains, depsfile):
    deps_dir = os.path.dirname(depsfile)
    sha256sum = ctx.run(f"cd {deps_dir} && sha256sum {os.path.basename(depsfile)}", warn=True)
    if not sha256sum.ok:
        return False

    check = sha256sum.stdout.rstrip('\n')
    for d in domains:
        if not d.run_cmd(ctx, f"cd / && echo \"{check}\" | sha256sum --check", allow_fail=True):
            warn(f"[-] VM {d} does not have dependencies.")
            return False

    return True


@task
def prepare(ctx, vms, stack=None, ssh_key=None, full_rebuild=False, packages="", verbose=True):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if vms == "":
        raise Exit("No vms specified to sync with")

    arch = arch_mapping[platform.machine()]

    infra = build_infrastructure(stack, ssh_key)
    domains = filter_target_domains(vms, infra, arch)
    build_from_scratch = (
        full_rebuild
        or (not os.path.exists(f"kmt-deps/{stack}"))
        or (not vms_have_correct_deps(ctx, domains, os.path.join("kmt-deps", stack, f"dependencies-{arch}.tar.gz")))
    )

    if not compiler_running(ctx):
        start_compiler(ctx)

    constrain_pkgs = ""
    if not build_from_scratch:
        constrain_pkgs = f"--packages={packages}"

    docker_exec(
        ctx,
        f"git config --global --add safe.directory /datadog-agent && inv -e system-probe.kitchen-prepare --ci {constrain_pkgs}",
        run_dir="/datadog-agent",
    )

    target_instances = list()
    for d in domains:
        target_instances.append(d.instance)

    if build_from_scratch:
        info("[+] Building all dependencies from scratch")
        build_dependencies(
            ctx, arch, "test/new-e2e/system-probe/test-runner/files/system-probe-dependencies.json", "./", stack=stack
        )

        for instance in target_instances:
            instance.copy_to_all_vms(ctx, f"kmt-deps/{stack}/dependencies-{full_arch(instance.arch)}.tar.gz")

        for d in domains:
            if not d.run_cmd(ctx, f"/root/fetch_dependencies.sh {arch}", allow_fail=True, verbose=verbose):
                raise Exit(f"failed to fetch dependencies for domain {d}")

            info(f"[+] Dependencies shared with target VM {d}")

    tests_archive = f"tests-{arch}.tar.gz"
    build_tests_package(ctx, "./", stack, arch, False)
    for d in domains:
        d.copy(ctx, f"kmt-deps/{stack}/{tests_archive}", "/")
        d.run_cmd(ctx, f"cd / && tar xzf {tests_archive}")
        info(f"[+] Tests packages setup in target VM {d}")


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


@task(
    help={
        "vms": "Comma seperated list of vms to target when running tests",
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
    ctx,
    vms,
    stack=None,
    packages="",
    run=None,
    quick=False,
    retry=2,
    run_count=1,
    full_rebuild=False,
    ssh_key=None,
    verbose=True,
    test_logs=False,
    test_extra_arguments=None,
):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not quick:
        prepare(ctx, stack=stack, vms=vms, ssh_key=ssh_key, full_rebuild=full_rebuild, packages=packages)

    infra = build_infrastructure(stack, ssh_key)
    arch = arch_mapping[platform.machine()]
    domains = filter_target_domains(vms, infra, arch)
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
            f"-packages-run-config {tmp.name}",
            f"-retry {retry}",
            "-verbose" if test_logs else "",
            f"-run-count {run_count}",
            "-test-root /opt/system-probe-tests",
            f"-extra-params {test_extra_arguments}" if test_extra_arguments is not None else "",
        ]
        for d in domains:
            d.copy(ctx, f"{tmp.name}", "/tmp")
            d.run_cmd(ctx, f"bash /micro-vm-init.sh {' '.join(args)}", verbose=verbose)


@task(
    help={
        "vms": "Comma seperated list of vms to target when running tests",
        "stack": "Stack in which the VMs exist. If not provided stack is autogenerated based on branch name",
        "ssh-key": "SSH key to use for connecting to a remote EC2 instance hosting the target VM",
        "full-rebuild": "Do a full rebuild of all test dependencies to share with VMs, before running tests. Useful when changes are not being picked up correctly",
        "verbose": "Enable full output of all commands executed",
    }
)
def build(ctx, vms, stack=None, ssh_key=None, full_rebuild=False, verbose=True):
    stack = check_and_get_stack(stack)
    if not stacks.stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not os.path.exists(f"kmt-deps/{stack}"):
        ctx.run(f"mkdir -p kmt-deps/{stack}")

    arch = arch_mapping[platform.machine()]
    infra = build_infrastructure(stack, ssh_key)
    domains = filter_target_domains(vms, infra, arch)

    build_from_scratch = (
        full_rebuild
        or (not os.path.exists(f"kmt-deps/{stack}"))
        or (not vms_have_correct_deps(ctx, domains, os.path.join("kmt-deps", stack, f"dependencies-{arch}.tar.gz")))
    )

    if build_from_scratch:
        build_dependencies(
            ctx, arch, "test/new-e2e/system-probe/test-runner/files/system-probe-dependencies.json", "./", stack=stack
        )

        target_instances = list()
        for d in domains:
            target_instances.append(d.instance)

        for instance in target_instances:
            instance.copy_to_all_vms(ctx, f"kmt-deps/{stack}/dependencies-{full_arch(instance.arch)}.tar.gz")

        for d in domains:
            d.run_cmd(ctx, f"/root/fetch_dependencies.sh {arch_mapping[platform.machine()]}")
            info(f"[+] Dependencies shared with target VM {d}")

    docker_exec(
        ctx,
        "cd /datadog-agent && git config --global --add safe.directory /datadog-agent && inv -e system-probe.build --no-bundle",
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
                # Disable host key checking, the IPs of the QEMU machines are reused and we don't want constant
                # warnings about changed host keys. We need the combination of both options, if we just set
                # StrictHostKeyChecking to no, it will still check the known hosts file and disable some options
                # and print out scary warnings if the key doesn't match.
                print("    UserKnownHostsFile /dev/null")
                print("    StrictHostKeyChecking accept-new")
                print("")

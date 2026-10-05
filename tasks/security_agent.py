from __future__ import annotations

import datetime
import errno
import json
import os
import shutil
import sys
from subprocess import check_output

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.build_tags import get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.go import run_golangci_lint
from tasks.libs.build.bazel import bazel, build_binaries_with_bazel, build_binary_with_bazel
from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_commit_sha, get_common_ancestor, get_current_branch
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
    get_go_version,
    get_version,
)
from tasks.libs.types.arch import Arch
from tasks.process_agent import TempDir
from tasks.schema.generate import schema_codegen
from tasks.system_probe import (
    CURRENT_ARCH,
    build_cws_object_files,
    build_libpcap,
    copy_ebpf_and_related_files,
    ebpf_bazel_flags,
    get_libpcap_cgo_flags,
    ninja_define_ebpf_compiler,
    ninja_define_exe_compiler,
)
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

is_windows = sys.platform == "win32"

# Bound so the ninja compiler helpers stay live until remaining ninja graphs are gone.
_NINJA_COMPILER_HELPERS = (ninja_define_ebpf_compiler, ninja_define_exe_compiler)

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "security-agent", bin_name("security-agent"))
CI_PROJECT_DIR = os.environ.get("CI_PROJECT_DIR", ".")

BAZEL_TARGET = "//cmd/security-agent:security-agent"


@task(iterable=["build_tags"])
def build(
    ctx,
    build_tags,
    race=False,
    rebuild=False,
    install_path=None,
    go_mod="readonly",
    static=False,
    fips_mode=False,
    enable_bazel=False,
):
    """
    Build the security agent
    """

    if enable_bazel:
        if build_tags:
            raise NotImplementedError("--enable-bazel does not support --build-tags.")
        if race:
            raise NotImplementedError("--enable-bazel does not support --race.")
        if install_path is not None:
            raise NotImplementedError("--enable-bazel does not support --install-path.")
        if static:
            raise NotImplementedError("--enable-bazel does not support --static.")

        bazel_args = ["--//packages/agent:flavor=fips"] if fips_mode else []
        build_binary_with_bazel(BAZEL_TARGET, args=bazel_args, bin_path=BIN_PATH)
        return

    ldflags, gcflags, env = get_build_flags(ctx, static=static, install_path=install_path)

    main = "main."
    ld_vars = {
        "Version": get_version(ctx),
        "GoVersion": get_go_version(),
        "GitBranch": get_current_branch(ctx),
        "GitCommit": get_commit_sha(ctx, short=True),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    ## build windows resources
    # generate windows resources
    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx)
        build_rc(
            ctx,
            "cmd/security-agent/windows_resources/security-agent.rc",
            vars=vars,
            out="cmd/security-agent/rsrc.syso",
        )

    ldflags += ' '.join([f"-X '{main + key}={value}'" for key, value in ld_vars.items()])
    build_tags += get_default_build_tags(
        build="security-agent", flavor=AgentFlavor.fips if fips_mode else AgentFlavor.base
    )

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/security-agent",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=BIN_PATH,
        env=env,
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
        coverage=os.getenv("E2E_COVERAGE_PIPELINE") == "true",
    )


@task
def build_dev_image(ctx, image=None, push=False, base_image="datadog/agent:latest", include_agent_binary=False):
    """
    Build a dev image of the security-agent based off an existing datadog-agent image

    image: the image name used to tag the image
    push: if true, run a docker push on the image
    base_image: base the docker image off this already build image (default: datadog/agent:latest)
    include_agent_binary: if true, use the agent binary in bin/agent/agent as opposite to the base image's binary
    """
    if image is None:
        raise Exit(message="image was not specified")

    with TempDir() as docker_context:
        ctx.run(f"cp tools/ebpf/Dockerfiles/Dockerfile-security-agent-dev {docker_context + '/Dockerfile'}")

        ctx.run(f"cp bin/security-agent/security-agent {docker_context + '/security-agent'}")
        ctx.run(f"cp bin/system-probe/system-probe {docker_context + '/system-probe'}")
        if include_agent_binary:
            ctx.run(f"cp bin/agent/agent {docker_context + '/agent'}")
            core_agent_dest = "/opt/datadog-agent/bin/agent/agent"
        else:
            # this is necessary so that the docker build doesn't fail while attempting to copy the agent binary
            ctx.run(f"touch {docker_context}/agent")
            core_agent_dest = "/dev/null"

        copy_ebpf_and_related_files(ctx, docker_context)

        with ctx.cd(docker_context):
            # --pull in the build will force docker to grab the latest base image
            ctx.run(
                f"docker build --pull --tag {image} --build-arg AGENT_BASE={base_image} --build-arg CORE_AGENT_DEST={core_agent_dest} ."
            )

    if push:
        ctx.run(f"docker push {image}")


@task()
def gen_mocks(_):
    """
    Generate mocks.
    """
    bazel("run", "//internal/tools:mockery")


@task
def run_functional_tests(ctx, testsuite, verbose=False, testflags=''):
    cmd = '{testsuite} {verbose_opt} {testflags}'
    if os.getuid() != 0:
        cmd = 'sudo -E PATH={path} ' + cmd

    args = {
        "testsuite": testsuite,
        "verbose_opt": "-test.v" if verbose else "",
        "testflags": testflags,
        "path": os.environ['PATH'],
    }

    ctx.run(cmd.format(**args))


@task
def run_ebpfless_functional_tests(ctx, testsuite, verbose=False, testflags=''):
    cmd = '{testsuite} -trace {verbose_opt} {testflags}'

    if os.getuid() != 0:
        cmd = 'sudo -E PATH={path} ' + cmd

    args = {
        "testsuite": testsuite,
        "verbose_opt": "-test.v" if verbose else "",
        "testflags": testflags,
        "path": os.environ['PATH'],
    }

    ctx.run(cmd.format(**args))


OTEL_TLS_BAZEL_TARGET = "//pkg/security/tests/syscall_tester/c:otel_tls_artifacts"


# The OTel TLS testers go through Bazel so they link against the hermetic
# crosstool-NG sysroot: glibc 2.23, of which only 2.17 symbols end up referenced. The host toolchain would link them against the
# build image's glibc instead, which is newer than every KMT host and than the
# ubuntu:20.04 image RunMultiMode's docker leg uses, and every dynamically
# linked variant would then be skipped outside the newest legs. The Node.js
# tester is Bazel-built the same way, alongside the native one; it is glibc-only,
# so there is no musl counterpart to build.
#
# musl is covered by TestResolveOTelTLSMuslDTV in
# pkg/security/resolvers/process/otel_tls_test.go instead: the only thing musl
# changes is the DTV layout its libc reports, which is resolved entirely in
# user space and needs neither eBPF nor a VM.
def build_otel_tls_artifacts(build_dir, arch: Arch):
    if arch.is_cross_compiling():
        # Both crosstool-NG toolchains are exec_compatible_with their own CPU,
        # so there is no toolchain that targets the other architecture.
        print("Skipping the OTel TLS glibc testers while cross-compiling")
        return

    bazel("build", OTEL_TLS_BAZEL_TARGET)

    # The filegroup is the one list of artifacts; asking Bazel for its files
    # keeps this from drifting from the BUILD file.
    execroot = bazel("info", "execution_root", capture_output=True).strip()
    artifacts = bazel("cquery", "--output=files", OTEL_TLS_BAZEL_TARGET, capture_output=True).split()

    for artifact in artifacts:
        src = os.path.join(execroot, artifact)
        dst = os.path.join(build_dir, os.path.basename(artifact))
        shutil.copy2(src, dst)
        os.chmod(dst, 0o755)


def create_dir_if_needed(dir):
    try:
        os.makedirs(dir)
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise


_SYSCALL_TESTER_TARGETS = {
    "//pkg/security/tests/syscall_tester/c:syscall_tester": "syscall_tester",
    "//pkg/security/tests/syscall_tester/go:syscall_go_tester": "syscall_go_tester",
    "//pkg/security/tests/syscall_tester/go/span:span_go_tester": "span_go_tester",
}


@task
def build_embed_syscall_tester(ctx, arch: str | Arch = CURRENT_ARCH, static=True, compiler="clang"):
    del ctx, static, compiler  # Bazel always produces static testers; ninja/clang are gone.
    arch = Arch.from_str(arch)
    build_dir = os.path.join("pkg", "security", "tests", "syscall_tester", "bin")
    create_dir_if_needed(build_dir)

    from tasks.kmt import kmt_bazel_flags

    dest_by_target = {target: os.path.join(build_dir, dest) for target, dest in _SYSCALL_TESTER_TARGETS.items()}
    # -m32 needs a 32-bit sysroot the hermetic toolchain does not provide.
    flags = kmt_bazel_flags(arch) + ebpf_bazel_flags(arch)
    build_binaries_with_bazel(dest_by_target, args=flags)
    build_otel_tls_artifacts(build_dir, arch)


@task
def build_functional_tests(
    ctx,
    output='pkg/security/tests/testsuite',
    srcpath='pkg/security/tests',
    arch: str | Arch = CURRENT_ARCH,
    build_tags='functionaltests',
    build_flags='',
    bundle_ebpf=True,
    static=False,
    skip_linters=False,
    race=False,
    skip_object_files=False,
    syscall_tester_compiler='clang',
):
    if not is_windows:
        if not skip_object_files:
            build_cws_object_files(
                ctx,
                arch=arch,
            )
        build_embed_syscall_tester(
            ctx,
            compiler=syscall_tester_compiler,
            arch=arch,
        )

    arch = Arch.from_str(arch)
    ldflags, gcflags, env = get_build_flags(ctx, static=static, arch=arch)
    common_ancestor = get_common_ancestor(ctx, "HEAD")
    print(f"Using git ref {common_ancestor} as common ancestor between HEAD and main branch")
    ldflags += f"-X {REPO_PATH}/{srcpath}.GitAncestorOnMain={common_ancestor} "

    env["CGO_ENABLED"] = "1"

    build_tags = build_tags.split(",")
    build_tags.append("test")
    build_tags.append("seclmax")
    if not is_windows:
        build_tags.append("bpf")
        build_tags.append("trivy")
        build_tags.append("containerd")

        if bundle_ebpf:
            build_tags.append("ebpf_bindata")

        build_tags.append("pcap")
        build_libpcap(ctx, env=env, arch=arch)
        cgo_flags = get_libpcap_cgo_flags(ctx)
        # append libpcap cgo-related environment variables to any existing ones
        for k, v in cgo_flags.items():
            if k in env:
                env[k] += f" {v}"
            else:
                env[k] = v

    if static:
        build_tags.extend(["osusergo", "netgo"])

    if not skip_linters:
        targets = [srcpath]
        results, _ = run_golangci_lint(ctx, base_path="", targets=targets, build_tags=build_tags)
        for result in results:
            # golangci exits with status 1 when it finds an issue
            if result.returncode != 0:
                raise Exit(code=1)
        print("golangci-lint found no issues")

    if race:
        build_flags += " -race"

    build_tags = ",".join(build_tags)
    cmd = 'go test -mod=readonly -tags {build_tags} -gcflags="{gcflags}" -ldflags="{ldflags}" -c -o {output} '
    cmd += '{build_flags} {repo_path}/{src_path}'

    args = {
        "output": output,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "build_flags": build_flags,
        "build_tags": build_tags,
        "repo_path": REPO_PATH,
        "src_path": srcpath,
    }

    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    ctx.run(cmd.format(**args), env=env)


@task
def functional_tests(
    ctx,
    verbose=False,
    race=False,
    output='pkg/security/tests/testsuite',
    bundle_ebpf=True,
    testflags='',
    skip_linters=False,
):
    build_functional_tests(
        ctx,
        output=output,
        bundle_ebpf=bundle_ebpf,
        skip_linters=skip_linters,
        race=race,
    )

    run_functional_tests(
        ctx,
        testsuite=output,
        verbose=verbose,
        testflags=testflags,
    )


@task
def ebpfless_functional_tests(
    ctx,
    verbose=False,
    race=False,
    arch=CURRENT_ARCH,
    output='pkg/security/tests/testsuite',
    bundle_ebpf=True,
    testflags='',
    skip_linters=False,
):
    build_functional_tests(
        ctx,
        output=output,
        bundle_ebpf=bundle_ebpf,
        skip_linters=skip_linters,
        race=race,
    )

    run_ebpfless_functional_tests(
        ctx,
        testsuite=output,
        verbose=verbose,
        testflags=testflags,
    )


@task
def docker_functional_tests(
    ctx,
    verbose=False,
    race=False,
    arch=CURRENT_ARCH,
    testflags='',
    bundle_ebpf=True,
    skip_linters=False,
):
    build_functional_tests(
        ctx,
        output="pkg/security/tests/testsuite",
        bundle_ebpf=bundle_ebpf,
        static=True,
        skip_linters=skip_linters,
        race=race,
    )

    image_tag = "ghcr.io/datadog/apps-cws-centos7:main"

    container_name = 'security-agent-tests'
    capabilities = ['SYS_ADMIN', 'SYS_RESOURCE', 'SYS_PTRACE', 'NET_ADMIN', 'IPC_LOCK', 'ALL']

    cmd = 'docker run --name {container_name} {caps} --privileged -d '
    cmd += '--env=CI '
    cmd += '-v /dev:/dev '
    cmd += '-v /proc:/host/proc -e HOST_PROC=/host/proc '
    cmd += '-v /etc:/host/etc -e HOST_ETC=/host/etc '
    cmd += '-v /sys:/host/sys -e HOST_SYS=/host/sys '
    cmd += '-v /etc/os-release:/host/etc/os-release '
    cmd += '-v /usr/lib/os-release:/host/usr/lib/os-release '
    cmd += '-v /etc/passwd:/etc/passwd '
    cmd += '-v /etc/group:/etc/group '
    cmd += '-v /opt/datadog-agent/embedded/:/opt/datadog-agent/embedded/ '
    cmd += '-v ./pkg/security/tests:/tests {image_tag} sleep 3600'

    args = {
        "container_name": container_name,
        "caps": ' '.join(f"--cap-add {cap}" for cap in capabilities),
        "image_tag": image_tag,
    }

    ctx.run(cmd.format(**args))

    cmd = 'docker exec {container_name} mount -t debugfs none /sys/kernel/debug'
    ctx.run(cmd.format(**args))

    cmd = 'docker exec {container_name} /tests/testsuite --env docker {testflags}'
    if verbose:
        cmd += ' -test.v'
    try:
        ctx.run(cmd.format(testflags=testflags, **args))
    finally:
        cmd = 'docker rm -f {container_name}'
        ctx.run(cmd.format(**args))


@task
def generate_cws_documentation(ctx):
    bazel("run", "//docs/cloud-workload-security:cws_docs")


@task
def cws_go_generate(ctx, windows=False):
    # CWS codegens keep their //go:generate directives so a future Gazelle
    # extension can emit the matching Bazel targets from them (ABLD-475).
    # Off Windows, cws_codegen renders backend_windows.md from the committed
    # schema, so refresh that first.
    if windows and sys.platform == "linux":
        bazel("run", "//docs/cloud-workload-security:backend_windows_schema")
    bazel("run", "//pkg/security:cws_codegen")


@task
def generate_syscall_table(ctx):
    """Regenerate secl model syscall enums from the pinned Linux kernel tables.

    Tables are fetched as http_file repos in MODULE.bazel (same pins as
    utils_syscall_table). Bumping the kernel version means updating those
    URLs and sha256 entries.
    """
    bazel("run", "//pkg/security/secl/model:syscall_table")


@task
def generate_utils_syscall_table(ctx):
    # The kernel files are fetched as `http_file` repos pinned in MODULE.bazel;
    # bumping the kernel version means updating those URLs and sha256 entries.
    bazel("run", "//pkg/security/utils:utils_syscall_table")


DEFAULT_BTFHUB_CONSTANTS_PATH = "./pkg/security/probe/constantfetch/btfhub/constants.json"
DEFAULT_BTFHUB_CONSTANTS_ARM64_PATH = "./pkg/security/probe/constantfetch/constants_arm64.json"
DEFAULT_BTFHUB_CONSTANTS_AMD64_PATH = "./pkg/security/probe/constantfetch/constants_amd64.json"


@task
def generate_btfhub_constants(ctx, archive_path, output_path=DEFAULT_BTFHUB_CONSTANTS_PATH):
    ctx.run(
        f"go run -tags bpf,btfhubsync ./pkg/security/probe/constantfetch/btfhub/ -archive-root {archive_path} -output {output_path}",
    )


@task
def combine_btfhub_constants(ctx, archive_path, output_path=DEFAULT_BTFHUB_CONSTANTS_PATH):
    ctx.run(
        f"go run -tags bpf,btfhubsync ./pkg/security/probe/constantfetch/btfhub/ -combine -archive-root {archive_path} -output {output_path}",
    )


@task
def extract_btfhub_constants(ctx, arch, input_path, output_path):
    res = {}
    used_contant_ids = set()
    with open(input_path) as fi:
        base = json.load(fi)
        res["constants"] = base["constants"]
        res["kernels"] = []
        for kernel in base["kernels"]:
            if kernel["arch"] == arch:
                res["kernels"].append(kernel)
                used_contant_ids.add(kernel["cindex"])

    new_constants = []
    mapping = {}
    for i, group in enumerate(res["constants"]):
        if i in used_contant_ids:
            new_i = len(new_constants)
            new_constants.append(group)
            mapping[i] = new_i

    for kernel in res["kernels"]:
        kernel["cindex"] = mapping[kernel["cindex"]]
    res["constants"] = new_constants

    with open(output_path, "w") as fo:
        json.dump(res, fo, indent="\t")


@task
def split_btfhub_constants(ctx):
    extract_btfhub_constants(ctx, "arm64", DEFAULT_BTFHUB_CONSTANTS_PATH, DEFAULT_BTFHUB_CONSTANTS_ARM64_PATH)
    extract_btfhub_constants(ctx, "x86_64", DEFAULT_BTFHUB_CONSTANTS_PATH, DEFAULT_BTFHUB_CONSTANTS_AMD64_PATH)


@task
def generate_cws_proto(ctx):
    print(
        color_message(
            """DEPRECATED - use one of the following instead:
- bazel run //pkg/security/proto/api:write_pb_go
- bazel run //:write_all
""",
            "orange",
        )
    )
    bazel("run", "//pkg/security/proto/api:write_pb_go")


def get_git_dirty_files():
    dirty_stats = check_output(["git", "status", "--porcelain=v1", "--untracked-files=no"]).decode('utf-8')
    paths = []

    # see https://git-scm.com/docs/git-status#_short_format for format documentation
    for line in dirty_stats.splitlines():
        if len(line) < 2:
            continue

        path_part = line[2:]
        path = path_part.split()[0]
        paths.append(path)
    return paths


@task
def go_generate_check(ctx):
    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    # The other CWS generated files are guarded by their Bazel diff tests;
    # mockery has none yet.
    gen_mocks(ctx)
    dirty_files = get_git_dirty_files()
    if dirty_files:
        print("Task `dda inv security-agent.gen-mocks` resulted in dirty files, please re-run it:")
        for file in dirty_files:
            print(f"* {file}")
        raise Exit(code=1)


E2E_ARTIFACT_DIR = os.path.join(CI_PROJECT_DIR, "test", "new-e2e", "tests", "security-agent-functional", "artifacts")


@task
def e2e_prepare_win(ctx):
    """
    Compile test suite for CWS windows new-e2e tests
    """

    out_binary = "testsuite.exe"

    testsuite_out_dir = E2E_ARTIFACT_DIR
    # Clean up previous build
    if os.path.exists(testsuite_out_dir):
        shutil.rmtree(testsuite_out_dir)

    testsuite_out_path = os.path.join(testsuite_out_dir, out_binary)
    build_functional_tests(
        ctx,
        bundle_ebpf=False,
        race=False,
        output=testsuite_out_path,
        skip_linters=True,
    )

    # build the ETW tests binary also
    testsuite_out_path = os.path.join(E2E_ARTIFACT_DIR, "etw", out_binary)
    srcpath = 'pkg/security/probe'
    build_functional_tests(
        ctx,
        output=testsuite_out_path,
        srcpath=srcpath,
        bundle_ebpf=False,
        race=False,
        skip_linters=True,
    )


@task
def run_ebpf_unit_tests(ctx, verbose=False, trace=False, testflags=''):
    build_cws_object_files(ctx, with_unit_test=True, arch=CURRENT_ARCH)

    env = {"CGO_ENABLED": "1"}

    build_libpcap(ctx, env=env)
    cgo_flags = get_libpcap_cgo_flags(ctx)
    # append libpcap cgo-related environment variables to any existing ones
    for k, v in cgo_flags.items():
        if k in env:
            env[k] += f" {v}"
        else:
            env[k] = v

    flags = '-tags ebpf_bindata,cgo,pcap'
    if verbose:
        flags += " -test.v"

    args = '-args'
    if trace:
        args += " -trace"

    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    ctx.run(f"go test {flags} ./pkg/security/ebpf/tests/... {args} {testflags}", env=env)


@task
def print_fentry_stats(ctx):
    fentry_o_path = "pkg/ebpf/bytecode/build/runtime-security-fentry.o"

    for kind in ["kprobe", "kretprobe", "fentry", "fexit"]:
        ctx.run(f"readelf -W -S {fentry_o_path} 2> /dev/null | grep PROGBITS | grep {kind} | wc -l")

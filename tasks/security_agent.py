from __future__ import annotations

import datetime
import errno
import glob
import json
import os
import re
import shutil
import sys
import tempfile
from itertools import chain
from subprocess import check_output

from invoke.exceptions import Exit
from invoke.tasks import task

import tasks.libs.cws.backend_doc_gen as backend_doc_gen
import tasks.libs.cws.secl_doc_gen as secl_doc_gen
from tasks.agent import generate_config
from tasks.build_tags import add_fips_tags, get_default_build_tags
from tasks.go import run_golangci_lint
from tasks.libs.build.ninja import NinjaWriter
from tasks.libs.common.git import get_commit_sha, get_current_branch
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import (
    REPO_PATH,
    bin_name,
    environ,
    get_build_flags,
    get_go_version,
    get_version,
)
from tasks.libs.types.arch import ARCH_AMD64, Arch
from tasks.process_agent import TempDir
from tasks.system_probe import (
    CURRENT_ARCH,
    build_cws_object_files,
    build_libpcap,
    check_for_ninja,
    copy_ebpf_and_related_files,
    get_libpcap_cgo_flags,
    ninja_define_ebpf_compiler,
    ninja_define_exe_compiler,
)
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

is_windows = sys.platform == "win32"

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "security-agent", bin_name("security-agent"))
CI_PROJECT_DIR = os.environ.get("CI_PROJECT_DIR", ".")


@task(iterable=["build_tags"])
def build(
    ctx,
    build_tags,
    race=False,
    rebuild=False,
    install_path=None,
    major_version='7',
    go_mod="readonly",
    skip_assets=False,
    static=False,
    fips_mode=False,
):
    """
    Build the security agent
    """

    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, static=static, install_path=install_path)

    main = "main."
    ld_vars = {
        "Version": get_version(ctx, major_version=major_version),
        "GoVersion": get_go_version(),
        "GitBranch": get_current_branch(ctx),
        "GitCommit": get_commit_sha(ctx, short=True),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    ## build windows resources
    # generate windows resources
    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx, major_version=major_version)
        build_rc(
            ctx,
            "cmd/security-agent/windows_resources/security-agent.rc",
            vars=vars,
            out="cmd/security-agent/rsrc.syso",
        )

    ldflags += ' '.join([f"-X '{main + key}={value}'" for key, value in ld_vars.items()])
    build_tags += get_default_build_tags(build="security-agent")
    build_tags = add_fips_tags(build_tags, fips_mode)

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
        coverage=os.getenv("E2E_COVERAGE_PIPELINE") == "true",
    )

    render_config(ctx, env=env, skip_assets=skip_assets)


def render_config(ctx, env, skip_assets=False):
    if not skip_assets:
        dist_folder = os.path.join(BIN_DIR, "agent", "dist")
        generate_config(ctx, build_type="security-agent", output_file="./cmd/agent/dist/security-agent.yaml", env=env)
        if not os.path.exists(dist_folder):
            os.makedirs(dist_folder)
        shutil.copy("./cmd/agent/dist/security-agent.yaml", os.path.join(dist_folder, "security-agent.yaml"))


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
def gen_mocks(ctx):
    """
    Generate mocks.
    """
    ctx.run("mockery")


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


def ninja_ebpf_probe_syscall_tester(nw, build_dir):
    c_dir = os.path.join("pkg", "security", "tests", "syscall_tester", "c")
    c_file = os.path.join(c_dir, "ebpf_probe.c")
    o_file = os.path.join(build_dir, "ebpf_probe.o")
    uname_m = os.uname().machine

    nw.build(
        inputs=[c_file],
        outputs=[o_file],
        rule="ebpfclang",
        variables={
            "target": "-target bpf",
            "flags": [f"-D__{uname_m}__", f"-isystem/usr/include/{uname_m}-linux-gnu", "-DBPF_NO_GLOBAL_DATA"],
        },
    )


def build_go_syscall_tester(ctx, build_dir):
    syscall_tester_go_dir = os.path.join(".", "pkg", "security", "tests", "syscall_tester", "go")
    syscall_tester_exe_file = os.path.join(build_dir, "syscall_go_tester")

    go_build(
        ctx,
        f"{syscall_tester_go_dir}/syscall_go_tester.go",
        build_tags=["syscalltesters", "osusergo", "netgo"],
        ldflags="-extldflags=-static",
        bin_path=syscall_tester_exe_file,
    )
    return syscall_tester_exe_file


def ninja_c_syscall_tester_common(nw, file_name, build_dir, flags=None, libs=None, static=True, compiler='clang'):
    if flags is None:
        flags = []
    if libs is None:
        libs = []

    syscall_tester_c_dir = os.path.join("pkg", "security", "tests", "syscall_tester", "c")
    syscall_tester_c_file = os.path.join(syscall_tester_c_dir, f"{file_name}.c")
    syscall_tester_exe_file = os.path.join(build_dir, file_name)
    uname_m = os.uname().machine

    if static:
        flags.append("-static")

    nw.build(
        inputs=[syscall_tester_c_file],
        outputs=[syscall_tester_exe_file],
        rule="exe" + compiler,
        variables={
            "exeflags": flags,
            "exelibs": libs,
            "flags": [f"-isystem/usr/include/{uname_m}-linux-gnu"],
        },
    )
    return syscall_tester_exe_file


def ninja_syscall_x86_tester(ctx, build_dir, static=True, compiler='clang'):
    return ninja_c_syscall_tester_common(
        ctx, "syscall_x86_tester", build_dir, flags=["-m32"], static=static, compiler=compiler
    )


def ninja_syscall_tester(ctx, build_dir, static=True, compiler='clang'):
    return ninja_c_syscall_tester_common(
        ctx, "syscall_tester", build_dir, libs=["-lpthread"], static=static, compiler=compiler
    )


def create_dir_if_needed(dir):
    try:
        os.makedirs(dir)
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise


@task
def build_embed_syscall_tester(ctx, arch: str | Arch = CURRENT_ARCH, static=True, compiler="clang"):
    arch = Arch.from_str(arch)
    check_for_ninja(ctx)
    build_dir = os.path.join("pkg", "security", "tests", "syscall_tester", "bin")
    go_dir = os.path.join("pkg", "security", "tests", "syscall_tester", "go")
    create_dir_if_needed(build_dir)

    nf_path = os.path.join(ctx.cwd, 'syscall-tester.ninja')
    with open(nf_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file, width=120)
        ninja_define_ebpf_compiler(nw, arch=arch)
        ninja_define_exe_compiler(nw, compiler=compiler)

        ninja_syscall_tester(nw, build_dir, static=static, compiler=compiler)
        if arch == ARCH_AMD64:
            ninja_syscall_x86_tester(nw, build_dir, static=static, compiler=compiler)
        ninja_ebpf_probe_syscall_tester(nw, go_dir)

    ctx.run(f"ninja -f {nf_path}")
    build_go_syscall_tester(ctx, build_dir)


@task
def build_functional_tests(
    ctx,
    output='pkg/security/tests/testsuite',
    srcpath='pkg/security/tests',
    arch: str | Arch = CURRENT_ARCH,
    major_version='7',
    build_tags='functionaltests',
    build_flags='',
    bundle_ebpf=True,
    static=False,
    skip_linters=False,
    race=False,
    kernel_release=None,
    debug=False,
    skip_object_files=False,
    syscall_tester_compiler='clang',
):
    if not is_windows:
        if not skip_object_files:
            build_cws_object_files(
                ctx,
                major_version=major_version,
                arch=arch,
                kernel_release=kernel_release,
                debug=debug,
                bundle_ebpf=bundle_ebpf,
            )
        build_embed_syscall_tester(ctx, compiler=syscall_tester_compiler)

    arch = Arch.from_str(arch)
    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, static=static, arch=arch)

    env["CGO_ENABLED"] = "1"

    build_tags = build_tags.split(",")
    build_tags.append("test")
    if not is_windows:
        build_tags.append("linux_bpf")
        build_tags.append("trivy")
        build_tags.append("containerd")

        if bundle_ebpf:
            build_tags.append("ebpf_bindata")

        build_tags.append("pcap")
        build_libpcap(ctx)
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
            if result.exited != 0:
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

    ctx.run(cmd.format(**args), env=env)


@task
def functional_tests(
    ctx,
    verbose=False,
    race=False,
    major_version='7',
    output='pkg/security/tests/testsuite',
    bundle_ebpf=True,
    testflags='',
    skip_linters=False,
    kernel_release=None,
):
    build_functional_tests(
        ctx,
        major_version=major_version,
        output=output,
        bundle_ebpf=bundle_ebpf,
        skip_linters=skip_linters,
        race=race,
        kernel_release=kernel_release,
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
    major_version='7',
    output='pkg/security/tests/testsuite',
    bundle_ebpf=True,
    testflags='',
    skip_linters=False,
    kernel_release=None,
):
    build_functional_tests(
        ctx,
        major_version=major_version,
        output=output,
        bundle_ebpf=bundle_ebpf,
        skip_linters=skip_linters,
        race=race,
        kernel_release=kernel_release,
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
    major_version='7',
    testflags='',
    bundle_ebpf=True,
    skip_linters=False,
    kernel_release=None,
):
    build_functional_tests(
        ctx,
        major_version=major_version,
        output="pkg/security/tests/testsuite",
        bundle_ebpf=bundle_ebpf,
        static=True,
        skip_linters=skip_linters,
        race=race,
        kernel_release=kernel_release,
    )

    image_tag = "ghcr.io/datadog/apps-cws-centos7:main"

    container_name = 'security-agent-tests'
    capabilities = ['SYS_ADMIN', 'SYS_RESOURCE', 'SYS_PTRACE', 'NET_ADMIN', 'IPC_LOCK', 'ALL']

    cmd = 'docker run --name {container_name} {caps} --privileged -d '
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
    # secl docs
    secl_doc_gen.generate_secl_documentation(
        "./docs/cloud-workload-security/secl_linux.json",
        "./docs/cloud-workload-security/linux_expressions.md",
        "./linux_expressions.md",
    )
    secl_doc_gen.generate_secl_documentation(
        "./docs/cloud-workload-security/secl_windows.json",
        "./docs/cloud-workload-security/windows_expressions.md",
        "./windows_expressions.md",
    )
    # backend event docs
    backend_doc_gen.generate_backend_documentation(
        "./docs/cloud-workload-security/backend_linux.schema.json",
        "./docs/cloud-workload-security/backend_linux.md",
        "./backend_linux.md",
    )
    backend_doc_gen.generate_backend_documentation(
        "./docs/cloud-workload-security/backend_windows.schema.json",
        "./docs/cloud-workload-security/backend_windows.md",
        "./backend_windows.md",
    )


@task
def cws_go_generate(ctx, verbose=False):
    # run different `go generate` for pkg/security/secl and pkg/security
    ctx.run("go install golang.org/x/tools/cmd/stringer")
    ctx.run("go install github.com/mailru/easyjson/easyjson")
    ctx.run("go install github.com/DataDog/datadog-agent/pkg/security/generators/accessors")
    ctx.run("go install github.com/DataDog/datadog-agent/pkg/security/generators/operators")
    with ctx.cd("./pkg/security/secl"):
        if sys.platform == "linux":
            ctx.run("GOOS=windows go generate ./...")
        # Disable cross generation from windows for now. Need to fix the stringer issue.
        # elif sys.platform == "win32":
        #     ctx.run("set GOOS=linux && go generate ./...")
        cmd = "go generate"
        if verbose:
            cmd += " -v"
        ctx.run(cmd + " ./...")

    if sys.platform == "linux":
        shutil.copy(
            "./pkg/security/serializers/serializers_linux_easyjson.mock",
            "./pkg/security/serializers/serializers_linux_easyjson.go",
        )

    ctx.run("go generate -tags=linux_bpf,cws_go_generate ./pkg/security/...")

    # synchronize the seclwin package from the secl package
    sync_secl_win_pkg(ctx)

    # generate documentation
    generate_cws_documentation(ctx)


@task
def generate_syscall_table(ctx):
    def single_run(ctx, table_url, output_file, output_string_file, abis=None):
        if abis:
            abis = f"-abis {abis}"
        ctx.run(
            f"go run github.com/DataDog/datadog-agent/pkg/security/generators/syscall_table_generator -table-url {table_url} -output {output_file} -output-string {output_string_file} {abis}"
        )

    linux_version = "v6.13"
    single_run(
        ctx,
        f"https://raw.githubusercontent.com/torvalds/linux/{linux_version}/arch/x86/entry/syscalls/syscall_64.tbl",
        "pkg/security/secl/model/syscalls_linux_amd64.go",
        "pkg/security/secl/model/syscalls_string_linux_amd64.go",
        abis="common,64",
    )
    single_run(
        ctx,
        f"https://raw.githubusercontent.com/torvalds/linux/{linux_version}/include/uapi/asm-generic/unistd.h",
        "pkg/security/secl/model/syscalls_linux_arm64.go",
        "pkg/security/secl/model/syscalls_string_linux_arm64.go",
    )


DEFAULT_BTFHUB_CONSTANTS_PATH = "./pkg/security/probe/constantfetch/btfhub/constants.json"
DEFAULT_BTFHUB_CONSTANTS_ARM64_PATH = "./pkg/security/probe/constantfetch/btfhub/constants_arm64.json"
DEFAULT_BTFHUB_CONSTANTS_AMD64_PATH = "./pkg/security/probe/constantfetch/btfhub/constants_amd64.json"


@task
def generate_btfhub_constants(ctx, archive_path, output_path=DEFAULT_BTFHUB_CONSTANTS_PATH):
    ctx.run(
        f"go run -tags linux_bpf,btfhubsync ./pkg/security/probe/constantfetch/btfhub/ -archive-root {archive_path} -output {output_path}",
    )


@task
def combine_btfhub_constants(ctx, archive_path, output_path=DEFAULT_BTFHUB_CONSTANTS_PATH):
    ctx.run(
        f"go run -tags linux_bpf,btfhubsync ./pkg/security/probe/constantfetch/btfhub/ -combine -archive-root {archive_path} -output {output_path}",
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
    with tempfile.TemporaryDirectory() as temp_gobin:
        with environ({"GOBIN": temp_gobin}):
            ctx.run("go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.3")
            ctx.run("go install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@v0.6.0")
            ctx.run("go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1")

            plugin_opts = " ".join(
                [
                    f"--plugin protoc-gen-go=\"{temp_gobin}/protoc-gen-go\"",
                    f"--plugin protoc-gen-go-grpc=\"{temp_gobin}/protoc-gen-go-grpc\"",
                    f"--plugin protoc-gen-go-vtproto=\"{temp_gobin}/protoc-gen-go-vtproto\"",
                ]
            )

            # API
            ctx.run(
                f"protoc -I. {plugin_opts} --go_out=paths=source_relative:. --go-vtproto_out=. --go-vtproto_opt=features=marshal+unmarshal+size --go-grpc_out=paths=source_relative:. pkg/security/proto/api/api.proto"
            )
            ctx.run(
                f"protoc -I. {plugin_opts} --go_out=paths=source_relative:. --go-vtproto_out=. --go-vtproto_opt=features=marshal+unmarshal+size --go-grpc_out=paths=source_relative:. pkg/eventmonitor/proto/api/api.proto"
            )

    security_files = glob.glob("pkg/security/**/*.pb.go", recursive=True)
    eventmonitor_files = glob.glob("pkg/eventmonitor/**/*.pb.go", recursive=True)
    for path in chain(security_files, eventmonitor_files):
        print(f"replacing protoc version in {path}")
        with open(path) as f:
            content = f.read()

        replaced_content = re.sub(r"\/\/\s*protoc\s*v\d+\.\d+\.\d+", "//  protoc", content)
        replaced_content = re.sub(r"\/\/\s*-\s+protoc\s*v\d+\.\d+\.\d+", "// - protoc", replaced_content)
        with open(path, "w") as f:
            f.write(replaced_content)


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


class FailingTask:
    def __init__(self, name, dirty_files):
        self.name = name
        self.dirty_files = dirty_files


@task
def go_generate_check(ctx):
    tasks = [
        [cws_go_generate],
        [gen_mocks],
    ]
    failing_tasks = []
    previous_dirty = set()

    for task_entry in tasks:
        task, args = task_entry[0], task_entry[1:]
        task(ctx, *args)
        # when running a non-interactive session, python may buffer too much data and thus mix stderr and stdout
        # this is especially visible in the Gitlab job logs
        # we flush to ensure correct separation between steps
        sys.stdout.flush()
        sys.stderr.flush()
        dirty_files = [f for f in get_git_dirty_files() if f not in previous_dirty]
        if dirty_files:
            failing_tasks.append(FailingTask(task.__name__, dirty_files))

        previous_dirty.update(dirty_files)

    if failing_tasks:
        for ft in failing_tasks:
            task = ft.name.replace("_", "-")
            print(f"Task `dda inv {task}` resulted in dirty files, please re-run it:")
            for file in ft.dirty_files:
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
        debug=True,
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
        debug=True,
        skip_linters=True,
    )


@task
def run_ebpf_unit_tests(ctx, verbose=False, trace=False, testflags=''):
    build_cws_object_files(
        ctx, major_version='7', kernel_release=None, with_unit_test=True, bundle_ebpf=True, arch=CURRENT_ARCH
    )

    env = {"CGO_ENABLED": "1"}

    build_libpcap(ctx)
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

    ctx.run(f"go test {flags} ./pkg/security/ebpf/tests/... {args} {testflags}", env=env)


@task
def print_fentry_stats(ctx):
    fentry_o_path = "pkg/ebpf/bytecode/build/runtime-security-fentry.o"

    for kind in ["kprobe", "kretprobe", "fentry", "fexit"]:
        ctx.run(f"readelf -W -S {fentry_o_path} 2> /dev/null | grep PROGBITS | grep {kind} | wc -l")


@task
def sync_secl_win_pkg(ctx):
    files_to_copy = [
        ("model.go", None),
        ("events.go", None),
        ("args_envs.go", None),
        ("consts_common.go", None),
        ("consts_windows.go", "consts_win.go"),
        ("model_windows.go", "model_win.go"),
        ("field_handlers_windows.go", "field_handlers_win.go"),
        ("accessors_helpers.go", None),
        ("accessors_windows.go", "accessors_win.go"),
        ("legacy_secl.go", None),
        ("security_profile.go", None),
        ("iterator.go", None),
    ]

    ctx.run("rm -r pkg/security/seclwin/model")
    ctx.run("mkdir -p pkg/security/seclwin/model")
    ctx.run("cp pkg/security/secl/doc.go pkg/security/seclwin/doc.go")

    for ffrom, fto in files_to_copy:
        if not fto:
            fto = ffrom

        ctx.run(f"cp pkg/security/secl/model/{ffrom} pkg/security/seclwin/model/{fto}")
        if sys.platform == "darwin":
            ctx.run(f"sed -i '' '/^\\/\\/go:build/d' pkg/security/seclwin/model/{fto}")
        else:
            ctx.run(f"sed -i '/^\\/\\/go:build/d' pkg/security/seclwin/model/{fto}")
        ctx.run(f"gofmt -s -w pkg/security/seclwin/model/{fto}")

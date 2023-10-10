import datetime
import errno
import glob
import json
import os
import re
import shutil
import sys
import tarfile
import tempfile
from subprocess import check_output

from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_default_build_tags
from .go import run_golangci_lint
from .libs.ninja_syntax import NinjaWriter
from .system_probe import (
    CURRENT_ARCH,
    build_cws_object_files,
    check_for_ninja,
    ninja_define_ebpf_compiler,
    ninja_define_exe_compiler,
)
from .test import environ
from .utils import (
    REPO_PATH,
    bin_name,
    generate_config,
    get_build_flags,
    get_git_branch_name,
    get_git_commit,
    get_go_version,
    get_gopath,
    get_version,
)
from .windows_resources import build_messagetable, build_rc, versioninfo_vars

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "security-agent", bin_name("security-agent"))
CI_PROJECT_DIR = os.environ.get("CI_PROJECT_DIR", ".")
KITCHEN_DIR = os.getenv('DD_AGENT_TESTING_DIR') or os.path.normpath(os.path.join(os.getcwd(), "test", "kitchen"))
KITCHEN_ARTIFACT_DIR = os.path.join(KITCHEN_DIR, "site-cookbooks", "dd-security-agent-check", "files")
STRESS_TEST_SUITE = "stresssuite"


@task(iterable=["build_tags"])
def build(
    ctx,
    build_tags,
    race=False,
    incremental_build=True,
    major_version='7',
    # arch is never used here; we keep it to have a
    # consistent CLI on the build task for all agents.
    arch=CURRENT_ARCH,  # noqa: U100
    go_mod="mod",
    skip_assets=False,
    static=False,
):
    """
    Build the security agent
    """
    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, python_runtimes='3', static=static)

    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx, major_version=major_version),
        "GoVersion": get_go_version(),
        "GitBranch": get_git_branch_name(),
        "GitCommit": get_git_commit(),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    ## build windows resources
    # generate windows resources
    if sys.platform == 'win32':
        if arch == "x86":
            env["GOARCH"] = "386"

        build_messagetable(ctx, arch=arch)
        vars = versioninfo_vars(ctx, major_version=major_version, arch=arch)
        build_rc(
            ctx,
            "cmd/security-agent/windows_resources/security-agent.rc",
            arch=arch,
            vars=vars,
            out="cmd/security-agent/rsrc.syso",
        )

    ldflags += ' '.join([f"-X '{main + key}={value}'" for key, value in ld_vars.items()])
    build_tags += get_default_build_tags(
        build="security-agent"
    )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?

    # TODO static option
    cmd = 'go build -mod={go_mod} {race_opt} {build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/security-agent'

    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "" if incremental_build else "-a",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": BIN_PATH,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)

    if not skip_assets:
        dist_folder = os.path.join(BIN_DIR, "agent", "dist")
        generate_config(ctx, build_type="security-agent", output_file="./cmd/agent/dist/security-agent.yaml", env=env)
        if not os.path.exists(dist_folder):
            os.makedirs(dist_folder)
        shutil.copy("./cmd/agent/dist/security-agent.yaml", os.path.join(dist_folder, "security-agent.yaml"))


@task()
def gen_mocks(ctx):
    """
    Generate mocks.
    """
    ctx.run("mockery")


@task
def run_functional_tests(ctx, testsuite, verbose=False, testflags='', fentry=False):
    cmd = '{testsuite} {verbose_opt} {testflags}'
    if fentry:
        cmd = "DD_EVENT_MONITORING_CONFIG_EVENT_STREAM_USE_FENTRY=true " + cmd

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
    ctx.run(f"go build -o {syscall_tester_exe_file} -tags syscalltesters {syscall_tester_go_dir}/syscall_go_tester.go ")
    return syscall_tester_exe_file


def ninja_c_syscall_tester_common(nw, file_name, build_dir, flags=None, libs=None, static=True):
    if flags is None:
        flags = []
    if libs is None:
        libs = []

    syscall_tester_c_dir = os.path.join("pkg", "security", "tests", "syscall_tester", "c")
    syscall_tester_c_file = os.path.join(syscall_tester_c_dir, f"{file_name}.c")
    syscall_tester_exe_file = os.path.join(build_dir, file_name)

    if static:
        flags.append("-static")

    nw.build(
        inputs=[syscall_tester_c_file],
        outputs=[syscall_tester_exe_file],
        rule="execlang",
        variables={
            "exeflags": flags,
            "exelibs": libs,
        },
    )
    return syscall_tester_exe_file


def ninja_c_latency_common(nw, file_name, build_dir, flags=None, libs=None, static=True):
    if flags is None:
        flags = []
    if libs is None:
        libs = []

    latency_c_dir = os.path.join("pkg", "security", "tests", "latency", "c")
    latency_c_file = os.path.join(latency_c_dir, f"{file_name}.c")
    latency_exe_file = os.path.join(build_dir, file_name)

    if static:
        flags.append("-static")

    nw.build(
        inputs=[latency_c_file],
        outputs=[latency_exe_file],
        rule="execlang",
        variables={"exeflags": flags, "exelibs": libs},
    )
    return latency_exe_file


def ninja_latency_tools(ctx, build_dir, static=True):
    return ninja_c_latency_common(ctx, "bench_net_DNS", build_dir, libs=["-lpthread"], static=static)


@task
def build_embed_latency_tools(ctx, static=True):
    check_for_ninja(ctx)
    build_dir = os.path.join("pkg", "security", "tests", "latency", "bin")
    create_dir_if_needed(build_dir)

    nf_path = os.path.join(ctx.cwd, 'latency-tools.ninja')
    with open(nf_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file, width=120)
        ninja_define_exe_compiler(nw)
        ninja_latency_tools(nw, build_dir, static=static)

    ctx.run(f"ninja -f {nf_path}")


def ninja_syscall_x86_tester(ctx, build_dir, static=True):
    return ninja_c_syscall_tester_common(ctx, "syscall_x86_tester", build_dir, flags=["-m32"], static=static)


def ninja_syscall_tester(ctx, build_dir, static=True):
    return ninja_c_syscall_tester_common(ctx, "syscall_tester", build_dir, libs=["-lpthread"], static=static)


def create_dir_if_needed(dir):
    try:
        os.makedirs(dir)
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise


@task
def build_embed_syscall_tester(ctx, arch=CURRENT_ARCH, static=True):
    check_for_ninja(ctx)
    build_dir = os.path.join("pkg", "security", "tests", "syscall_tester", "bin")
    go_dir = os.path.join("pkg", "security", "tests", "syscall_tester", "go")
    create_dir_if_needed(build_dir)

    nf_path = os.path.join(ctx.cwd, 'syscall-tester.ninja')
    with open(nf_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file, width=120)
        ninja_define_ebpf_compiler(nw)
        ninja_define_exe_compiler(nw)

        ninja_syscall_tester(nw, build_dir, static=static)
        if arch == "x64":
            ninja_syscall_x86_tester(nw, build_dir, static=static)
        ninja_ebpf_probe_syscall_tester(nw, go_dir)

    ctx.run(f"ninja -f {nf_path}")
    build_go_syscall_tester(ctx, build_dir)


@task
def build_functional_tests(
    ctx,
    output='pkg/security/tests/testsuite',
    arch=CURRENT_ARCH,
    major_version='7',
    build_tags='functionaltests',
    build_flags='',
    bundle_ebpf=True,
    static=False,
    skip_linters=False,
    race=False,
    kernel_release=None,
    debug=False,
):
    build_cws_object_files(
        ctx,
        major_version=major_version,
        arch=arch,
        kernel_release=kernel_release,
        debug=debug,
    )

    build_embed_syscall_tester(ctx)

    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, static=static)

    env["CGO_ENABLED"] = "1"
    if arch == "x86":
        env["GOARCH"] = "386"

    build_tags = build_tags.split(",")
    build_tags.append("linux_bpf")
    build_tags.append("trivy")
    build_tags.append("containerd")

    if bundle_ebpf:
        build_tags.append("ebpf_bindata")

    if static:
        build_tags.extend(["osusergo", "netgo"])

    if not skip_linters:
        targets = ['./pkg/security/tests']
        results = run_golangci_lint(ctx, targets=targets, build_tags=build_tags, arch=arch)
        for result in results:
            # golangci exits with status 1 when it finds an issue
            if result.exited != 0:
                raise Exit(code=1)
        print("golangci-lint found no issues")

    if race:
        build_flags += " -race"

    build_tags = ",".join(build_tags)
    cmd = 'go test -mod=mod -tags {build_tags} -gcflags="{gcflags}" -ldflags="{ldflags}" -c -o {output} '
    cmd += '{build_flags} {repo_path}/pkg/security/tests'

    args = {
        "output": output,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "build_flags": build_flags,
        "build_tags": build_tags,
        "repo_path": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)


@task
def build_stress_tests(
    ctx,
    output=f"pkg/security/tests/{STRESS_TEST_SUITE}",
    arch=CURRENT_ARCH,
    major_version='7',
    bundle_ebpf=True,
    skip_linters=False,
    kernel_release=None,
):
    build_embed_latency_tools(ctx)
    build_functional_tests(
        ctx,
        output=output,
        arch=arch,
        major_version=major_version,
        build_tags='stresstests',
        bundle_ebpf=bundle_ebpf,
        skip_linters=skip_linters,
        kernel_release=kernel_release,
    )


@task
def stress_tests(
    ctx,
    verbose=False,
    arch=CURRENT_ARCH,
    major_version='7',
    output=f"pkg/security/tests/{STRESS_TEST_SUITE}",
    bundle_ebpf=True,
    testflags='',
    skip_linters=False,
    kernel_release=None,
):
    build_stress_tests(
        ctx,
        arch=arch,
        major_version=major_version,
        output=output,
        bundle_ebpf=bundle_ebpf,
        skip_linters=skip_linters,
        kernel_release=kernel_release,
    )

    run_functional_tests(
        ctx,
        testsuite=output,
        verbose=verbose,
        testflags=testflags,
    )


@task
def functional_tests(
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
    fentry=False,
):
    build_functional_tests(
        ctx,
        arch=arch,
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
        fentry=fentry,
    )


@task
def kitchen_functional_tests(
    ctx,
    verbose=False,
    arch=CURRENT_ARCH,
    major_version='7',
    build_tests=False,
    testflags='',
):
    if build_tests:
        functional_tests(
            ctx,
            verbose=verbose,
            arch=arch,
            major_version=major_version,
            output="test/kitchen/site-cookbooks/dd-security-agent-check/files/testsuite",
            testflags=testflags,
        )

    kitchen_dir = os.path.join("test", "kitchen")
    shutil.copy(
        os.path.join(kitchen_dir, "kitchen-vagrant-security-agent.yml"), os.path.join(kitchen_dir, "kitchen.yml")
    )

    with ctx.cd(kitchen_dir):
        ctx.run("kitchen test")


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
        arch=arch,
        major_version=major_version,
        output="pkg/security/tests/testsuite",
        bundle_ebpf=bundle_ebpf,
        static=True,
        skip_linters=skip_linters,
        race=race,
        kernel_release=kernel_release,
    )

    add_arch_line = ""
    if arch == "x86":
        add_arch_line = "RUN dpkg --add-architecture i386"

    dockerfile = f"""
FROM ubuntu:22.04

ENV DOCKER_DD_AGENT=yes

{add_arch_line}

RUN apt-get update -y \
    && apt-get install -y --no-install-recommends xfsprogs ca-certificates iproute2 clang-14 llvm-14 \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/datadog-agent/embedded/bin
RUN ln -s $(which clang-14) /opt/datadog-agent/embedded/bin/clang-bpf
RUN ln -s $(which llc-14) /opt/datadog-agent/embedded/bin/llc-bpf
    """

    docker_image_tag_name = "docker-functional-tests"

    # build docker image
    with tempfile.TemporaryDirectory() as temp_dir:
        print("Create tmp dir:", temp_dir)
        with open(os.path.join(temp_dir, "Dockerfile"), "w") as f:
            f.write(dockerfile)

        cmd = 'docker build {docker_file_ctx} --tag {image_tag}'
        ctx.run(cmd.format(**{"docker_file_ctx": temp_dir, "image_tag": docker_image_tag_name}))

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
    cmd += '-v {GOPATH}/src/{REPO_PATH}/pkg/security/tests:/tests {image_tag} sleep 3600'

    args = {
        "GOPATH": get_gopath(ctx),
        "REPO_PATH": REPO_PATH,
        "container_name": container_name,
        "caps": ' '.join(f"--cap-add {cap}" for cap in capabilities),
        "image_tag": f"{docker_image_tag_name}:latest",
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
def generate_cws_documentation(ctx, go_generate=False):
    if go_generate:
        cws_go_generate(ctx)

    # secl docs
    ctx.run(
        "python3 ./docs/cloud-workload-security/scripts/secl-doc-gen.py --input ./docs/cloud-workload-security/secl.json --output ./docs/cloud-workload-security/agent_expressions.md"
    )
    # backend event docs
    ctx.run(
        "python3 ./docs/cloud-workload-security/scripts/backend-doc-gen.py --input ./docs/cloud-workload-security/backend.schema.json --output ./docs/cloud-workload-security/backend.md"
    )


@task
def cws_go_generate(ctx):
    with ctx.cd("./pkg/security/secl"):
        ctx.run("go generate ./...")

    if sys.platform == "win32":
        shutil.copy(
            "./pkg/security/serializers/serializers_windows_easyjson.mock",
            "./pkg/security/serializers/serializers_windows_easyjson.go",
        )
    else:
        shutil.copy(
            "./pkg/security/serializers/serializers_linux_easyjson.mock",
            "./pkg/security/serializers/serializers_linux_easyjson.go",
        )

        shutil.copy(
            "./pkg/security/security_profile/dump/activity_dump_easyjson.mock",
            "./pkg/security/security_profile/dump/activity_dump_easyjson.go",
        )

    ctx.run("go generate ./pkg/security/...")


@task
def generate_syscall_table(ctx):
    def single_run(ctx, table_url, output_file, output_string_file, abis=None):
        if abis:
            abis = f"-abis {abis}"
        ctx.run(
            f"go run github.com/DataDog/datadog-agent/pkg/security/secl/model/syscall_table_generator -table-url {table_url} -output {output_file} -output-string {output_string_file} {abis}"
        )

    linux_version = "v6.1"
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


@task
def generate_btfhub_constants(ctx, archive_path, force_refresh=False):
    output_path = "./pkg/security/probe/constantfetch/btfhub/constants.json"
    force_refresh_opt = "-force-refresh" if force_refresh else ""
    ctx.run(
        f"go run -tags linux_bpf,btfhubsync ./pkg/security/probe/constantfetch/btfhub/ -archive-root {archive_path} -output {output_path} {force_refresh_opt}",
    )


@task
def generate_cws_proto(ctx):
    with tempfile.TemporaryDirectory() as temp_gobin:
        with environ({"GOBIN": temp_gobin}):
            ctx.run("go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1")
            ctx.run("go install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@v0.4.0")
            ctx.run("go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0")

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

    for path in glob.glob("pkg/security/**/*.pb.go", recursive=True):
        print(f"replacing protoc version in {path}")
        with open(path) as f:
            content = f.read()

        replaced_content = re.sub(r"\/\/\s*protoc\s*v\d+\.\d+\.\d+", "//  protoc", content)
        with open(path, "w") as f:
            f.write(replaced_content)


def get_git_dirty_files():
    dirty_stats = check_output(["git", "status", "--porcelain=v1"]).decode('utf-8')
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
        [generate_cws_documentation],
        [gen_mocks],
    ]
    failing_tasks = []

    for task_entry in tasks:
        task, args = task_entry[0], task_entry[1:]
        task(ctx, *args)
        # when running a non-interactive session, python may buffer too much data and thus mix stderr and stdout
        # this is especially visible in the Gitlab job logs
        # we flush to ensure correct separation between steps
        sys.stdout.flush()
        sys.stderr.flush()
        dirty_files = get_git_dirty_files()
        if dirty_files:
            failing_tasks.append(FailingTask(task.__name__, dirty_files))

    if failing_tasks:
        for ft in failing_tasks:
            print(f"Task `{ft.name}` resulted in dirty files, please re-run it:")
            for file in ft.dirty_files:
                print(f"* {file}")
            raise Exit(code=1)


@task
def kitchen_prepare(ctx, skip_linters=False):
    """
    Compile test suite for kitchen
    """

    # Clean up previous build
    if os.path.exists(KITCHEN_ARTIFACT_DIR):
        shutil.rmtree(KITCHEN_ARTIFACT_DIR)

    testsuite_out_path = os.path.join(KITCHEN_ARTIFACT_DIR, "tests", "testsuite")
    build_functional_tests(
        ctx,
        bundle_ebpf=False,
        race=True,
        debug=True,
        output=testsuite_out_path,
        skip_linters=skip_linters,
    )
    stresssuite_out_path = os.path.join(KITCHEN_ARTIFACT_DIR, "tests", STRESS_TEST_SUITE)
    build_stress_tests(ctx, output=stresssuite_out_path, skip_linters=skip_linters)

    # Copy clang binaries
    for bin in ["clang-bpf", "llc-bpf"]:
        ctx.run(f"cp /opt/datadog-agent/embedded/bin/{bin} {KITCHEN_ARTIFACT_DIR}/{bin}")

    # Copy gotestsum binary
    gopath = get_gopath(ctx)
    ctx.run(f"cp {gopath}/bin/gotestsum {KITCHEN_ARTIFACT_DIR}/")

    # Build test2json binary
    ctx.run(f"go build -o {KITCHEN_ARTIFACT_DIR}/test2json -ldflags=\"-s -w\" cmd/test2json", env={"CGO_ENABLED": "0"})

    ebpf_bytecode_dir = os.path.join(KITCHEN_ARTIFACT_DIR, "ebpf_bytecode")
    ebpf_runtime_dir = os.path.join(ebpf_bytecode_dir, "runtime")
    bytecode_build_dir = os.path.join(CI_PROJECT_DIR, "pkg", "ebpf", "bytecode", "build")

    ctx.run(f"mkdir -p {ebpf_runtime_dir}")
    ctx.run(f"cp {bytecode_build_dir}/runtime-security* {ebpf_bytecode_dir}")
    ctx.run(f"cp {bytecode_build_dir}/runtime/runtime-security* {ebpf_runtime_dir}")


@task
def run_ebpf_unit_tests(ctx, verbose=False, trace=False):
    build_cws_object_files(
        ctx,
        major_version='7',
        arch=CURRENT_ARCH,
        kernel_release=None,
        with_unit_test=True,
    )

    flags = '-tags ebpf_bindata'
    if verbose:
        flags += " -test.v"

    args = '-args'
    if trace:
        args += " -trace"

    ctx.run(f"go test {flags} ./pkg/security/ebpf/tests/... {args}")


# TODO: this task does the same thing as system-probe.print-failed-tests.
# They could be refactored to have the logic in only one place.
@task
def print_failed_tests(_, output_dir):
    """
    print_failed_tests is run at the end of the platform functional test Gitlab job to print failed tests
    """
    fail_count = 0
    for testjson_tgz in glob.glob(f"{output_dir}/**/testjson.tar.gz"):
        test_platform = os.path.basename(os.path.dirname(testjson_tgz))

        if os.path.isdir(testjson_tgz):
            # handle weird kitchen bug where it places the tarball in a subdirectory of the same name
            testjson_tgz = os.path.join(testjson_tgz, "testjson.tar.gz")

        with tempfile.TemporaryDirectory() as unpack_dir:
            with tarfile.open(testjson_tgz) as tgz:
                tgz.extractall(path=unpack_dir)

            for test_json in glob.glob(f"{unpack_dir}/*.json"):
                bundle, _ = os.path.splitext(os.path.basename(test_json))
                with open(test_json) as tf:
                    for line in tf:
                        json_test = json.loads(line.strip())
                        if 'Test' in json_test:
                            name = json_test['Test']
                            action = json_test["Action"]

                            if action == "fail":
                                print(f"FAIL: [{test_platform}] [{bundle}] {name}")
                                fail_count += 1

    if fail_count > 0:
        raise Exit(code=1)


@task
def print_fentry_stats(ctx):
    fentry_o_path = "pkg/ebpf/bytecode/build/runtime-security-fentry.o"

    for kind in ["kprobe", "kretprobe", "fentry", "fexit"]:
        ctx.run(f"readelf -W -S {fentry_o_path} 2> /dev/null | grep PROGBITS | grep {kind} | wc -l")

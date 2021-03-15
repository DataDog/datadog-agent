import contextlib
import datetime
import glob
import os
import shutil
import sys
import tempfile
from subprocess import CalledProcessError, check_output

from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_default_build_tags
from .utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
    get_git_branch_name,
    get_git_commit,
    get_go_version,
    get_version,
    get_version_numeric_only,
)

BIN_DIR = os.path.join(".", "bin", "system-probe")
BIN_PATH = os.path.join(BIN_DIR, bin_name("system-probe", android=False))

EBPF_BUILDER_IMAGE = 'datadog/tracer-bpf-builder'
EBPF_BUILDER_FILE = os.path.join(".", "tools", "ebpf", "Dockerfiles", "Dockerfile-ebpf")

BPF_TAG = "linux_bpf"
BCC_TAG = "bcc"
GIMME_ENV_VARS = ['GOROOT', 'PATH']

DATADOG_AGENT_EMBEDDED_PATH = '/opt/datadog-agent/embedded'

KITCHEN_DIR = os.path.join("test", "kitchen")
KITCHEN_ARTIFACT_DIR = os.path.join(KITCHEN_DIR, "site-cookbooks", "dd-system-probe-check", "files", "default", "tests")
TEST_PACKAGES_LIST = ["./pkg/ebpf/...", "./pkg/network/..."]
TEST_PACKAGES = " ".join(TEST_PACKAGES_LIST)

is_windows = sys.platform == "win32"


@task
def build(
    ctx,
    race=False,
    go_version=None,
    incremental_build=False,
    major_version='7',
    python_runtimes='3',
    with_bcc=True,
    go_mod="mod",
    windows=is_windows,
    arch="x64",
    embedded_path=DATADOG_AGENT_EMBEDDED_PATH,
    bundle_ebpf=False,
):
    """
    Build the system_probe
    """

    # Only build ebpf files on unix
    if not windows:
        build_object_files(ctx, bundle_ebpf=bundle_ebpf)

    ldflags, gcflags, env = get_build_flags(
        ctx, major_version=major_version, python_runtimes=python_runtimes, embedded_path=embedded_path
    )

    # generate windows resources
    if sys.platform == 'win32':
        windres_target = "pe-x86-64"
        if arch == "x86":
            print("system probe not supported on x86")
            raise

        ver = get_version_numeric_only(ctx, major_version=major_version)
        maj_ver, min_ver, patch_ver = ver.split(".")
        resdir = os.path.join(".", "cmd", "system-probe", "windows_resources")

        ctx.run(
            "windmc --target {target_arch} -r {resdir} {resdir}/system-probe-msg.mc".format(
                resdir=resdir, target_arch=windres_target
            )
        )

        ctx.run(
            "windres "
            "--define MAJ_VER={maj_ver} "
            "--define MIN_VER={min_ver} "
            "--define PATCH_VER={patch_ver} "
            "-i cmd/system-probe/windows_resources/system-probe.rc "
            "--target {target_arch} "
            "-O coff "
            "-o cmd/system-probe/rsrc.syso".format(
                maj_ver=maj_ver, min_ver=min_ver, patch_ver=patch_ver, target_arch=windres_target
            )
        )
    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx, major_version=major_version),
        "GoVersion": get_go_version(),
        "GitBranch": get_git_branch_name(),
        "GitCommit": get_git_commit(),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    goenv = {}
    if go_version:
        lines = ctx.run("gimme {version}".format(version=go_version)).stdout.split("\n")
        for line in lines:
            for env_var in GIMME_ENV_VARS:
                if env_var in line:
                    goenv[env_var] = line[line.find(env_var) + len(env_var) + 1 : -1].strip('\'\"')
        ld_vars["GoVersion"] = go_version

    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]
    env.update(goenv)

    # Add custom ld flags
    ldflags += ' '.join(["-X '{name}={value}'".format(name=main + key, value=value) for key, value in ld_vars.items()])

    build_tags = get_default_build_tags(build="system-probe", arch=arch)
    if bundle_ebpf:
        build_tags.append("ebpf_bindata")

    if with_bcc:
        build_tags.append(BCC_TAG)

    # TODO static option
    cmd = 'go build -mod={go_mod} {race_opt} {build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/system-probe'

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


@task
def build_in_docker(
    ctx, rebuild_ebpf_builder=False, race=False, incremental_build=False, major_version='7', bundle_ebpf=False
):
    """
    Build the system_probe using a container
    This can be used when the current OS don't have up to date linux headers
    """

    if rebuild_ebpf_builder:
        build_ebpf_builder(ctx)

    docker_cmd = "docker run --rm \
            -v {cwd}:/go/src/github.com/DataDog/datadog-agent \
            --workdir=/go/src/github.com/DataDog/datadog-agent \
            {builder} \
            {cmd}"

    if should_use_sudo(ctx):
        docker_cmd = "sudo " + docker_cmd

    cmd = "invoke -e system-probe.build --major-version {}".format(major_version)

    if race:
        cmd += " --race"
    if incremental_build:
        cmd += " --incremental-build"
    if bundle_ebpf:
        cmd += " --bundle-ebpf"

    ctx.run(docker_cmd.format(cwd=os.getcwd(), builder=EBPF_BUILDER_IMAGE, cmd=cmd))


@task
def test(
    ctx,
    packages=TEST_PACKAGES,
    skip_object_files=False,
    bundle_ebpf=True,
    output_path=None,
    runtime_compiled=False,
    skip_linters=False,
):
    """
    Run tests on eBPF parts
    If skip_object_files is set to True, this won't rebuild object files
    If output_path is set, we run `go test` with the flags `-c -o output_path`, which *compiles* the test suite
    into a single binary. This artifact is meant to be used in conjunction with kitchen tests.
    """

    if not skip_linters:
        cfmt(ctx)
        ctidy(ctx)

    if not skip_object_files:
        build_object_files(ctx, bundle_ebpf=bundle_ebpf)

    cmd = 'go test -mod=mod -v -tags {bpf_tag} {output_params} {pkgs}'
    if not is_root():
        cmd = 'sudo -E PATH={path} ' + cmd

    bpf_tag = BPF_TAG
    # temporary measure until we have a good default for BPFDir for testing
    bpf_tag += ",ebpf_bindata"
    if os.getenv("GOPATH") is None:
        print(
            "GOPATH is not set, if you are running tests with sudo, you may need to use the -E option to preserve "
            "your environment "
        )
        raise Exit(code=1)

    args = {
        "path": os.environ['PATH'],
        "bpf_tag": bpf_tag,
        "output_params": "-c -o " + output_path if output_path else "",
        "pkgs": packages,
    }

    env = {'CGO_LDFLAGS_ALLOW': "-Wl,--wrap=.*"}
    if runtime_compiled:
        env['DD_TESTS_RUNTIME_COMPILED'] = "1"

    ctx.run(cmd.format(**args), env=env)


@task
def kitchen_prepare(ctx):
    """
    Compile test suite for kitchen
    """

    # Clean up previous build
    if os.path.exists(KITCHEN_ARTIFACT_DIR):
        shutil.rmtree(KITCHEN_ARTIFACT_DIR)

    # Retrieve a list of all packages we want to test
    # This handles the elipsis notation (eg. ./pkg/ebpf/...)
    target_packages = []
    for pkg in TEST_PACKAGES_LIST:
        target_packages += (
            check_output("go list -f '{{ .Dir }}' -tags linux_bpf %s" % pkg, shell=True)
            .decode('utf-8')
            .strip()
            .split("\n")
        )

    # This will compile one 'testsuite' file per package by running `go test -c -o output_path`.
    # These artifacts will be "vendored" inside a chef recipe like the following:
    # test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg/network/testsuite
    # test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg/network/netlink/testsuite
    # test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg/ebpf/testsuite
    # test/kitchen/site-cookbooks/dd-system-probe-check/files/default/tests/pkg/ebpf/bytecode/testsuite
    for i, pkg in enumerate(target_packages):
        relative_path = os.path.relpath(pkg)
        target_path = os.path.join(KITCHEN_ARTIFACT_DIR, relative_path)

        test(
            ctx,
            packages=pkg,
            skip_object_files=(i != 0),
            skip_linters=True,
            bundle_ebpf=False,
            output_path=os.path.join(target_path, "testsuite"),
        )

        # copy ancillary data, if applicable
        for extra in ["testdata", "build"]:
            extra_path = os.path.join(pkg, extra)
            if os.path.isdir(extra_path):
                shutil.copytree(extra_path, os.path.join(target_path, extra))


@task
def kitchen_test(ctx):
    """
    Run tests (locally) using chef kitchen against an array of different platforms.
    * Make sure to run `inv -e system-probe.kitchen-prepare` using the agent-development VM;
    * Then we recommend to run `inv -e system-probe.kitchen-test` directly from your (macOS) machine;
    """
    with ctx.cd(KITCHEN_DIR):
        ctx.run("kitchen test", env={"KITCHEN_YAML": "kitchen-vagrant-system-probe.yml"})


@task
def nettop(ctx, incremental_build=False, go_mod="mod"):
    """
    Build and run the `nettop` utility for testing
    """
    build_object_files(ctx, bundle_ebpf=False)

    cmd = 'go build -mod={go_mod} {build_type} -tags linux_bpf,ebpf_bindata -o {bin_path} {path}'
    bin_path = os.path.join(BIN_DIR, "nettop")
    # Build
    ctx.run(
        cmd.format(
            path=os.path.join(REPO_PATH, "pkg", "ebpf", "nettop"),
            bin_path=bin_path,
            go_mod=go_mod,
            build_type="" if incremental_build else "-a",
        )
    )

    # Run
    if should_use_sudo(ctx):
        ctx.sudo(bin_path)
    else:
        ctx.run(bin_path)


@task
def cfmt(ctx, fix=False, fail_on_issue=False):
    """
    Format C code using clang-format
    """
    print("checking for clang-format executable...")
    ctx.run("which clang-format")
    print("found clang-format")

    fmt_cmd = "clang-format -i --style=file --fallback-style=none"
    if not fix:
        fmt_cmd = fmt_cmd + " --dry-run"
    if fail_on_issue:
        fmt_cmd = fmt_cmd + " --Werror"

    files = get_ebpf_targets()
    # remove externally maintained files
    files.remove("pkg/ebpf/c/bpf_helpers.h")
    files.remove("pkg/ebpf/c/bpf_endian.h")

    ctx.run("{cmd} {files}".format(cmd=fmt_cmd, files=" ".join(files)))


@task
def ctidy(ctx, fix=False, fail_on_issue=False):
    """
    Lint C code using clang-tidy
    """

    print("checking for clang-tidy executable...")
    ctx.run("which clang-tidy")
    print("found clang-tidy")

    build_flags = get_ebpf_build_flags()
    build_flags.append("-DDEBUG=1")

    bpf_dir = os.path.join(".", "pkg", "ebpf")
    base_files = glob.glob(bpf_dir + "/c/**/*.c")

    network_bpf_dir = os.path.join(".", "pkg", "network", "ebpf")
    network_c_dir = os.path.join(network_bpf_dir, "c")
    network_files = list(base_files)
    network_files.extend(glob.glob(network_c_dir + "/**/*.c"))
    network_flags = list(build_flags)
    network_flags.append("-I{}".format(network_c_dir))
    network_flags.append("-I{}".format(os.path.join(network_c_dir, "prebuilt")))
    network_flags.append("-I{}".format(os.path.join(network_c_dir, "runtime")))
    run_tidy(ctx, files=network_files, build_flags=network_flags, fix=fix, fail_on_issue=fail_on_issue)

    security_agent_c_dir = os.path.join(".", "pkg", "security", "ebpf", "c")
    security_files = list(base_files)
    security_files.extend(glob.glob(security_agent_c_dir + "/**/*.c"))
    security_flags = list(build_flags)
    security_flags.append("-I{}".format(security_agent_c_dir))
    security_flags.append("-DUSE_SYSCALL_WRAPPER=0")
    run_tidy(ctx, files=security_files, build_flags=security_flags, fix=fix, fail_on_issue=fail_on_issue)


def run_tidy(ctx, files, build_flags, fix=False, fail_on_issue=False):
    flags = ["--quiet"]
    if fix:
        flags.append("--fix")
    if fail_on_issue:
        flags.append("--warnings-as-errors='*'")

    ctx.run(
        "clang-tidy {flags} {files} -- {build_flags}".format(
            flags=" ".join(flags), build_flags=" ".join(build_flags), files=" ".join(files)
        )
    )


@task
def build_dev_docker_image(ctx, image_name, push=False):
    """
    Build a system-probe-agent Docker image (development only)
    if push is set to true the image will be pushed to the given registry
    """

    dev_file = os.path.join(".", "tools", "ebpf", "Dockerfiles", "Dockerfile-tracer-dev")
    cmd = "docker build {directory} -t {image_name} -f {file}"
    push_cmd = "docker push {image_name}"

    # Build in a temporary directory to make the docker build context small
    with tempdir() as d:
        shutil.copy(BIN_PATH, d)
        ctx.run(cmd.format(directory=d, image_name=image_name, file=dev_file))
        if push:
            ctx.run(push_cmd.format(image_name=image_name))


@task
def object_files(ctx, bundle_ebpf=True):
    """object_files builds the eBPF object files"""
    build_object_files(ctx, bundle_ebpf=bundle_ebpf)


def get_ebpf_c_files():
    files = glob.glob("pkg/ebpf/c/**/*.c")
    files.extend(glob.glob("pkg/network/ebpf/c/**/*.c"))
    files.extend(glob.glob("pkg/security/ebpf/c/**/*.c"))
    files.extend(glob.glob("pkg/collector/corechecks/ebpf/c/**/*.c"))
    return files


def get_ebpf_targets():
    files = glob.glob("pkg/ebpf/c/*.[c,h]")
    files.extend(glob.glob("pkg/network/ebpf/c/*.[c,h]"))
    files.extend(glob.glob("pkg/security/ebpf/c/*.[c,h]"))
    return files


def get_linux_header_dirs():
    os_info = os.uname()
    centos_headers_dir = "/usr/src/kernels"
    debian_headers_dir = "/usr/src"
    linux_headers = []
    if os.path.isdir(centos_headers_dir):
        for d in os.listdir(centos_headers_dir):
            if os_info.release in d:
                linux_headers.append(os.path.join(centos_headers_dir, d))
    else:
        for d in os.listdir(debian_headers_dir):
            if d.startswith("linux-") and os_info.release in d:
                linux_headers.append(os.path.join(debian_headers_dir, d))

    # fallback to non-filtered version for Docker where `uname -r` is not correct
    if len(linux_headers) == 0:
        if os.path.isdir(centos_headers_dir):
            linux_headers = [os.path.join(centos_headers_dir, d) for d in os.listdir(centos_headers_dir)]
        else:
            linux_headers = [
                os.path.join(debian_headers_dir, d) for d in os.listdir(debian_headers_dir) if d.startswith("linux-")
            ]

    # Mapping used by the kernel, from https://elixir.bootlin.com/linux/latest/source/scripts/subarch.include
    arch = (
        check_output(
            '''uname -m | sed -e s/i.86/x86/ -e s/x86_64/x86/ \
                    -e s/sun4u/sparc64/ \
                    -e s/arm.*/arm/ -e s/sa110/arm/ \
                    -e s/s390x/s390/ -e s/parisc64/parisc/ \
                    -e s/ppc.*/powerpc/ -e s/mips.*/mips/ \
                    -e s/sh[234].*/sh/ -e s/aarch64.*/arm64/ \
                    -e s/riscv.*/riscv/''',
            shell=True,
        )
        .decode('utf-8')
        .strip()
    )

    subdirs = [
        "include",
        "include/uapi",
        "include/generated/uapi",
        "arch/{}/include".format(arch),
        "arch/{}/include/uapi".format(arch),
        "arch/{}/include/generated".format(arch),
    ]

    dirs = []
    for d in linux_headers:
        for s in subdirs:
            dirs.extend([os.path.join(d, s)])

    return dirs


def get_ebpf_build_flags():
    bpf_dir = os.path.join(".", "pkg", "ebpf")
    c_dir = os.path.join(bpf_dir, "c")

    flags = [
        '-D__KERNEL__',
        '-DCONFIG_64BIT',
        '-D__BPF_TRACING__',
        '-DKBUILD_MODNAME=\'"ddsysprobe"\'',
        '-Wno-unused-value',
        '-Wno-pointer-sign',
        '-Wno-compare-distinct-pointer-types',
        '-Wunused',
        '-Wall',
        '-Werror',
        "-include {}".format(os.path.join(c_dir, "asm_goto_workaround.h")),
        '-O2',
        '-emit-llvm',
        # Some linux distributions enable stack protector by default which is not available on eBPF
        '-fno-stack-protector',
        '-fno-color-diagnostics',
        '-fno-unwind-tables',
        '-fno-asynchronous-unwind-tables',
        '-fno-jump-tables',
        "-I{}".format(c_dir),
    ]

    header_dirs = get_linux_header_dirs()
    for d in header_dirs:
        flags.extend(["-isystem", d])

    return flags


def build_object_files(ctx, bundle_ebpf=False):
    """build_object_files builds only the eBPF object
    set bundle_ebpf to False to disable replacing the assets
    """

    # if clang is missing, subsequent calls to ctx.run("clang ...") will fail silently, and result in us not building a
    # new .o file
    print("checking for clang executable...")
    ctx.run("which clang")
    print("found clang")

    bpf_dir = os.path.join(".", "pkg", "ebpf")
    build_dir = os.path.join(bpf_dir, "bytecode", "build")
    build_runtime_dir = os.path.join(build_dir, "runtime")

    network_bpf_dir = os.path.join(".", "pkg", "network", "ebpf")
    network_c_dir = os.path.join(network_bpf_dir, "c")
    network_prebuilt_dir = os.path.join(network_c_dir, "prebuilt")

    flags = get_ebpf_build_flags()
    cmd = "clang {flags} -c '{c_file}' -o '{bc_file}'"
    llc_cmd = "llc -march=bpf -filetype=obj -o '{obj_file}' '{bc_file}'"

    commands = [
        "mkdir -p {build_dir}".format(build_dir=build_dir),
        "mkdir -p {build_runtime_dir}".format(build_runtime_dir=build_runtime_dir),
    ]
    bindata_files = []

    corechecks_c_dir = os.path.join(".", "pkg", "collector", "corechecks", "ebpf", "c")
    corechecks_bcc_dir = os.path.join(corechecks_c_dir, "bcc")
    bcc_files = [
        os.path.join(corechecks_bcc_dir, "tcp-queue-length-kern.c"),
        os.path.join(corechecks_c_dir, "tcp-queue-length-kern-user.h"),
        os.path.join(corechecks_bcc_dir, "oom-kill-kern.c"),
        os.path.join(corechecks_c_dir, "oom-kill-kern-user.h"),
        os.path.join(corechecks_bcc_dir, "bpf-common.h"),
    ]
    for f in bcc_files:
        commands.append("cp {file} {dest}".format(file=f, dest=build_dir))
        bindata_files.append(os.path.join(build_dir, os.path.basename(f)))

    compiled_programs = [
        "tracer",
        "offset-guess",
    ]
    network_flags = list(flags)
    network_flags.append("-I{}".format(network_c_dir))
    for p in compiled_programs:
        # Build both the standard and debug version
        src_file = os.path.join(network_prebuilt_dir, "{}.c".format(p))
        bc_file = os.path.join(build_dir, "{}.bc".format(p))
        obj_file = os.path.join(build_dir, "{}.o".format(p))
        commands.append(cmd.format(flags=" ".join(network_flags), bc_file=bc_file, c_file=src_file))
        commands.append(llc_cmd.format(flags=" ".join(network_flags), bc_file=bc_file, obj_file=obj_file))

        debug_bc_file = os.path.join(build_dir, "{}-debug.bc".format(p))
        debug_obj_file = os.path.join(build_dir, "{}-debug.o".format(p))
        commands.append(
            cmd.format(flags=" ".join(network_flags + ["-DDEBUG=1"]), bc_file=debug_bc_file, c_file=src_file)
        )
        commands.append(llc_cmd.format(flags=" ".join(network_flags), bc_file=debug_bc_file, obj_file=debug_obj_file))

        bindata_files.extend([obj_file, debug_obj_file])

    security_agent_c_dir = os.path.join(".", "pkg", "security", "ebpf", "c")
    security_agent_prebuilt_dir = os.path.join(security_agent_c_dir, "prebuilt")
    security_c_file = os.path.join(security_agent_prebuilt_dir, "probe.c")
    security_bc_file = os.path.join(build_dir, "runtime-security.bc")
    security_agent_obj_file = os.path.join(build_dir, "runtime-security.o")
    security_flags = list(flags)
    security_flags.append("-I{}".format(security_agent_c_dir))

    commands.append(
        cmd.format(
            flags=" ".join(security_flags + ["-DUSE_SYSCALL_WRAPPER=0"]),
            c_file=security_c_file,
            bc_file=security_bc_file,
        )
    )
    commands.append(
        llc_cmd.format(flags=" ".join(security_flags), bc_file=security_bc_file, obj_file=security_agent_obj_file)
    )

    security_agent_syscall_wrapper_bc_file = os.path.join(build_dir, "runtime-security-syscall-wrapper.bc")
    security_agent_syscall_wrapper_obj_file = os.path.join(build_dir, "runtime-security-syscall-wrapper.o")
    commands.append(
        cmd.format(
            flags=" ".join(security_flags + ["-DUSE_SYSCALL_WRAPPER=1"]),
            c_file=security_c_file,
            bc_file=security_agent_syscall_wrapper_bc_file,
        )
    )
    commands.append(
        llc_cmd.format(
            flags=" ".join(security_flags),
            bc_file=security_agent_syscall_wrapper_bc_file,
            obj_file=security_agent_syscall_wrapper_obj_file,
        )
    )
    bindata_files.extend([security_agent_obj_file, security_agent_syscall_wrapper_obj_file])

    for cmd in commands:
        ctx.run(cmd)

    generate_runtime_files(ctx)

    if bundle_ebpf:
        go_dir = os.path.join(bpf_dir, "bytecode", "bindata")
        bundle_files(ctx, bindata_files, "pkg/.*/", go_dir)


@task
def generate_runtime_files(ctx):
    runtime_compiler_files = [
        "./pkg/network/tracer/compile.go",
        "./pkg/security/probe/compile.go",
    ]
    for f in runtime_compiler_files:
        ctx.run("go generate -mod=mod -tags linux_bpf {}".format(f))


def bundle_files(ctx, bindata_files, dir_prefix, go_dir):
    assets_cmd = (
        "go run github.com/shuLhan/go-bindata/cmd/go-bindata -tags ebpf_bindata -split"
        + " -pkg bindata -prefix '{dir_prefix}' -modtime 1 -o '{go_dir}' '{bindata_files}'"
    )
    ctx.run(assets_cmd.format(dir_prefix=dir_prefix, go_dir=go_dir, bindata_files="' '".join(bindata_files)))
    ctx.run("gofmt -w -s {go_dir}".format(go_dir=go_dir))


def build_ebpf_builder(ctx):
    """
    build_ebpf_builder builds the docker image for the ebpf builder
    """

    cmd = "docker build -t {image} -f {file} ."

    if should_use_sudo(ctx):
        cmd = "sudo " + cmd

    ctx.run(cmd.format(image=EBPF_BUILDER_IMAGE, file=EBPF_BUILDER_FILE))


def is_root():
    return os.getuid() == 0


def should_use_sudo(_):
    # We are already root
    if is_root():
        return False

    with open(os.devnull, 'w') as FNULL:
        try:
            check_output(['docker', 'info'], stderr=FNULL)
        except CalledProcessError:
            return True

    return False


@contextlib.contextmanager
def tempdir():
    """
    Helper to create a temp directory and clean it
    """
    dirpath = tempfile.mkdtemp()
    try:
        yield dirpath
    finally:
        shutil.rmtree(dirpath)

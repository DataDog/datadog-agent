import contextlib
import glob
import json
import os
import platform
import re
import shutil
import sys
import tempfile
from pathlib import Path
from subprocess import check_output

from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from .build_tags import get_default_build_tags
from .utils import REPO_PATH, bin_name, get_build_flags, get_version_numeric_only

BIN_DIR = os.path.join(".", "bin", "system-probe")
BIN_PATH = os.path.join(BIN_DIR, bin_name("system-probe", android=False))

BPF_TAG = "linux_bpf"
BUNDLE_TAG = "ebpf_bindata"
NPM_TAG = "npm"
GIMME_ENV_VARS = ['GOROOT', 'PATH']
DNF_TAG = "dnf"

CLANG_CMD = "clang {flags} -c '{c_file}' -o '{bc_file}'"
LLC_CMD = "llc -march=bpf -filetype=obj -o '{obj_file}' '{bc_file}'"

KITCHEN_DIR = os.getenv('DD_AGENT_TESTING_DIR') or os.path.normpath(os.path.join(os.getcwd(), "test", "kitchen"))
KITCHEN_ARTIFACT_DIR = os.path.join(KITCHEN_DIR, "site-cookbooks", "dd-system-probe-check", "files", "default", "tests")
TEST_PACKAGES_LIST = ["./pkg/ebpf/...", "./pkg/network/...", "./pkg/collector/corechecks/ebpf/..."]
TEST_PACKAGES = " ".join(TEST_PACKAGES_LIST)
CWS_PREBUILT_MINIMUM_KERNEL_VERSION = [5, 8, 0]

is_windows = sys.platform == "win32"

arch_mapping = {
    "amd64": "x64",
    "x86_64": "x64",
    "x64": "x64",
    "i386": "x86",
    "i686": "x86",
    "aarch64": "arm64",  # linux
    "arm64": "arm64",  # darwin
}
CURRENT_ARCH = arch_mapping.get(platform.machine(), "x64")


@task
def build(
    ctx,
    race=False,
    incremental_build=True,
    major_version='7',
    python_runtimes='3',
    go_mod="mod",
    windows=is_windows,
    arch=CURRENT_ARCH,
    compile_ebpf=True,
    nikos_embedded_path=None,
    bundle_ebpf=False,
    parallel_build=True,
    kernel_release=None,
    debug=False,
):
    """
    Build the system_probe
    """

    # generate windows resources
    if windows:
        windres_target = "pe-x86-64"
        if arch == "x86":
            raise Exit(message="system probe not supported on x86")

        ver = get_version_numeric_only(ctx, major_version=major_version)
        maj_ver, min_ver, patch_ver = ver.split(".")
        resdir = os.path.join(".", "cmd", "system-probe", "windows_resources")

        ctx.run(f"windmc --target {windres_target} -r {resdir} {resdir}/system-probe-msg.mc")

        ctx.run(
            f"windres --define MAJ_VER={maj_ver} --define MIN_VER={min_ver} --define PATCH_VER={patch_ver} -i cmd/system-probe/windows_resources/system-probe.rc --target {windres_target} -O coff -o cmd/system-probe/rsrc.syso"
        )
    elif compile_ebpf:
        # Only build ebpf files on unix
        build_object_files(ctx, parallel_build=parallel_build, kernel_release=kernel_release, debug=debug)

    generate_cgo_types(ctx, windows=windows)
    ldflags, gcflags, env = get_build_flags(
        ctx,
        major_version=major_version,
        python_runtimes=python_runtimes,
        nikos_embedded_path=nikos_embedded_path,
    )

    build_tags = get_default_build_tags(build="system-probe", arch=arch)
    if bundle_ebpf:
        build_tags.append(BUNDLE_TAG)
    if nikos_embedded_path:
        build_tags.append(DNF_TAG)

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
def test(
    ctx,
    packages=TEST_PACKAGES,
    skip_object_files=False,
    bundle_ebpf=False,
    output_path=None,
    runtime_compiled=False,
    skip_linters=False,
    run=None,
    windows=is_windows,
    parallel_build=True,
    failfast=False,
    kernel_release=None,
):
    """
    Run tests on eBPF parts
    If skip_object_files is set to True, this won't rebuild object files
    If output_path is set, we run `go test` with the flags `-c -o output_path`, which *compiles* the test suite
    into a single binary. This artifact is meant to be used in conjunction with kitchen tests.
    """
    if os.getenv("GOPATH") is None:
        raise Exit(
            code=1,
            message="GOPATH is not set, if you are running tests with sudo, you may need to use the -E option to "
            "preserve your environment",
        )

    if not skip_linters and not windows:
        clang_format(ctx)
        clang_tidy(ctx)

    if not skip_object_files and not windows:
        build_object_files(ctx, parallel_build=parallel_build, kernel_release=kernel_release)

    build_tags = [NPM_TAG]
    if not windows:
        build_tags.append(BPF_TAG)
        if bundle_ebpf:
            build_tags.append(BUNDLE_TAG)

    args = {
        "build_tags": ",".join(build_tags),
        "output_params": "-c -o " + output_path if output_path else "",
        "pkgs": packages,
        "run": "-run " + run if run else "",
        "failfast": "-failfast" if failfast else "",
    }

    _, _, env = get_build_flags(ctx)
    env['DD_SYSTEM_PROBE_BPF_DIR'] = os.path.join("/opt", "datadog-agent", "embedded", "share", "system-probe", "ebpf")
    if runtime_compiled:
        env['DD_TESTS_RUNTIME_COMPILED'] = "1"

    cmd = 'go test -mod=mod -v {failfast} -tags "{build_tags}" {output_params} {pkgs} {run}'
    if not windows and not output_path and not is_root():
        cmd = 'sudo -E ' + cmd

    ctx.run(cmd.format(**args), env=env)


@task
def kitchen_prepare(ctx, windows=is_windows, kernel_release=None):
    """
    Compile test suite for kitchen
    """

    # Clean up previous build
    if os.path.exists(KITCHEN_ARTIFACT_DIR):
        shutil.rmtree(KITCHEN_ARTIFACT_DIR)

    build_tags = [NPM_TAG]
    if not windows:
        build_tags.append(BPF_TAG)

    # Retrieve a list of all packages we want to test
    # This handles the elipsis notation (eg. ./pkg/ebpf/...)
    target_packages = []
    for pkg in TEST_PACKAGES_LIST:
        target_packages += (
            check_output(
                f"go list -f \"{{{{ .Dir }}}}\" -mod=mod -tags \"{','.join(build_tags)}\" {pkg}",
                shell=True,
            )
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
        target_path = os.path.join(KITCHEN_ARTIFACT_DIR, re.sub("^.*datadog-agent.", "", pkg))
        target_bin = "testsuite"
        if windows:
            target_bin = "testsuite.exe"

        test(
            ctx,
            packages=pkg,
            skip_object_files=(i != 0),
            skip_linters=True,
            bundle_ebpf=False,
            output_path=os.path.join(target_path, target_bin),
            kernel_release=kernel_release,
        )

        # copy ancillary data, if applicable
        for extra in ["testdata", "build"]:
            extra_path = os.path.join(pkg, extra)
            if os.path.isdir(extra_path):
                shutil.copytree(extra_path, os.path.join(target_path, extra))

    if os.path.exists("/opt/datadog-agent/embedded/bin/clang-bpf"):
        shutil.copy("/opt/datadog-agent/embedded/bin/clang-bpf", os.path.join(KITCHEN_ARTIFACT_DIR, ".."))
    if os.path.exists("/opt/datadog-agent/embedded/bin/llc-bpf"):
        shutil.copy("/opt/datadog-agent/embedded/bin/llc-bpf", os.path.join(KITCHEN_ARTIFACT_DIR, ".."))


@task
def kitchen_test(ctx, target=None, provider="virtualbox"):
    """
    Run tests (locally) using chef kitchen against an array of different platforms.
    * Make sure to run `inv -e system-probe.kitchen-prepare` using the agent-development VM;
    * Then we recommend to run `inv -e system-probe.kitchen-test` directly from your (macOS) machine;
    """

    vagrant_arch = ""
    if CURRENT_ARCH == "x64":
        vagrant_arch = "x86_64"
    elif CURRENT_ARCH == "arm64":
        vagrant_arch = "arm64"
    else:
        raise Exit(f"Unsupported vagrant arch for {CURRENT_ARCH}", code=1)

    # Retrieve a list of all available vagrant images
    images = {}
    platform_file = os.path.join(KITCHEN_DIR, "platforms.json")
    with open(platform_file, 'r') as f:
        for kplatform, by_provider in json.load(f).items():
            if "vagrant" in by_provider and vagrant_arch in by_provider["vagrant"]:
                for image in by_provider["vagrant"][vagrant_arch]:
                    images[image] = kplatform

    if not (target in images):
        print(
            f"please run inv -e system-probe.kitchen-test --target <IMAGE>, where <IMAGE> is one of the following:\n{list(images.keys())}"
        )
        raise Exit(code=1)

    with ctx.cd(KITCHEN_DIR):
        ctx.run(
            f"inv kitchen.genconfig --platform {images[target]} --osversions {target} --provider vagrant --testfiles system-probe-test --platformfile {platform_file} --arch {vagrant_arch}",
            env={"KITCHEN_VAGRANT_PROVIDER": provider},
        )
        ctx.run("kitchen test")


@task
def kitchen_genconfig(
    ctx, ssh_key, platform, osversions, image_size=None, provider="azure", arch=None, azure_sub_id=None
):
    if not arch:
        arch = CURRENT_ARCH

    if arch == "x64":
        arch = "x86_64"
    elif arch == "arm64":
        arch = "arm64"
    else:
        raise UnexpectedExit("unsupported arch specified")

    if not image_size and provider == "azure":
        image_size = "Standard_D2_v2"

    if not image_size:
        raise UnexpectedExit("Image size must be specified")

    if azure_sub_id is None and provider == "azure":
        raise Exit("azure subscription id must be specified with --azure-sub-id")

    env = {
        "KITCHEN_RSA_SSH_KEY_PATH": ssh_key,
    }
    if azure_sub_id:
        env["AZURE_SUBSCRIPTION_ID"] = azure_sub_id

    with ctx.cd(KITCHEN_DIR):
        ctx.run(
            f"inv -e kitchen.genconfig --platform={platform} --osversions={osversions} --provider={provider} --arch={arch} --imagesize={image_size} --testfiles=system-probe-test --platformfile=platforms.json",
            env=env,
        )


@task
def nettop(ctx, incremental_build=False, go_mod="mod", parallel_build=True, kernel_release=None):
    """
    Build and run the `nettop` utility for testing
    """
    build_object_files(ctx, parallel_build=parallel_build, kernel_release=kernel_release)

    cmd = 'go build -mod={go_mod} {build_type} -tags {tags} -o {bin_path} {path}'
    bin_path = os.path.join(BIN_DIR, "nettop")
    # Build
    ctx.run(
        cmd.format(
            path=os.path.join(REPO_PATH, "pkg", "network", "nettop"),
            bin_path=bin_path,
            go_mod=go_mod,
            build_type="" if incremental_build else "-a",
            tags=BPF_TAG,
        )
    )

    # Run
    if not is_root():
        ctx.sudo(bin_path)
    else:
        ctx.run(bin_path)


@task
def clang_format(ctx, targets=None, fix=False, fail_on_issue=False):
    """
    Format C code using clang-format
    """
    ctx.run("which clang-format")
    if isinstance(targets, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        targets = targets.split(',')

    if not targets:
        targets = get_ebpf_targets()

    # remove externally maintained files
    ignored_files = ["pkg/ebpf/c/bpf_helpers.h", "pkg/ebpf/c/bpf_endian.h", "pkg/ebpf/compiler/clang-stdarg.h"]
    for f in ignored_files:
        if f in targets:
            targets.remove(f)

    fmt_cmd = "clang-format -i --style=file --fallback-style=none"
    if not fix:
        fmt_cmd = fmt_cmd + " --dry-run"
    if fail_on_issue:
        fmt_cmd = fmt_cmd + " --Werror"

    ctx.run(f"{fmt_cmd} {' '.join(targets)}")


@task
def clang_tidy(ctx, fix=False, fail_on_issue=False):
    """
    Lint C code using clang-tidy
    """

    print("checking for clang-tidy executable...")
    ctx.run("which clang-tidy")

    build_flags = get_ebpf_build_flags()
    build_flags.append("-DDEBUG=1")

    bpf_dir = os.path.join(".", "pkg", "ebpf")
    base_files = glob.glob(bpf_dir + "/c/**/*.c")

    network_bpf_dir = os.path.join(".", "pkg", "network", "ebpf")
    network_c_dir = os.path.join(network_bpf_dir, "c")
    network_files = list(base_files)
    network_files.extend(glob.glob(network_c_dir + "/**/*[!http].c"))
    network_files.append(os.path.join(network_c_dir, "runtime", "http.c"))
    network_flags = list(build_flags)
    network_flags.append(f"-I{network_c_dir}")
    network_flags.append(f"-I{os.path.join(network_c_dir, 'prebuilt')}")
    network_flags.append(f"-I{os.path.join(network_c_dir, 'runtime')}")
    run_tidy(ctx, files=network_files, build_flags=network_flags, fix=fix, fail_on_issue=fail_on_issue)

    # special treatment for prebuilt/http.c
    http_files = [os.path.join(network_c_dir, "prebuilt", "http.c")]
    http_flags = get_http_prebuilt_build_flags(network_c_dir)
    http_flags.append(f"-I{network_c_dir}")
    http_flags.append(f"-I{os.path.join(network_c_dir, 'prebuilt')}")
    run_tidy(ctx, files=http_files, build_flags=http_flags, fix=fix, fail_on_issue=fail_on_issue)

    security_agent_c_dir = os.path.join(".", "pkg", "security", "ebpf", "c")
    security_files = list(base_files)
    security_files.extend(glob.glob(security_agent_c_dir + "/**/*.c"))
    security_flags = list(build_flags)
    security_flags.append(f"-I{security_agent_c_dir}")
    security_flags.append("-DUSE_SYSCALL_WRAPPER=0")
    security_checks = ["-readability-function-cognitive-complexity"]
    run_tidy(
        ctx,
        files=security_files,
        build_flags=security_flags,
        fix=fix,
        fail_on_issue=fail_on_issue,
        checks=security_checks,
    )


def run_tidy(ctx, files, build_flags, fix=False, fail_on_issue=False, checks=None):
    flags = ["--quiet"]
    if fix:
        flags.append("--fix")
    if fail_on_issue:
        flags.append("--warnings-as-errors='*'")

    if checks is not None:
        flags.append(f"--checks={','.join(checks)}")

    ctx.run(f"clang-tidy {' '.join(flags)} {' '.join(files)} -- {' '.join(build_flags)}", warn=True)


@task
def object_files(ctx, parallel_build=True, kernel_release=None):
    """object_files builds the eBPF object files"""
    build_object_files(ctx, parallel_build=parallel_build, kernel_release=kernel_release)


def get_ebpf_targets():
    files = glob.glob("pkg/ebpf/c/*.[c,h]")
    files.extend(glob.glob("pkg/network/ebpf/c/*.[c,h]"))
    files.extend(glob.glob("pkg/security/ebpf/c/*.[c,h]"))
    return files


def get_linux_header_dirs(kernel_release=None, minimal_kernel_release=None):
    if not kernel_release:
        os_info = os.uname()
        kernel_release = os_info.release

    if kernel_release and minimal_kernel_release:
        match = re.compile(r'(\d+)\.(\d+)(\.(\d+))?').match(kernel_release)
        version_tuple = list(map(int, map(lambda x: x or '0', match.group(1, 2, 4))))
        if version_tuple < minimal_kernel_release:
            print(
                f"You need to have kernel headers for at least {'.'.join(map(lambda x: str(x), minimal_kernel_release))} to enable all system-probe features"
            )

    src_kernels_dir = "/usr/src/kernels"
    src_dir = "/usr/src"
    possible_dirs = [
        f"/lib/modules/{kernel_release}/build",
        f"/lib/modules/{kernel_release}/source",
        f"{src_dir}/linux-headers-{kernel_release}",
        f"{src_kernels_dir}/{kernel_release}",
    ]
    linux_headers = []
    for d in possible_dirs:
        if os.path.isdir(d):
            # resolve symlinks
            linux_headers.append(Path(d).resolve())

    # fallback to non-release-specific directories
    if len(linux_headers) == 0:
        if os.path.isdir(src_kernels_dir):
            linux_headers = [os.path.join(src_kernels_dir, d) for d in os.listdir(src_kernels_dir)]
        else:
            linux_headers = [os.path.join(src_dir, d) for d in os.listdir(src_dir) if d.startswith("linux-")]

    # fallback to /usr as a last report
    if len(linux_headers) == 0:
        linux_headers = ["/usr"]

    # deduplicate
    linux_headers = list(dict.fromkeys(linux_headers))

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
        f"arch/{arch}/include",
        f"arch/{arch}/include/uapi",
        f"arch/{arch}/include/generated",
    ]

    dirs = []
    for d in linux_headers:
        for s in subdirs:
            dirs.extend([os.path.join(d, s)])

    return dirs


def get_ebpf_build_flags(target=None, kernel_release=None, minimal_kernel_release=None):
    bpf_dir = os.path.join(".", "pkg", "ebpf")
    c_dir = os.path.join(bpf_dir, "c")
    if not target:
        target = ['-emit-llvm']

    flags = [
        '-D__KERNEL__',
        '-DCONFIG_64BIT',
        '-D__BPF_TRACING__',
        '-DKBUILD_MODNAME=\\"ddsysprobe\\"',
        '-Wno-unused-value',
        '-Wno-pointer-sign',
        '-Wno-compare-distinct-pointer-types',
        '-Wunused',
        '-Wall',
        '-Werror',
    ]
    flags.extend(target)
    flags.extend(
        [
            f"-include {os.path.join(c_dir, 'asm_goto_workaround.h')}",
            '-O2',
            # Some linux distributions enable stack protector by default which is not available on eBPF
            '-fno-stack-protector',
            '-fno-color-diagnostics',
            '-fno-unwind-tables',
            '-fno-asynchronous-unwind-tables',
            '-fno-jump-tables',
            f"-I{c_dir}",
        ]
    )

    header_dirs = get_linux_header_dirs(kernel_release=kernel_release, minimal_kernel_release=minimal_kernel_release)
    for d in header_dirs:
        flags.extend(["-isystem", d])

    return flags


def build_network_ebpf_compile_file(
    ctx, parallel_build, build_dir, p, debug, network_prebuilt_dir, network_flags, extension=".bc"
):
    src_file = os.path.join(network_prebuilt_dir, f"{p}.c")
    if not debug:
        bc_file = os.path.join(build_dir, f"{p}{extension}")
        return ctx.run(
            CLANG_CMD.format(flags=" ".join(network_flags), bc_file=bc_file, c_file=src_file),
            asynchronous=parallel_build,
        )
    else:
        debug_bc_file = os.path.join(build_dir, f"{p}-debug{extension}")
        return ctx.run(
            CLANG_CMD.format(flags=" ".join(network_flags + ["-DDEBUG=1"]), bc_file=debug_bc_file, c_file=src_file),
            asynchronous=parallel_build,
        )


def build_network_ebpf_link_file(ctx, parallel_build, build_dir, p, debug, network_flags):
    if not debug:
        bc_file = os.path.join(build_dir, f"{p}.bc")
        obj_file = os.path.join(build_dir, f"{p}.o")
        return ctx.run(
            LLC_CMD.format(flags=" ".join(network_flags), bc_file=bc_file, obj_file=obj_file),
            asynchronous=parallel_build,
        )
    else:
        debug_bc_file = os.path.join(build_dir, f"{p}-debug.bc")
        debug_obj_file = os.path.join(build_dir, f"{p}-debug.o")
        return ctx.run(
            LLC_CMD.format(flags=" ".join(network_flags), bc_file=debug_bc_file, obj_file=debug_obj_file),
            asynchronous=parallel_build,
        )


def get_http_prebuilt_build_flags(network_c_dir, kernel_release=None):
    uname_m = check_output("uname -m", shell=True).decode('utf-8').strip()
    flags = get_ebpf_build_flags(kernel_release=kernel_release)
    flags.append(f"-I{network_c_dir}")
    flags.append(f"-D__{uname_m}__")
    flags.append(f"-isystem /usr/include/{uname_m}-linux-gnu")
    return flags


def build_http_ebpf_files(ctx, build_dir, kernel_release=None):
    network_bpf_dir = os.path.join(".", "pkg", "network", "ebpf")
    network_c_dir = os.path.join(network_bpf_dir, "c")
    network_prebuilt_dir = os.path.join(network_c_dir, "prebuilt")

    network_flags = get_http_prebuilt_build_flags(network_c_dir, kernel_release=kernel_release)

    build_network_ebpf_compile_file(ctx, False, build_dir, "http", True, network_prebuilt_dir, network_flags)
    build_network_ebpf_link_file(ctx, False, build_dir, "http", True, network_flags)

    build_network_ebpf_compile_file(ctx, False, build_dir, "http", False, network_prebuilt_dir, network_flags)
    build_network_ebpf_link_file(ctx, False, build_dir, "http", False, network_flags)


def get_network_build_flags(network_c_dir, kernel_release=None):
    flags = get_ebpf_build_flags(kernel_release=kernel_release)
    flags.append(f"-I{network_c_dir}")
    return flags


def build_network_ebpf_files(ctx, build_dir, parallel_build=True, kernel_release=None):
    network_bpf_dir = os.path.join(".", "pkg", "network", "ebpf")
    network_c_dir = os.path.join(network_bpf_dir, "c")
    network_prebuilt_dir = os.path.join(network_c_dir, "prebuilt")

    compiled_programs = ["dns", "offset-guess", "tracer"]

    network_flags = get_network_build_flags(network_c_dir, kernel_release=kernel_release)

    flavor = []
    for prog in compiled_programs:
        for debug in [False, True]:
            flavor.append((prog, debug))

    promises = []
    for p, debug in flavor:
        promises.append(
            build_network_ebpf_compile_file(
                ctx, parallel_build, build_dir, p, debug, network_prebuilt_dir, network_flags
            )
        )
        if not parallel_build:
            build_network_ebpf_link_file(ctx, parallel_build, build_dir, p, debug, network_flags)

    if not parallel_build:
        return

    promises_link = []
    for i, promise in enumerate(promises):
        promise.join()
        (p, debug) = flavor[i]
        promises_link.append(build_network_ebpf_link_file(ctx, parallel_build, build_dir, p, debug, network_flags))

    for promise in promises_link:
        promise.join()


def get_security_agent_build_flags(security_agent_c_dir, kernel_release=None, minimal_kernel_release=None, debug=False):
    security_flags = get_ebpf_build_flags(kernel_release=kernel_release, minimal_kernel_release=minimal_kernel_release)
    security_flags.append(f"-I{security_agent_c_dir}")
    if debug:
        security_flags.append("-DDEBUG=1")
    return security_flags


def build_security_offset_guesser_ebpf_files(ctx, build_dir, kernel_release=None, debug=False):
    security_agent_c_dir = os.path.join(".", "pkg", "security", "ebpf", "c")
    security_agent_prebuilt_dir = os.path.join(security_agent_c_dir, "prebuilt")
    security_c_file = os.path.join(security_agent_prebuilt_dir, "offset-guesser.c")
    security_bc_file = os.path.join(build_dir, "runtime-security-offset-guesser.bc")
    security_agent_obj_file = os.path.join(build_dir, "runtime-security-offset-guesser.o")

    security_flags = get_security_agent_build_flags(
        security_agent_c_dir,
        kernel_release=kernel_release,
        minimal_kernel_release=CWS_PREBUILT_MINIMUM_KERNEL_VERSION,
        debug=debug,
    )

    ctx.run(
        CLANG_CMD.format(
            flags=" ".join(security_flags),
            c_file=security_c_file,
            bc_file=security_bc_file,
        ),
    )
    ctx.run(
        LLC_CMD.format(flags=" ".join(security_flags), bc_file=security_bc_file, obj_file=security_agent_obj_file),
    )


def build_security_probe_ebpf_files(ctx, build_dir, parallel_build=True, kernel_release=None, debug=False):
    security_agent_c_dir = os.path.join(".", "pkg", "security", "ebpf", "c")
    security_agent_prebuilt_dir = os.path.join(security_agent_c_dir, "prebuilt")
    security_c_file = os.path.join(security_agent_prebuilt_dir, "probe.c")
    security_bc_file = os.path.join(build_dir, "runtime-security.bc")
    security_agent_obj_file = os.path.join(build_dir, "runtime-security.o")

    security_flags = get_security_agent_build_flags(
        security_agent_c_dir,
        kernel_release=kernel_release,
        minimal_kernel_release=CWS_PREBUILT_MINIMUM_KERNEL_VERSION,
        debug=debug,
    )

    # compile
    promises = []
    promises.append(
        ctx.run(
            CLANG_CMD.format(
                flags=" ".join(security_flags + ["-DUSE_SYSCALL_WRAPPER=0"]),
                c_file=security_c_file,
                bc_file=security_bc_file,
            ),
            asynchronous=parallel_build,
        )
    )
    security_agent_syscall_wrapper_bc_file = os.path.join(build_dir, "runtime-security-syscall-wrapper.bc")
    promises.append(
        ctx.run(
            CLANG_CMD.format(
                flags=" ".join(security_flags + ["-DUSE_SYSCALL_WRAPPER=1"]),
                c_file=security_c_file,
                bc_file=security_agent_syscall_wrapper_bc_file,
            ),
            asynchronous=parallel_build,
        )
    )

    if parallel_build:
        for p in promises:
            p.join()

    # link
    promises = []
    promises.append(
        ctx.run(
            LLC_CMD.format(flags=" ".join(security_flags), bc_file=security_bc_file, obj_file=security_agent_obj_file),
            asynchronous=parallel_build,
        )
    )

    security_agent_syscall_wrapper_obj_file = os.path.join(build_dir, "runtime-security-syscall-wrapper.o")
    promises.append(
        ctx.run(
            LLC_CMD.format(
                flags=" ".join(security_flags),
                bc_file=security_agent_syscall_wrapper_bc_file,
                obj_file=security_agent_syscall_wrapper_obj_file,
            ),
            asynchronous=parallel_build,
        )
    )

    if parallel_build:
        for p in promises:
            p.join()


def build_security_ebpf_files(ctx, build_dir, parallel_build=True, kernel_release=None, debug=False):
    build_security_probe_ebpf_files(ctx, build_dir, parallel_build, kernel_release=kernel_release, debug=debug)
    build_security_offset_guesser_ebpf_files(ctx, build_dir, kernel_release=kernel_release, debug=debug)


def build_object_files(ctx, parallel_build, kernel_release=None, debug=False):
    """build_object_files builds only the eBPF object"""

    # if clang is missing, subsequent calls to ctx.run("clang ...") will fail silently
    print("checking for clang executable...")
    ctx.run("which clang")

    build_dir = os.path.join(".", "pkg", "ebpf", "bytecode", "build")
    build_runtime_dir = os.path.join(build_dir, "runtime")

    ctx.run(f"mkdir -p {build_dir}")
    ctx.run(f"mkdir -p {build_runtime_dir}")

    build_network_ebpf_files(ctx, build_dir=build_dir, parallel_build=parallel_build, kernel_release=kernel_release)
    build_http_ebpf_files(ctx, build_dir=build_dir, kernel_release=kernel_release)
    build_security_ebpf_files(
        ctx, build_dir=build_dir, parallel_build=parallel_build, kernel_release=kernel_release, debug=debug
    )

    generate_runtime_files(ctx)

    # We need to copy the bpf files out of the mounted build directory in order to be able to
    # change their ownership to root
    src_files = os.path.join(build_dir, "*")
    bpf_dir = os.path.join("/opt", "datadog-agent", "embedded", "share", "system-probe", "ebpf")

    if not is_root():
        ctx.sudo(f"mkdir -p {bpf_dir}")
        ctx.sudo(f"cp -R {src_files} {bpf_dir}")
        ctx.sudo(f"chown root:root -R {bpf_dir}")
    else:
        ctx.run(f"mkdir -p {bpf_dir}")
        ctx.run(f"cp -R {src_files} {bpf_dir}")
        ctx.run(f"chown root:root -R {bpf_dir}")


@task
def generate_runtime_files(ctx):
    runtime_compiler_files = [
        "./pkg/collector/corechecks/ebpf/probe/oom_kill.go",
        "./pkg/collector/corechecks/ebpf/probe/tcp_queue_length.go",
        "./pkg/network/http/compile.go",
        "./pkg/network/tracer/compile.go",
        "./pkg/network/tracer/connection/kprobe/compile.go",
        "./pkg/security/ebpf/compile.go",
    ]
    for f in runtime_compiler_files:
        ctx.run(f"go generate -mod=mod -tags {BPF_TAG} {f}")


def replace_cgo_tag_absolute_path(file_path, windows=is_windows):
    # read
    f = open(file_path)
    lines = []
    for line in f:
        if (windows and line.startswith("// cgo.exe -godefs")) or (not windows and line.startswith("// cgo -godefs")):
            path = line.split()[-1]
            if os.path.isabs(path):
                _, filename = os.path.split(path)
                lines.append(line.replace(path, filename))
                continue
        lines.append(line)
    f.close()

    # write
    f = open(file_path, "w")
    res = "".join(lines)
    f.write(res)
    f.close()


@task
def generate_cgo_types(ctx, windows=is_windows, replace_absolutes=True):
    if windows:
        platform = "windows"
        def_files = ["./pkg/network/driver/types.go"]
    else:
        platform = "linux"
        def_files = [
            "./pkg/network/ebpf/offsetguess_types.go",
            "./pkg/network/ebpf/conntrack_types.go",
            "./pkg/network/ebpf/tuple_types.go",
            "./pkg/network/ebpf/kprobe_types.go",
        ]

    env = {}
    if not is_windows:
        env["CC"] = "clang"

    for f in def_files:
        fdir, file = os.path.split(f)
        base, _ = os.path.splitext(file)
        with ctx.cd(fdir):
            output_file = f"{base}_{platform}.go"
            ctx.run(f"go tool cgo -godefs -- -fsigned-char {file} > {output_file}", env=env)
            ctx.run(f"gofmt -w -s {output_file}")
            if replace_absolutes:
                # replace absolute path with relative ones in generated file
                replace_cgo_tag_absolute_path(file_path=os.path.join(fdir, output_file), windows=windows)


@task
def generate_lookup_tables(ctx, windows=is_windows):
    if windows:
        return

    lookup_table_generate_files = [
        "./pkg/network/go/goid/main.go",
        "./pkg/network/http/gotls/lookup/main.go",
    ]
    for file in lookup_table_generate_files:
        ctx.run(f"go generate {file}")


def is_root():
    return os.getuid() == 0


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

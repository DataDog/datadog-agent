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
from .libs.ninja_syntax import NinjaWriter
from .utils import REPO_PATH, bin_name, get_build_flags, get_version_numeric_only

BIN_DIR = os.path.join(".", "bin", "system-probe")
BIN_PATH = os.path.join(BIN_DIR, bin_name("system-probe"))

BPF_TAG = "linux_bpf"
BUNDLE_TAG = "ebpf_bindata"
NPM_TAG = "npm"
DNF_TAG = "dnf"

CHECK_SOURCE_CMD = "grep -v '^//' {src_file} | if grep -q ' inline ' ; then echo -e '\u001b[7mPlease use __always_inline instead of inline in {src_file}\u001b[0m';exit 1;fi"

KITCHEN_DIR = os.getenv('DD_AGENT_TESTING_DIR') or os.path.normpath(os.path.join(os.getcwd(), "test", "kitchen"))
KITCHEN_ARTIFACT_DIR = os.path.join(KITCHEN_DIR, "site-cookbooks", "dd-system-probe-check", "files", "default", "tests")
TEST_PACKAGES_LIST = ["./pkg/ebpf/...", "./pkg/network/...", "./pkg/collector/corechecks/ebpf/..."]
TEST_PACKAGES = " ".join(TEST_PACKAGES_LIST)
CWS_PREBUILT_MINIMUM_KERNEL_VERSION = [5, 8, 0]
EMBEDDED_SHARE_DIR = os.path.join("/opt", "datadog-agent", "embedded", "share", "system-probe", "ebpf")

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
CLANG_VERSION = "12.0.1"


def ninja_define_windows_resources(ctx, nw, major_version):
    maj_ver, min_ver, patch_ver = get_version_numeric_only(ctx, major_version=major_version).split(".")
    nw.variable("maj_ver", maj_ver)
    nw.variable("min_ver", min_ver)
    nw.variable("patch_ver", patch_ver)
    nw.variable("windrestarget", "pe-x86-64")
    nw.rule(name="windmc", command="windmc --target $windrestarget -r $rcdir $in")
    nw.rule(
        name="windres",
        command="windres --define MAJ_VER=$maj_ver --define MIN_VER=$min_ver --define PATCH_VER=$patch_ver "
        + "-i $in --target $windrestarget -O coff -o $out",
    )


def ninja_define_ebpf_compiler(nw, strip_object_files=False, kernel_release=None):
    nw.variable("target", "-emit-llvm")
    nw.variable("ebpfflags", get_ebpf_build_flags())
    nw.variable("kheaders", get_kernel_headers_flags(kernel_release))

    nw.rule(
        name="ebpfclang",
        command="clang -MD -MF $out.d $target $ebpfflags $kheaders $flags -c $in -o $out",
        depfile="$out.d",
    )
    strip = "&& llvm-strip -g $out" if strip_object_files else ""
    nw.rule(
        name="llc",
        command=f"llc -march=bpf -filetype=obj -o $out $in {strip}",
    )


def ninja_define_exe_compiler(nw):
    nw.rule(
        name="execlang",
        command="clang -MD -MF $out.d $exeflags $in -o $out $exelibs",
        depfile="$out.d",
    )


def ninja_ebpf_program(nw, infile, outfile, variables=None):
    outdir, basefile = os.path.split(outfile)
    basename = os.path.basename(os.path.splitext(basefile)[0])
    out_base = f"{outdir}/{basename}"
    nw.build(
        inputs=[infile],
        outputs=[f"{out_base}.bc"],
        rule="ebpfclang",
        variables=variables,
    )
    nw.build(
        inputs=[f"{out_base}.bc"],
        outputs=[f"{out_base}.o"],
        rule="llc",
    )


def ninja_security_ebpf_programs(nw, build_dir, debug, kernel_release):
    security_agent_c_dir = os.path.join("pkg", "security", "ebpf", "c")
    security_agent_prebuilt_dir = os.path.join(security_agent_c_dir, "prebuilt")

    kernel_headers = get_linux_header_dirs(
        kernel_release=kernel_release, minimal_kernel_release=CWS_PREBUILT_MINIMUM_KERNEL_VERSION
    )
    kheaders = " ".join(f"-isystem{d}" for d in kernel_headers)
    debugdef = "-DDEBUG=1" if debug else ""
    security_flags = f"-I{security_agent_c_dir} {debugdef}"

    outfiles = []

    # basic
    infile = os.path.join(security_agent_prebuilt_dir, "probe.c")
    outfile = os.path.join(build_dir, "runtime-security.o")
    ninja_ebpf_program(
        nw,
        infile=infile,
        outfile=outfile,
        variables={
            "flags": security_flags + " -DUSE_SYSCALL_WRAPPER=0",
            "kheaders": kheaders,
        },
    )
    outfiles.append(outfile)

    # syscall wrapper
    root, ext = os.path.splitext(outfile)
    syscall_wrapper_outfile = f"{root}-syscall-wrapper{ext}"
    ninja_ebpf_program(
        nw,
        infile=infile,
        outfile=syscall_wrapper_outfile,
        variables={
            "flags": security_flags + " -DUSE_SYSCALL_WRAPPER=1",
            "kheaders": kheaders,
        },
    )
    outfiles.append(syscall_wrapper_outfile)

    # offset guesser
    offset_guesser_outfile = os.path.join(build_dir, "runtime-security-offset-guesser.o")
    ninja_ebpf_program(
        nw,
        infile=os.path.join(security_agent_prebuilt_dir, "offset-guesser.c"),
        outfile=offset_guesser_outfile,
        variables={
            "flags": security_flags,
            "kheaders": kheaders,
        },
    )
    outfiles.append(offset_guesser_outfile)

    nw.build(rule="phony", inputs=outfiles, outputs=["cws"])


def ninja_network_ebpf_program(nw, infile, outfile, flags):
    ninja_ebpf_program(nw, infile, outfile, {"flags": flags})
    root, ext = os.path.splitext(outfile)
    ninja_ebpf_program(nw, infile, f"{root}-debug{ext}", {"flags": flags + " -DDEBUG=1"})


def ninja_network_ebpf_programs(nw, build_dir):
    network_bpf_dir = os.path.join("pkg", "network", "ebpf")
    network_c_dir = os.path.join(network_bpf_dir, "c")
    network_prebuilt_dir = os.path.join(network_c_dir, "prebuilt")

    network_flags = "-Ipkg/network/ebpf/c -g"
    network_programs = ["dns", "offset-guess", "tracer", "http"]
    for prog in network_programs:
        infile = os.path.join(network_prebuilt_dir, f"{prog}.c")
        outfile = os.path.join(build_dir, f"{prog}.o")
        ninja_network_ebpf_program(nw, infile, outfile, network_flags)


def ninja_runtime_compilation_files(nw):
    bc_dir = os.path.join("pkg", "ebpf", "bytecode")
    build_dir = os.path.join(bc_dir, "build")

    runtime_compiler_files = {
        "pkg/collector/corechecks/ebpf/probe/oom_kill.go": "oom-kill",
        "pkg/collector/corechecks/ebpf/probe/tcp_queue_length.go": "tcp-queue-length",
        "pkg/network/http/compile.go": "http",
        "pkg/network/tracer/compile.go": "conntrack",
        "pkg/network/tracer/connection/kprobe/compile.go": "tracer",
        "pkg/security/ebpf/compile.go": "runtime-security",
    }

    nw.rule(name="headerincl", command="go generate -mod=mod -tags linux_bpf $in", depfile="$out.d")
    hash_dir = os.path.join(bc_dir, "runtime")
    rc_dir = os.path.join(build_dir, "runtime")
    rc_outputs = []
    for in_path, out_filename in runtime_compiler_files.items():
        c_file = os.path.join(rc_dir, f"{out_filename}.c")
        hash_file = os.path.join(hash_dir, f"{out_filename}.go")
        nw.build(
            inputs=[in_path],
            outputs=[c_file],
            implicit_outputs=[hash_file],
            rule="headerincl",
        )
        rc_outputs.extend([c_file, hash_file])
    nw.build(rule="phony", inputs=rc_outputs, outputs=["runtime-compilation"])


def ninja_cgo_type_files(nw, windows):
    # TODO we could probably preprocess the input files to find out the dependencies
    nw.pool(name="cgo_pool", depth=1)
    if windows:
        go_platform = "windows"
        def_files = {"pkg/network/driver/types.go": ["pkg/network/driver/ddnpmapi.h"]}
        nw.rule(
            name="godefs",
            pool="cgo_pool",
            command="powershell -Command \"$$PSDefaultParameterValues['Out-File:Encoding'] = 'ascii';"
            + "(cd $in_dir);"
            + "(go tool cgo -godefs -- -fsigned-char $in_file | "
            + "go run $script_path | Out-File -encoding ascii $out_file);"
            + "exit $$LastExitCode\"",
        )
    else:
        go_platform = "linux"
        def_files = {
            "pkg/network/ebpf/offsetguess_types.go": ["pkg/network/ebpf/c/prebuilt/offset-guess.h"],
            "pkg/network/ebpf/conntrack_types.go": ["pkg/network/ebpf/c/runtime/conntrack-types.h"],
            "pkg/network/ebpf/tuple_types.go": ["pkg/network/ebpf/c/tracer.h"],
            "pkg/network/ebpf/kprobe_types.go": [
                "pkg/network/ebpf/c/tracer.h",
                "pkg/network/ebpf/c/tcp_states.h",
                "pkg/network/ebpf/c/tags-types.h",
                "pkg/network/ebpf/c/prebuilt/offset-guess.h",
            ],
            "pkg/network/http/http_types.go": [
                "pkg/network/ebpf/c/tracer.h",
                "pkg/network/ebpf/c/http-types.h",
            ],
            "pkg/network/http/transaction/transaction_types.go":[
                "pkg/network/ebpf/c/tracer.h",
                "pkg/network/ebpf/c/http-types.h",
            ]
        }
        nw.rule(
            name="godefs",
            pool="cgo_pool",
            command="cd $in_dir && "
            + "CC=clang go tool cgo -godefs -- -fsigned-char $in_file | "
            + "go run $script_path > $out_file",
        )

    script_path = os.path.join(os.getcwd(), "pkg", "ebpf", "cgo", "genpost.go")
    for f, headers in def_files.items():
        in_dir, in_file = os.path.split(f)
        in_base, _ = os.path.splitext(in_file)
        out_file = f"{in_base}_{go_platform}.go"
        nw.build(
            inputs=[f],
            outputs=[os.path.join(in_dir, out_file)],
            rule="godefs",
            implicit=headers,
            variables={
                "in_dir": in_dir,
                "in_file": in_file,
                "out_file": out_file,
                "script_path": script_path,
            },
        )


def ninja_generate(
    ctx,
    ninja_path,
    windows,
    major_version='7',
    arch=CURRENT_ARCH,
    debug=False,
    strip_object_files=False,
    kernel_release=None,
):
    build_dir = os.path.join("pkg", "ebpf", "bytecode", "build")

    with open(ninja_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file, width=120)

        if windows:
            if arch == "x86":
                raise Exit(message="system probe not supported on x86")

            ninja_define_windows_resources(ctx, nw, major_version)
            rcout = "cmd/system-probe/windows_resources/system-probe.rc"
            in_path = "cmd/system-probe/windows_resources/system-probe-msg.mc"
            in_dir, _ = os.path.split(in_path)
            nw.build(inputs=[in_path], outputs=[rcout], rule="windmc", variables={"rcdir": in_dir})
            nw.build(inputs=[rcout], outputs=["cmd/system-probe/rsrc.syso"], rule="windres")
        else:
            ninja_define_ebpf_compiler(nw, strip_object_files, kernel_release)
            ninja_network_ebpf_programs(nw, build_dir)
            ninja_security_ebpf_programs(nw, build_dir, debug, kernel_release)
            ninja_runtime_compilation_files(nw)

        ninja_cgo_type_files(nw, windows)


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
    nikos_embedded_path=None,
    bundle_ebpf=False,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
    strip_binary=False,
):
    """
    Build the system-probe
    """
    build_object_files(
        ctx,
        windows=windows,
        major_version=major_version,
        arch=arch,
        kernel_release=kernel_release,
        debug=debug,
        strip_object_files=strip_object_files,
    )

    build_sysprobe_binary(
        ctx,
        major_version=major_version,
        python_runtimes=python_runtimes,
        nikos_embedded_path=nikos_embedded_path,
        bundle_ebpf=bundle_ebpf,
        arch=arch,
        go_mod=go_mod,
        race=race,
        incremental_build=incremental_build,
        strip_binary=strip_binary,
    )


@task
def clean(
    ctx,
    windows=is_windows,
    arch=CURRENT_ARCH,
):
    clean_object_files(
        ctx,
        windows=windows,
        arch=arch,
    )
    ctx.run("go clean -cache")


def build_sysprobe_binary(
    ctx,
    race=False,
    incremental_build=True,
    major_version='7',
    python_runtimes='3',
    go_mod="mod",
    arch=CURRENT_ARCH,
    nikos_embedded_path=None,
    bundle_ebpf=False,
    strip_binary=False,
):
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

    if strip_binary:
        ldflags += ' -s -w'

    cmd = 'go build -mod={go_mod}{race_opt}{build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/system-probe'

    args = {
        "go_mod": go_mod,
        "race_opt": " -race" if race else "",
        "build_type": "" if incremental_build else " -a",
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
    bundle_ebpf=False,
    output_path=None,
    runtime_compiled=False,
    skip_linters=False,
    skip_object_files=False,
    run=None,
    windows=is_windows,
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

    if not skip_object_files:
        build_object_files(
            ctx,
            windows=windows,
            kernel_release=kernel_release,
        )

    build_tags = [NPM_TAG]
    if not windows:
        build_tags.append(BPF_TAG)
        if bundle_ebpf:
            build_tags.append(BUNDLE_TAG)

    args = {
        "build_tags": ",".join(build_tags),
        "output_params": f"-c -o {output_path}" if output_path else "",
        "pkgs": packages,
        "run": f"-run {run}" if run else "",
        "failfast": "-failfast" if failfast else "",
        "go": "go",
    }

    _, _, env = get_build_flags(ctx)
    env['DD_SYSTEM_PROBE_BPF_DIR'] = EMBEDDED_SHARE_DIR
    if runtime_compiled:
        env['DD_TESTS_RUNTIME_COMPILED'] = "1"

    go_root = os.getenv("GOROOT")
    if go_root:
        args["go"] = os.path.join(go_root, "bin", "go")

    cmd = '{go} test -mod=mod -v {failfast} -tags "{build_tags}" {output_params} {pkgs} {run}'
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
def clang_tidy(ctx, fix=False, fail_on_issue=False, kernel_release=None):
    """
    Lint C code using clang-tidy
    """

    print("checking for clang-tidy executable...")
    ctx.run("which clang-tidy")

    build_flags = get_ebpf_build_flags()
    build_flags.append("-DDEBUG=1")
    build_flags.append("-emit-llvm")
    build_flags.extend(get_kernel_headers_flags(kernel_release=kernel_release))

    bpf_dir = os.path.join(".", "pkg", "ebpf")
    base_files = glob.glob(f"{bpf_dir}/c/**/*.c")

    network_c_dir = os.path.join(".", "pkg", "network", "ebpf", "c")
    network_files = list(base_files)
    network_files.extend(glob.glob(f"{network_c_dir}/**/*.c"))
    network_flags = list(build_flags)
    network_flags.append(f"-I{network_c_dir}")
    network_flags.append(f"-I{os.path.join(network_c_dir, 'prebuilt')}")
    network_flags.append(f"-I{os.path.join(network_c_dir, 'runtime')}")
    run_tidy(ctx, files=network_files, build_flags=network_flags, fix=fix, fail_on_issue=fail_on_issue)

    security_agent_c_dir = os.path.join(".", "pkg", "security", "ebpf", "c")
    security_files = list(base_files)
    security_files.extend(glob.glob(f"{security_agent_c_dir}/**/*.c"))
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


def get_ebpf_targets():
    files = glob.glob("pkg/ebpf/c/*.[c,h]")
    files.extend(glob.glob("pkg/network/ebpf/c/**/*.[c,h]", recursive=True))
    files.extend(glob.glob("pkg/security/ebpf/c/**/*.[c,h]", recursive=True))
    return files


def get_kernel_arch():
    # Mapping used by the kernel, from https://elixir.bootlin.com/linux/latest/source/scripts/subarch.include
    return (
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


def get_linux_header_dirs(kernel_release=None, minimal_kernel_release=None):
    if not kernel_release:
        os_info = os.uname()
        kernel_release = os_info.release

    if kernel_release and minimal_kernel_release:
        match = re.compile(r'(\d+)\.(\d+)(\.(\d+))?').match(kernel_release)
        version_tuple = [int(x) or 0 for x in match.group(1, 2, 4)]
        if version_tuple < minimal_kernel_release:
            print(
                f"You need to have kernel headers for at least {'.'.join([str(x) for x in minimal_kernel_release])} to enable all system-probe features"
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
    arch = get_kernel_arch()

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


def get_ebpf_build_flags():
    flags = []
    flags.extend(
        [
            '-D__KERNEL__',
            '-DCONFIG_64BIT',
            '-D__BPF_TRACING__',
            '-DKBUILD_MODNAME=\\"ddsysprobe\\"',
        ]
    )
    flags.extend(
        [
            '-Wno-unused-value',
            '-Wno-pointer-sign',
            '-Wno-compare-distinct-pointer-types',
            '-Wunused',
            '-Wall',
            '-Werror',
        ]
    )
    flags.extend(["-include pkg/ebpf/c/asm_goto_workaround.h"])
    flags.extend(["-O2"])
    flags.extend(
        [
            # Some linux distributions enable stack protector by default which is not available on eBPF
            '-fno-stack-protector',
            '-fno-color-diagnostics',
            '-fno-unwind-tables',
            '-fno-asynchronous-unwind-tables',
            '-fno-jump-tables',
            '-fmerge-all-constants',
        ]
    )
    flags.extend(["-Ipkg/ebpf/c"])
    return flags


def get_kernel_headers_flags(kernel_release=None, minimal_kernel_release=None):
    return [
        f"-isystem{d}"
        for d in get_linux_header_dirs(kernel_release=kernel_release, minimal_kernel_release=minimal_kernel_release)
    ]


def check_for_inline(ctx):
    for p in [ebpf_check_source_file(ctx, parallel_build=True, src_file=f) for f in get_ebpf_targets()]:
        p.join()


def ebpf_check_source_file(ctx, parallel_build, src_file):
    return ctx.run(CHECK_SOURCE_CMD.format(src_file=src_file), echo=False, asynchronous=parallel_build)


def run_ninja(
    ctx,
    task="",
    target="",
    explain=False,
    windows=is_windows,
    major_version='7',
    arch=CURRENT_ARCH,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
):
    check_for_ninja(ctx)
    nf_path = os.path.join(ctx.cwd, 'system-probe.ninja')
    ninja_generate(ctx, nf_path, windows, major_version, arch, debug, strip_object_files, kernel_release)
    explain_opt = "-d explain" if explain else ""
    if task:
        ctx.run(f"ninja {explain_opt} -f {nf_path} -t {task}")
    else:
        ctx.run(f"ninja {explain_opt} -f {nf_path} {target}")


def setup_runtime_clang(ctx):
    # check if correct version is already present
    sudo = "sudo" if not is_root() else ""
    clang_res = ctx.run(f"{sudo} /opt/datadog-agent/embedded/bin/clang-bpf --version", warn=True)
    llc_res = ctx.run(f"{sudo} /opt/datadog-agent/embedded/bin/llc-bpf --version", warn=True)
    clang_version_str = clang_res.stdout.split("\n")[0].split(" ")[2].strip() if clang_res.ok else ""
    llc_version_str = llc_res.stdout.split("\n")[1].strip().split(" ")[2].strip() if llc_res.ok else ""

    if not os.path.exists("/opt/datadog-agent/embedded/bin"):
        ctx.run(f"{sudo} mkdir -p /opt/datadog-agent/embedded/bin")

    arch = arch_mapping.get(platform.machine())
    if arch == "x64":
        arch = "amd64"

    if clang_version_str != CLANG_VERSION:
        # download correct version from dd-agent-omnibus S3 bucket
        clang_url = f"https://dd-agent-omnibus.s3.amazonaws.com/llvm/clang-{CLANG_VERSION}.{arch}"
        ctx.run(f"{sudo} wget -q {clang_url} -O /opt/datadog-agent/embedded/bin/clang-bpf")
        ctx.run(f"{sudo} chmod 0755 /opt/datadog-agent/embedded/bin/clang-bpf")

    if llc_version_str != CLANG_VERSION:
        llc_url = f"https://dd-agent-omnibus.s3.amazonaws.com/llvm/llc-{CLANG_VERSION}.{arch}"
        ctx.run(f"{sudo} wget -q {llc_url} -O /opt/datadog-agent/embedded/bin/llc-bpf")
        ctx.run(f"{sudo} chmod 0755 /opt/datadog-agent/embedded/bin/llc-bpf")


def build_object_files(
    ctx,
    windows=is_windows,
    major_version='7',
    arch=CURRENT_ARCH,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
):
    build_dir = os.path.join("pkg", "ebpf", "bytecode", "build")

    if not windows:
        # if clang is missing, subsequent calls to ctx.run("clang ...") will fail silently
        setup_runtime_clang(ctx)
        print("checking for clang executable...")
        ctx.run("which clang")

        if strip_object_files:
            print("checking for llvm-strip...")
            ctx.run("which llvm-strip")

        check_for_inline(ctx)
        ctx.run(f"mkdir -p {build_dir}/runtime")

    run_ninja(
        ctx,
        explain=True,
        windows=windows,
        major_version=major_version,
        arch=arch,
        kernel_release=kernel_release,
        debug=debug,
        strip_object_files=strip_object_files,
    )

    if not windows:
        sudo = "" if is_root() else "sudo"
        ctx.run(f"{sudo} mkdir -p {EMBEDDED_SHARE_DIR}")
        ctx.run(f"{sudo} cp -R {build_dir}/* {EMBEDDED_SHARE_DIR}")
        ctx.run(f"{sudo} chown root:root -R {EMBEDDED_SHARE_DIR}")


def build_cws_object_files(
    ctx, major_version='7', arch=CURRENT_ARCH, kernel_release=None, debug=False, strip_object_files=False
):
    run_ninja(
        ctx,
        target="cws",
        major_version=major_version,
        arch=arch,
        debug=debug,
        strip_object_files=strip_object_files,
        kernel_release=kernel_release,
    )


@task
def object_files(ctx, kernel_release=None):
    build_object_files(ctx, kernel_release=kernel_release)


def clean_object_files(
    ctx, windows, major_version='7', arch=CURRENT_ARCH, kernel_release=None, debug=False, strip_object_files=False
):
    run_ninja(
        ctx,
        task="clean",
        windows=windows,
        major_version=major_version,
        arch=arch,
        debug=debug,
        strip_object_files=strip_object_files,
        kernel_release=kernel_release,
    )


# deprecated: this function is only kept to prevent breaking security-agent.go-generate-check
def generate_runtime_files(ctx):
    run_ninja(ctx, explain=True, target="runtime-compilation")


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


def check_for_ninja(ctx):
    if is_windows:
        ctx.run("where ninja")
    else:
        ctx.run("which ninja")


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

import contextlib
import glob
import json
import os
import platform
import re
import shutil
import string
import sys
import tarfile
import tempfile
from pathlib import Path
from subprocess import check_output

from invoke import task
from invoke.exceptions import Exit

from .build_tags import UNIT_TEST_TAGS, get_default_build_tags
from .libs.common.color import color_message
from .libs.ninja_syntax import NinjaWriter
from .test import environ
from .utils import REPO_PATH, bin_name, get_build_flags, get_gobin, get_version_numeric_only
from .windows_resources import MESSAGESTRINGS_MC_PATH, arch_to_windres_target

BIN_DIR = os.path.join(".", "bin", "system-probe")
BIN_PATH = os.path.join(BIN_DIR, bin_name("system-probe"))

BPF_TAG = "linux_bpf"
BUNDLE_TAG = "ebpf_bindata"
NPM_TAG = "npm"

KITCHEN_DIR = os.getenv('DD_AGENT_TESTING_DIR') or os.path.normpath(os.path.join(os.getcwd(), "test", "kitchen"))
KITCHEN_ARTIFACT_DIR = os.path.join(KITCHEN_DIR, "site-cookbooks", "dd-system-probe-check", "files", "default", "tests")
TEST_PACKAGES_LIST = ["./pkg/ebpf/...", "./pkg/network/...", "./pkg/collector/corechecks/ebpf/..."]
TEST_PACKAGES = " ".join(TEST_PACKAGES_LIST)
TEST_TIMEOUTS = {
    "pkg/network/tracer$": "0",
    "pkg/network/protocols/http$": "0",
    "pkg/network/protocols": "5m",
}
CWS_PREBUILT_MINIMUM_KERNEL_VERSION = [5, 8, 0]
EMBEDDED_SHARE_DIR = os.path.join("/opt", "datadog-agent", "embedded", "share", "system-probe", "ebpf")
EMBEDDED_SHARE_JAVA_DIR = os.path.join("/opt", "datadog-agent", "embedded", "share", "system-probe", "java")

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
CLANG_VERSION_RUNTIME = "12.0.1"
CLANG_VERSION_SYSTEM_PREFIX = "12.0"


def ninja_define_windows_resources(ctx, nw, major_version, arch=CURRENT_ARCH):
    maj_ver, min_ver, patch_ver = get_version_numeric_only(ctx, major_version=major_version).split(".")
    nw.variable("maj_ver", maj_ver)
    nw.variable("min_ver", min_ver)
    nw.variable("patch_ver", patch_ver)
    nw.variable("windrestarget", arch_to_windres_target(arch))
    nw.rule(name="windmc", command="windmc --target $windrestarget -r $rcdir -h $rcdir $in")
    nw.rule(
        name="windres",
        command="windres --define MAJ_VER=$maj_ver --define MIN_VER=$min_ver --define PATCH_VER=$patch_ver "
        + "-i $in --target $windrestarget -O coff -o $out",
    )


def ninja_define_ebpf_compiler(nw, strip_object_files=False, kernel_release=None, with_unit_test=False):
    nw.variable("target", "-emit-llvm")
    nw.variable("ebpfflags", get_ebpf_build_flags(with_unit_test))
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


def ninja_define_co_re_compiler(nw):
    nw.variable("ebpfcoreflags", get_co_re_build_flags())

    nw.rule(
        name="ebpfcoreclang",
        command="clang -MD -MF $out.d -target bpf $ebpfcoreflags $flags -c $in -o $out",
        depfile="$out.d",
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


def ninja_ebpf_co_re_program(nw, infile, outfile, variables=None):
    outdir, basefile = os.path.split(outfile)
    basename = os.path.basename(os.path.splitext(basefile)[0])
    out_base = f"{outdir}/{basename}"
    nw.build(
        inputs=[infile],
        outputs=[f"{out_base}.bc"],
        rule="ebpfcoreclang",
        variables=variables,
    )
    nw.build(
        inputs=[f"{out_base}.bc"],
        outputs=[f"{out_base}.o"],
        rule="llc",
    )


def ninja_security_ebpf_programs(nw, build_dir, debug, kernel_release):
    security_agent_c_dir = os.path.join("pkg", "security", "ebpf", "c")
    security_agent_prebuilt_dir_include = os.path.join(security_agent_c_dir, "include")
    security_agent_prebuilt_dir = os.path.join(security_agent_c_dir, "prebuilt")

    kernel_headers = get_linux_header_dirs(
        kernel_release=kernel_release, minimal_kernel_release=CWS_PREBUILT_MINIMUM_KERNEL_VERSION
    )
    kheaders = " ".join(f"-isystem{d}" for d in kernel_headers)
    debugdef = "-DDEBUG=1" if debug else ""
    security_flags = f"-g -I{security_agent_prebuilt_dir_include} {debugdef}"

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

    # fentry + syscall wrapper
    root, ext = os.path.splitext(outfile)
    syscall_wrapper_outfile = f"{root}-fentry{ext}"
    ninja_ebpf_program(
        nw,
        infile=infile,
        outfile=syscall_wrapper_outfile,
        variables={
            "flags": security_flags + " -DUSE_SYSCALL_WRAPPER=1 -DUSE_FENTRY=1",
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


def ninja_network_ebpf_co_re_program(nw, infile, outfile, flags):
    ninja_ebpf_co_re_program(nw, infile, outfile, {"flags": flags})
    root, ext = os.path.splitext(outfile)
    ninja_ebpf_co_re_program(nw, infile, f"{root}-debug{ext}", {"flags": flags + " -DDEBUG=1"})


def ninja_network_ebpf_programs(nw, build_dir, co_re_build_dir):
    network_bpf_dir = os.path.join("pkg", "network", "ebpf")
    network_c_dir = os.path.join(network_bpf_dir, "c")

    network_flags = "-Ipkg/network/ebpf/c -g"
    network_programs = [
        "prebuilt/dns",
        "prebuilt/offset-guess",
        "tracer",
        "prebuilt/usm",
        "prebuilt/usm_events_test",
        "prebuilt/shared-libraries",
        "prebuilt/conntrack",
    ]
    network_co_re_programs = ["tracer", "co-re/tracer-fentry", "runtime/usm", "runtime/shared-libraries"]

    for prog in network_programs:
        infile = os.path.join(network_c_dir, f"{prog}.c")
        outfile = os.path.join(build_dir, f"{os.path.basename(prog)}.o")
        ninja_network_ebpf_program(nw, infile, outfile, network_flags)

    for prog_path in network_co_re_programs:
        prog = os.path.basename(prog_path)
        src_dir = os.path.join(network_c_dir, os.path.dirname(prog_path))
        network_co_re_flags = f"-I{src_dir} -Ipkg/network/ebpf/c"

        infile = os.path.join(src_dir, f"{prog}.c")
        outfile = os.path.join(co_re_build_dir, f"{prog}.o")
        ninja_network_ebpf_co_re_program(nw, infile, outfile, network_co_re_flags)


def ninja_container_integrations_ebpf_programs(nw, co_re_build_dir):
    container_integrations_co_re_dir = os.path.join("pkg", "collector", "corechecks", "ebpf", "c", "runtime")
    container_integrations_co_re_flags = f"-I{container_integrations_co_re_dir}"
    container_integrations_co_re_programs = ["oom-kill", "tcp-queue-length", "ebpf"]

    for prog in container_integrations_co_re_programs:
        infile = os.path.join(container_integrations_co_re_dir, f"{prog}-kern.c")
        outfile = os.path.join(co_re_build_dir, f"{prog}.o")
        ninja_ebpf_co_re_program(nw, infile, outfile, {"flags": container_integrations_co_re_flags})
        root, ext = os.path.splitext(outfile)
        ninja_ebpf_co_re_program(
            nw, infile, f"{root}-debug{ext}", {"flags": container_integrations_co_re_flags + " -DDEBUG=1"}
        )


def ninja_runtime_compilation_files(nw, gobin):
    bc_dir = os.path.join("pkg", "ebpf", "bytecode")
    build_dir = os.path.join(bc_dir, "build")

    rc_tools = {
        "pkg/ebpf/include_headers.go": "include_headers",
        "pkg/ebpf/bytecode/runtime/integrity.go": "integrity",
    }

    toolpaths = []
    nw.rule(name="rctool", command="go install $in")
    for in_path, toolname in rc_tools.items():
        toolpath = os.path.join(gobin, toolname)
        toolpaths.append(toolpath)
        nw.build(
            inputs=[in_path],
            outputs=[toolpath],
            rule="rctool",
        )

    runtime_compiler_files = {
        "pkg/collector/corechecks/ebpf/probe/oomkill/oom_kill.go": "oom-kill",
        "pkg/collector/corechecks/ebpf/probe/tcpqueuelength/tcp_queue_length.go": "tcp-queue-length",
        "pkg/network/usm/compile.go": "usm",
        "pkg/network/usm/sharedlibraries/compile.go": "shared-libraries",
        "pkg/network/tracer/compile.go": "conntrack",
        "pkg/network/tracer/connection/kprobe/compile.go": "tracer",
        "pkg/network/tracer/offsetguess_test.go": "offsetguess-test",
        "pkg/security/ebpf/compile.go": "runtime-security",
    }

    nw.rule(name="headerincl", command="go generate -mod=mod -tags linux_bpf $in", depfile="$out.d")
    hash_dir = os.path.join(bc_dir, "runtime")
    rc_dir = os.path.join(build_dir, "runtime")
    for in_path, out_filename in runtime_compiler_files.items():
        c_file = os.path.join(rc_dir, f"{out_filename}.c")
        hash_file = os.path.join(hash_dir, f"{out_filename}.go")
        nw.build(
            inputs=[in_path],
            outputs=[c_file],
            implicit=toolpaths,
            implicit_outputs=[hash_file],
            rule="headerincl",
        )


def ninja_cgo_type_files(nw, windows):
    # TODO we could probably preprocess the input files to find out the dependencies
    nw.pool(name="cgo_pool", depth=1)
    if windows:
        go_platform = "windows"
        def_files = {
            "pkg/network/driver/types.go": [
                "pkg/network/driver/ddnpmapi.h",
            ],
            "pkg/util/winutil/etw/types.go": [
                "pkg/util/winutil/etw/etw-provider.h",
            ],
            "pkg/windowsdriver/procmon/types.go": [
                "pkg/windowsdriver/include/procmonapi.h",
            ],
        }
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
            "pkg/network/ebpf/conntrack_types.go": ["pkg/network/ebpf/c/conntrack/types.h"],
            "pkg/network/ebpf/tuple_types.go": ["pkg/network/ebpf/c/tracer/tracer.h"],
            "pkg/network/ebpf/kprobe_types.go": [
                "pkg/network/ebpf/c/tracer/tracer.h",
                "pkg/network/ebpf/c/tcp_states.h",
                "pkg/network/ebpf/c/prebuilt/offset-guess.h",
                "pkg/network/ebpf/c/protocols/classification/defs.h",
            ],
            "pkg/network/protocols/http/gotls/go_tls_types.go": [
                "pkg/network/ebpf/c/protocols/tls/go-tls-types.h",
            ],
            "pkg/network/protocols/http/types.go": [
                "pkg/network/ebpf/c/tracer/tracer.h",
                "pkg/network/ebpf/c/protocols/tls/tags-types.h",
                "pkg/network/ebpf/c/protocols/http/types.h",
                "pkg/network/ebpf/c/protocols/classification/defs.h",
            ],
            "pkg/network/protocols/http2/types.go": [
                "pkg/network/ebpf/c/tracer/tracer.h",
                "pkg/network/ebpf/c/protocols/http2/decoding-defs.h",
            ],
            "pkg/network/protocols/kafka/types.go": [
                "pkg/network/ebpf/c/tracer/tracer.h",
                "pkg/network/ebpf/c/protocols/kafka/types.h",
            ],
            "pkg/network/telemetry/telemetry_types.go": [
                "pkg/ebpf/c/telemetry_types.h",
            ],
            "pkg/network/tracer/offsetguess/offsetguess_types.go": [
                "pkg/network/ebpf/c/prebuilt/offset-guess.h",
            ],
            "pkg/network/protocols/events/types.go": [
                "pkg/network/ebpf/c/protocols/events-types.h",
            ],
            "pkg/collector/corechecks/ebpf/probe/tcpqueuelength/tcp_queue_length_kern_types.go": [
                "pkg/collector/corechecks/ebpf/c/runtime/tcp-queue-length-kern-user.h",
            ],
            "pkg/network/usm/sharedlibraries/types.go": [
                "pkg/network/ebpf/c/shared-libraries/types.h",
            ],
            "pkg/collector/corechecks/ebpf/probe/ebpfcheck/c_types.go": [
                "pkg/collector/corechecks/ebpf/c/runtime/ebpf-kern-user.h"
            ],
        }
        nw.rule(
            name="godefs",
            pool="cgo_pool",
            command="cd $in_dir && "
            + "CC=clang go tool cgo -godefs -- $rel_import -fsigned-char $in_file | "
            + "go run $script_path > $out_file",
        )

    script_path = os.path.join(os.getcwd(), "pkg", "ebpf", "cgo", "genpost.go")
    for f, headers in def_files.items():
        in_dir, in_file = os.path.split(f)
        in_base, _ = os.path.splitext(in_file)
        out_file = f"{in_base}_{go_platform}.go"
        rel_import = f"-I {os.path.relpath('pkg/network/ebpf/c', in_dir)} -I {os.path.relpath('pkg/ebpf/c', in_dir)}"
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
                "rel_import": rel_import,
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
    with_unit_test=False,
):
    build_dir = os.path.join("pkg", "ebpf", "bytecode", "build")
    co_re_build_dir = os.path.join(build_dir, "co-re")

    with open(ninja_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file, width=120)

        if windows:
            if arch == "x86":
                raise Exit(message="system probe not supported on x86")

            ninja_define_windows_resources(ctx, nw, major_version, arch=arch)
            # messagestrings
            in_path = MESSAGESTRINGS_MC_PATH
            in_name = os.path.splitext(os.path.basename(in_path))[0]
            in_dir = os.path.dirname(in_path)
            rcout = os.path.join(in_dir, f"{in_name}.rc")
            hout = os.path.join(in_dir, f'{in_name}.h')
            msgout = os.path.join(in_dir, 'MSG00409.bin')
            nw.build(
                inputs=[in_path],
                outputs=[rcout],
                implicit_outputs=[hout, msgout],
                rule="windmc",
                variables={"rcdir": in_dir},
            )
            nw.build(inputs=[rcout], outputs=[os.path.join(in_dir, "rsrc.syso")], rule="windres")
            # system-probe
            rcin = "cmd/system-probe/windows_resources/system-probe.rc"
            nw.build(inputs=[rcin], outputs=["cmd/system-probe/rsrc.syso"], rule="windres")
        else:
            gobin = get_gobin(ctx)
            ninja_define_ebpf_compiler(nw, strip_object_files, kernel_release, with_unit_test)
            ninja_define_co_re_compiler(nw)
            ninja_network_ebpf_programs(nw, build_dir, co_re_build_dir)
            ninja_security_ebpf_programs(nw, build_dir, debug, kernel_release)
            ninja_container_integrations_ebpf_programs(nw, co_re_build_dir)
            ninja_runtime_compilation_files(nw, gobin)

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
    bundle_ebpf=False,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
    strip_binary=False,
    with_unit_test=False,
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
        with_unit_test=with_unit_test,
    )

    build_sysprobe_binary(
        ctx,
        major_version=major_version,
        python_runtimes=python_runtimes,
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
    bundle_ebpf=False,
    strip_binary=False,
):
    ldflags, gcflags, env = get_build_flags(
        ctx,
        major_version=major_version,
        python_runtimes=python_runtimes,
    )

    build_tags = get_default_build_tags(build="system-probe", arch=arch)
    if bundle_ebpf:
        build_tags.append(BUNDLE_TAG)
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
    co_re=False,
    skip_linters=False,
    skip_object_files=False,
    run=None,
    windows=is_windows,
    failfast=False,
    kernel_release=None,
    timeout=None,
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
    build_tags.extend(UNIT_TEST_TAGS)
    if not windows:
        build_tags.append(BPF_TAG)
        if bundle_ebpf:
            build_tags.append(BUNDLE_TAG)

    args = {
        "build_tags": ",".join(build_tags),
        "output_params": f"-c -o {output_path}" if output_path else "",
        "run": f"-run {run}" if run else "",
        "failfast": "-failfast" if failfast else "",
        "go": "go",
        "sudo": "sudo -E " if not windows and not output_path and not is_root() else "",
    }

    _, _, env = get_build_flags(ctx)
    env['DD_SYSTEM_PROBE_BPF_DIR'] = EMBEDDED_SHARE_DIR
    if runtime_compiled:
        env['DD_ENABLE_RUNTIME_COMPILER'] = "true"
        env['DD_ALLOW_PRECOMPILED_FALLBACK'] = "false"
        env['DD_ENABLE_CO_RE'] = "false"
    elif co_re:
        env['DD_ENABLE_CO_RE'] = "true"
        env['DD_ALLOW_RUNTIME_COMPILED_FALLBACK'] = "false"
        env['DD_ALLOW_PRECOMPILED_FALLBACK'] = "false"

    go_root = os.getenv("GOROOT")
    if go_root:
        args["go"] = os.path.join(go_root, "bin", "go")

    failed_pkgs = list()
    package_dirs = go_package_dirs(packages.split(" "), build_tags)
    # we iterate over the packages here to get the nice streaming test output
    for pdir in package_dirs:
        args["dir"] = pdir
        testto = timeout if timeout else get_test_timeout(pdir)
        args["timeout"] = f"-timeout {testto}" if testto else ""
        cmd = '{sudo}{go} test -mod=mod -v {failfast} {timeout} -tags "{build_tags}" {output_params} {dir} {run}'
        res = ctx.run(cmd.format(**args), env=env, warn=True)
        if res.exited is None or res.exited > 0:
            failed_pkgs.append(os.path.relpath(pdir, ctx.cwd))
            if failfast:
                break

    if len(failed_pkgs) > 0:
        print(color_message("failed packages:\n" + "\n".join(failed_pkgs), "red"))
        raise Exit(code=1, message="system-probe tests failed")


def get_test_timeout(pkg):
    for tt, to in TEST_TIMEOUTS.items():
        if re.search(tt, pkg) is not None:
            return to
    return None


@contextlib.contextmanager
def chdir(dirname=None):
    curdir = os.getcwd()
    try:
        if dirname is not None:
            os.chdir(dirname)
        yield
    finally:
        os.chdir(curdir)


def go_package_dirs(packages, build_tags):
    """
    Retrieve a list of all packages we want to test
    This handles the ellipsis notation (eg. ./pkg/ebpf/...)
    """

    target_packages = []
    for pkg in packages:
        target_packages += (
            check_output(
                f"go list -find -f \"{{{{ .Dir }}}}\" -mod=mod -tags \"{','.join(build_tags)}\" {pkg}",
                shell=True,
            )
            .decode('utf-8')
            .strip()
            .split("\n")
        )

    return target_packages


BUILD_COMMIT = os.path.join(KITCHEN_ARTIFACT_DIR, "build.commit")


def clean_build(ctx):
    if not os.path.exists(KITCHEN_ARTIFACT_DIR):
        return True

    if not os.path.exists(BUILD_COMMIT):
        return True

    # if this build happens on a new commit do it cleanly
    with open(BUILD_COMMIT, 'r') as f:
        build_commit = f.read().rstrip()
        curr_commit = ctx.run("git rev-parse HEAD", hide=True).stdout.rstrip()
        if curr_commit != build_commit:
            return True

    return False


def full_pkg_path(name):
    return os.path.join(os.getcwd(), name[name.index("pkg") :])


@task
def kitchen_prepare(ctx, windows=is_windows, kernel_release=None, ci=False, packages=""):
    """
    Compile test suite for kitchen
    """
    build_tags = [NPM_TAG]
    if not windows:
        build_tags.append(BPF_TAG)

    target_packages = go_package_dirs(TEST_PACKAGES_LIST, build_tags)

    # Clean up previous build
    if os.path.exists(KITCHEN_ARTIFACT_DIR) and (packages == "" or clean_build(ctx)):
        shutil.rmtree(KITCHEN_ARTIFACT_DIR)
    elif packages != "":
        packages = [full_pkg_path(name) for name in packages.split(",")]
        # make sure valid packages were provided.
        for pkg in packages:
            if pkg not in target_packages:
                raise Exit(f"Unknown target packages {pkg} specified")

        target_packages = packages

    if os.path.exists(BUILD_COMMIT):
        os.remove(BUILD_COMMIT)

    os.makedirs(KITCHEN_ARTIFACT_DIR, exist_ok=True)

    # clean target_packages only
    for pkg_dir in target_packages:
        test_dir = pkg_dir.lstrip(os.getcwd())
        if os.path.exists(os.path.join(KITCHEN_ARTIFACT_DIR, test_dir)):
            shutil.rmtree(os.path.join(KITCHEN_ARTIFACT_DIR, test_dir))

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

        if pkg.endswith("java"):
            shutil.copy(os.path.join(pkg, "agent-usm.jar"), os.path.join(target_path, "agent-usm.jar"))

        for gobin in ["gotls_client", "fmapper", "prefetch_file"]:
            src_file_path = os.path.join(pkg, f"{gobin}.go")
            if not windows and os.path.isdir(pkg) and os.path.isfile(src_file_path):
                binary_path = os.path.join(target_path, gobin)
                with chdir(pkg):
                    ctx.run(f"go build -o {binary_path} -ldflags=\"-extldflags '-static'\" {gobin}.go")

    gopath = os.getenv("GOPATH")
    copy_files = [
        "/opt/datadog-agent/embedded/bin/clang-bpf",
        "/opt/datadog-agent/embedded/bin/llc-bpf",
        f"{gopath}/bin/gotestsum",
    ]

    files_dir = os.path.join(KITCHEN_ARTIFACT_DIR, "..")
    for cf in copy_files:
        if os.path.exists(cf):
            shutil.copy(cf, files_dir)

    if not ci:
        kitchen_prepare_btfs(ctx, files_dir)

    ctx.run(f"go build -o {files_dir}/test2json -ldflags=\"-s -w\" cmd/test2json", env={"CGO_ENABLED": "0"})
    ctx.run(f"echo $(git rev-parse HEAD) > {BUILD_COMMIT}")


@task
def kitchen_test(ctx, target=None, provider=None):
    """
    Run tests (locally with vagrant) using chef kitchen against an array of different platforms.
    * Make sure to run `inv -e system-probe.kitchen-prepare` using the agent-development VM;
    * Then we recommend to run `inv -e system-probe.kitchen-test` directly from your (macOS) machine;
    """

    if CURRENT_ARCH == "x64":
        vagrant_arch = "x86_64"
        provider = provider or "virtualbox"
    elif CURRENT_ARCH == "arm64":
        vagrant_arch = "arm64"
        provider = provider or "parallels"
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

    args = [
        f"--platform {images[target]}",
        f"--osversions {target}",
        "--provider vagrant",
        "--testfiles system-probe-test",
        f"--platformfile {platform_file}",
        f"--arch {vagrant_arch}",
    ]

    with ctx.cd(KITCHEN_DIR):
        ctx.run(
            f"inv kitchen.genconfig {' '.join(args)}",
            env={"KITCHEN_VAGRANT_PROVIDER": provider},
        )
        ctx.run("kitchen test")


@task
def kitchen_genconfig(
    ctx,
    ssh_key,
    platform,
    osversions,
    image_size=None,
    provider="azure",
    arch=None,
    azure_sub_id=None,
    ec2_device_name="/dev/sda1",
    mount_path="/mnt/ci",
):
    if not arch:
        arch = CURRENT_ARCH

    if arch_mapping[arch] == "x64":
        arch = "x86_64"
    elif arch_mapping[arch] == "arm64":
        arch = "arm64"
    else:
        raise Exit("unsupported arch specified")

    if not image_size and provider == "azure":
        image_size = "Standard_D2_v2"

    if azure_sub_id is None and provider == "azure":
        raise Exit("azure subscription id must be specified with --azure-sub-id")

    env = {
        "KITCHEN_CI_MOUNT_PATH": mount_path,
        "KITCHEN_CI_ROOT_PATH": "/tmp/ci",
    }
    if provider == "azure":
        env["KITCHEN_RSA_SSH_KEY_PATH"] = ssh_key
        if azure_sub_id:
            env["AZURE_SUBSCRIPTION_ID"] = azure_sub_id
    elif provider == "ec2":
        env["KITCHEN_EC2_SSH_KEY_PATH"] = ssh_key
        env["KITCHEN_EC2_DEVICE_NAME"] = ec2_device_name

    args = [
        f"--platform={platform}",
        f"--osversions={osversions}",
        f"--provider={provider}",
        f"--arch={arch}",
        f"--imagesize={image_size}",
        "--testfiles=system-probe-test",
        "--platformfile=platforms.json",
    ]

    env["KITCHEN_ARCH"] = arch
    env["KITCHEN_PLATFORM"] = platform
    with ctx.cd(KITCHEN_DIR):
        ctx.run(
            f"inv -e kitchen.genconfig {' '.join(args)}",
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


def get_ebpf_build_flags(unit_test=False):
    flags = []
    flags.extend(
        [
            '-D__KERNEL__',
            '-DCONFIG_64BIT',
            '-D__BPF_TRACING__',
            '-DKBUILD_MODNAME=\\"ddsysprobe\\"',
            '-DCOMPILE_PREBUILT',
        ]
    )
    if unit_test:
        flags.extend(['-D__BALOUM__'])
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


def get_co_re_build_flags():
    flags = get_ebpf_build_flags()

    flags.remove('-DCOMPILE_PREBUILT')
    flags.remove('-DCONFIG_64BIT')
    flags.remove('-include pkg/ebpf/c/asm_goto_workaround.h')

    arch = get_kernel_arch()
    flags.extend(
        [
            f"-D__TARGET_ARCH_{arch}",
            "-DCOMPILE_CORE",
            '-emit-llvm',
            '-g',
        ]
    )

    return flags


def get_kernel_headers_flags(kernel_release=None, minimal_kernel_release=None):
    return [
        f"-isystem{d}"
        for d in get_linux_header_dirs(kernel_release=kernel_release, minimal_kernel_release=minimal_kernel_release)
    ]


def check_for_inline(ctx):
    print("checking for invalid inline usage...")
    src_dirs = ["pkg/ebpf/c/", "pkg/network/ebpf/c/", "pkg/security/ebpf/c/"]
    grep_filter = "--include='*.c' --include '*.h'"
    grep_exclude = "--exclude='bpf_helpers.h'"
    pattern = "'^[^/]*\\binline\\b'"
    grep_res = ctx.run(f"grep -n {grep_filter} {grep_exclude} -r {pattern} {' '.join(src_dirs)}", warn=True, hide=True)
    if grep_res.ok:
        print(color_message("Use __always_inline instead of inline:", "red"))
        print(grep_res.stdout)
        raise Exit(code=1)


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
    with_unit_test=False,
):
    check_for_ninja(ctx)
    nf_path = os.path.join(ctx.cwd, 'system-probe.ninja')
    ninja_generate(
        ctx, nf_path, windows, major_version, arch, debug, strip_object_files, kernel_release, with_unit_test
    )
    explain_opt = "-d explain" if explain else ""
    if task:
        ctx.run(f"ninja {explain_opt} -f {nf_path} -t {task}")
    else:
        with open("compile_commands.json", "w") as compiledb:
            ctx.run(f"ninja -f {nf_path} -t compdb {target}", out_stream=compiledb)
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

    if clang_version_str != CLANG_VERSION_RUNTIME:
        # download correct version from dd-agent-omnibus S3 bucket
        clang_url = f"https://dd-agent-omnibus.s3.amazonaws.com/llvm/clang-{CLANG_VERSION_RUNTIME}.{arch}"
        ctx.run(f"{sudo} wget -q {clang_url} -O /opt/datadog-agent/embedded/bin/clang-bpf")
        ctx.run(f"{sudo} chmod 0755 /opt/datadog-agent/embedded/bin/clang-bpf")

    if llc_version_str != CLANG_VERSION_RUNTIME:
        llc_url = f"https://dd-agent-omnibus.s3.amazonaws.com/llvm/llc-{CLANG_VERSION_RUNTIME}.{arch}"
        ctx.run(f"{sudo} wget -q {llc_url} -O /opt/datadog-agent/embedded/bin/llc-bpf")
        ctx.run(f"{sudo} chmod 0755 /opt/datadog-agent/embedded/bin/llc-bpf")


def verify_system_clang_version(ctx):
    if os.getenv('DD_SYSPROBE_SKIP_CLANG_CHECK') == "true":
        return

    clang_res = ctx.run("clang --version", warn=True)
    clang_version_str = ""
    if clang_res.ok:
        clang_version_parts = clang_res.stdout.splitlines()[0].split(" ")
        version_index = clang_version_parts.index("version")
        clang_version_str = clang_version_parts[version_index + 1].split("-")[0]

    if not clang_version_str.startswith(CLANG_VERSION_SYSTEM_PREFIX):
        raise Exit(
            f"unsupported clang version {clang_version_str} in use. Please install {CLANG_VERSION_SYSTEM_PREFIX}."
        )


def build_object_files(
    ctx,
    windows=is_windows,
    major_version='7',
    arch=CURRENT_ARCH,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
    with_unit_test=False,
):
    build_dir = os.path.join("pkg", "ebpf", "bytecode", "build")

    if not windows:
        verify_system_clang_version(ctx)
        # if clang is missing, subsequent calls to ctx.run("clang ...") will fail silently
        setup_runtime_clang(ctx)

        if strip_object_files:
            print("checking for llvm-strip...")
            ctx.run("which llvm-strip")

        check_for_inline(ctx)
        ctx.run(f"mkdir -p -m 0755 {build_dir}/runtime")
        ctx.run(f"mkdir -p -m 0755 {build_dir}/co-re")

    run_ninja(
        ctx,
        explain=True,
        windows=windows,
        major_version=major_version,
        arch=arch,
        kernel_release=kernel_release,
        debug=debug,
        strip_object_files=strip_object_files,
        with_unit_test=with_unit_test,
    )

    if not windows:
        sudo = "" if is_root() else "sudo"
        ctx.run(f"{sudo} mkdir -p {EMBEDDED_SHARE_DIR}")

        java_dir = os.path.join("pkg", "network", "protocols", "tls", "java")
        ctx.run(f"{sudo} mkdir -p {EMBEDDED_SHARE_JAVA_DIR}")
        ctx.run(f"{sudo} install -m644 -oroot -groot {java_dir}/agent-usm.jar {EMBEDDED_SHARE_JAVA_DIR}/agent-usm.jar")

        if ctx.run("command -v rsync >/dev/null 2>&1", warn=True, hide=True).ok:
            rsync_filter = "--filter='+ */' --filter='+ *.o' --filter='+ *.c' --filter='- *'"
            ctx.run(
                f"{sudo} rsync --chmod=F644 --chown=root:root -rvt {rsync_filter} {build_dir}/ {EMBEDDED_SHARE_DIR}"
            )
        else:
            with ctx.cd(build_dir):

                def cp_cmd(out_dir):
                    dest = os.path.join(EMBEDDED_SHARE_DIR, out_dir)
                    return " ".join(
                        [
                            f"-execdir cp -p {{}} {dest}/ \\;",
                            f"-execdir chown root:root {dest}/{{}} \\;",
                            f"-execdir chmod 0644 {dest}/{{}} \\;",
                        ]
                    )

                ctx.run(f"{sudo} find . -maxdepth 1 -type f -name '*.o' {cp_cmd('.')}")
                ctx.run(f"{sudo} mkdir -p {EMBEDDED_SHARE_DIR}/co-re")
                ctx.run(f"{sudo} find ./co-re -maxdepth 1 -type f -name '*.o' {cp_cmd('co-re')}")
                ctx.run(f"{sudo} mkdir -p {EMBEDDED_SHARE_DIR}/runtime")
                ctx.run(f"{sudo} find ./runtime -maxdepth 1 -type f -name '*.c' {cp_cmd('runtime')}")


def build_cws_object_files(
    ctx,
    major_version='7',
    arch=CURRENT_ARCH,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
    with_unit_test=False,
):
    run_ninja(
        ctx,
        target="cws",
        major_version=major_version,
        arch=arch,
        debug=debug,
        strip_object_files=strip_object_files,
        kernel_release=kernel_release,
        with_unit_test=with_unit_test,
    )


@task
def object_files(ctx, kernel_release=None, with_unit_test=False):
    build_object_files(ctx, kernel_release=kernel_release, with_unit_test=with_unit_test)


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


@task
def generate_lookup_tables(ctx, windows=is_windows):
    if windows:
        return

    lookup_table_generate_files = [
        "./pkg/network/go/goid/main.go",
        "./pkg/network/protocols/http/gotls/lookup/main.go",
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


def is_bpftool_compatible(ctx):
    try:
        ctx.run("bpftool gen min_core_btf 2>&1 | grep -q \"'min_core_btf' needs at least 3 arguments, 0 found\"")
        return True
    except Exception:
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


def kitchen_prepare_btfs(ctx, files_dir, arch=CURRENT_ARCH):
    btf_dir = "/opt/datadog-agent/embedded/share/system-probe/ebpf/co-re/btf"

    if arch == "x64":
        arch = "x86_64"
    elif arch == "arm64":
        arch = "aarch64"

    if not os.path.exists(f"{btf_dir}/kitchen-btfs-{arch}.tar.xz"):
        exit("BTFs for kitchen test environments not found. Please update & re-provision your dev VM.")

    sudo = "sudo" if not is_root() else ""
    ctx.run(f"{sudo} chmod -R 0777 {btf_dir}")

    if not os.path.exists(f"{btf_dir}/kitchen-btfs-{arch}"):
        ctx.run(
            f"mkdir {btf_dir}/kitchen-btfs-{arch} && "
            + f"tar xf {btf_dir}/kitchen-btfs-{arch}.tar.xz -C {btf_dir}/kitchen-btfs-{arch}"
        )

    can_minimize = True
    if not is_bpftool_compatible(ctx):
        print(
            "Cannot minimize BTFs: bpftool version 6 or higher is required: preparing kitchen environment with full sized BTFs instead."
        )
        can_minimize = False

    if can_minimize:
        co_re_programs = " ".join(glob.glob("/opt/datadog-agent/embedded/share/system-probe/ebpf/co-re/*.o"))
        generate_minimized_btfs(
            ctx,
            source_dir=f"{btf_dir}/kitchen-btfs-{arch}",
            output_dir=f"{btf_dir}/minimized-btfs",
            input_bpf_programs=co_re_programs,
        )

        ctx.run(
            f"cd {btf_dir}/minimized-btfs && "
            + "tar -cJf minimized-btfs.tar.xz * && "
            + f"mv minimized-btfs.tar.xz {files_dir}"
        )
    else:
        ctx.run(f"cp {btf_dir}/kitchen-btfs-{arch}.tar.xz {files_dir}/minimized-btfs.tar.xz")


@task
def generate_minimized_btfs(
    ctx,
    source_dir,
    output_dir,
    input_bpf_programs,
):
    """
    Given an input directory containing compressed full-sized BTFs, generates an identically-structured
    output directory containing compressed minimized versions of those BTFs, tailored to the given
    bpf program(s).
    """

    # If there are no input programs, we don't need to actually do anything; however, in order to
    # prevent CI jobs from failing, we'll create a dummy output directory
    if input_bpf_programs == "":
        ctx.run(f"mkdir -p {output_dir}/dummy_data")
        return

    ctx.run(f"mkdir -p {output_dir}")

    check_for_ninja(ctx)

    ninja_file_path = os.path.join(ctx.cwd, 'generate-minimized-btfs.ninja')
    with open(ninja_file_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file, width=180)

        nw.rule(name="decompress_btf", command="tar -xf $in -C $target_directory")
        nw.rule(name="minimize_btf", command="bpftool gen min_core_btf $in $out $input_bpf_programs")
        nw.rule(name="compress_minimized_btf", command="tar -cJf $out -C $tar_working_directory $rel_in && rm $in")

        for root, dirs, files in os.walk(source_dir):
            path_from_root = os.path.relpath(root, source_dir)

            for d in dirs:
                output_subdir = os.path.join(output_dir, path_from_root, d)
                ctx.run(f"mkdir -p {output_subdir}")

            for file in files:
                if not file.endswith(".tar.xz"):
                    continue

                btf_filename = file[: -len(".tar.xz")]
                minimized_btf_path = os.path.join(output_dir, path_from_root, btf_filename)

                nw.build(
                    rule="decompress_btf",
                    inputs=[os.path.join(root, file)],
                    outputs=[os.path.join(root, btf_filename)],
                    variables={
                        "target_directory": root,
                    },
                )

                nw.build(
                    rule="minimize_btf",
                    inputs=[os.path.join(root, btf_filename)],
                    outputs=[minimized_btf_path],
                    variables={
                        "input_bpf_programs": input_bpf_programs,
                    },
                )

                nw.build(
                    rule="compress_minimized_btf",
                    inputs=[minimized_btf_path],
                    outputs=[f"{minimized_btf_path}.tar.xz"],
                    variables={
                        "tar_working_directory": os.path.join(output_dir, path_from_root),
                        "rel_in": btf_filename,
                    },
                )

    ctx.run(f"ninja -f {ninja_file_path}")


@task
def generate_event_monitor_proto(ctx):
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

            ctx.run(
                f"protoc -I. {plugin_opts} --go_out=paths=source_relative:. --go-vtproto_out=. --go-vtproto_opt=features=marshal+unmarshal+size --go-grpc_out=paths=source_relative:. pkg/eventmonitor/proto/api/api.proto"
            )

    for path in glob.glob("pkg/eventmonitor/**/*.pb.go", recursive=True):
        print(f"replacing protoc version in {path}")
        with open(path) as f:
            content = f.read()

        replaced_content = re.sub(r"\/\/\s*protoc\s*v\d+\.\d+\.\d+", "//  protoc", content)
        with open(path, "w") as f:
            f.write(replaced_content)


@task
def print_failed_tests(_, output_dir):
    fail_count = 0
    for testjson_tgz in glob.glob(f"{output_dir}/**/testjson.tar.gz"):
        test_platform = os.path.basename(os.path.dirname(testjson_tgz))
        test_results = {}

        if os.path.isdir(testjson_tgz):
            # handle weird kitchen bug where it places the tarball in a subdirectory of the same name
            testjson_tgz = os.path.join(testjson_tgz, "testjson.tar.gz")

        with tempfile.TemporaryDirectory() as unpack_dir:
            with tarfile.open(testjson_tgz) as tgz:
                tgz.extractall(path=unpack_dir)

            for test_json in glob.glob(f"{unpack_dir}/*.json"):
                with open(test_json) as tf:
                    for line in tf:
                        json_test = json.loads(line.strip())
                        if 'Test' in json_test:
                            name = json_test['Test']
                            package = json_test['Package']
                            action = json_test["Action"]

                            if action == "pass" or action == "fail" or action == "skip":
                                test_key = f"{package}.{name}"
                                res = test_results.get(test_key)
                                if res is None:
                                    test_results[test_key] = action
                                    continue

                                if res == "fail":
                                    print(f"re-ran [{test_platform}] {package} {name}: {action}")
                                if (action == "pass" or action == "skip") and res == "fail":
                                    test_results[test_key] = action

        for key, res in test_results.items():
            if res == "fail":
                package, name = key.split(".", maxsplit=1)
                print(color_message(f"FAIL: [{test_platform}] {package} {name}", "red"))
                fail_count += 1

    if fail_count > 0:
        raise Exit(code=1)


@task
def save_test_dockers(ctx, output_dir, arch, windows=is_windows):
    import yaml

    if windows:
        return

    docker_compose_paths = glob.glob("./pkg/network/protocols/**/*/docker-compose.yml", recursive=True)
    # Add relative docker-compose paths
    # For example:
    #   docker_compose_paths.append("./pkg/network/protocols/dockers/testdata/docker-compose.yml")

    images = set()
    for docker_compose_path in docker_compose_paths:
        with open(docker_compose_path, "r") as f:
            docker_compose = yaml.safe_load(f.read())
        for component in docker_compose["services"]:
            images.add(docker_compose["services"][component]["image"])

    # Java tests have dynamic images in docker-compose.yml
    images.update(
        ["openjdk:21-oraclelinux8", "openjdk:15-oraclelinux8", "openjdk:8u151-jre", "menci/archlinuxarm:base"]
    )

    # Special use-case in javatls
    images.remove("${IMAGE_VERSION}")
    for image in images:
        output_path = image.translate(str.maketrans('', '', string.punctuation))
        ctx.run(f"docker pull --platform linux/{arch} {image}")
        ctx.run(f"docker save {image} > {os.path.join(output_dir, output_path)}.tar")


@task
def start_microvms(
    ctx,
    infra_env,
    instance_type_x86=None,
    instance_type_arm=None,
    x86_ami_id=None,
    arm_ami_id=None,
    destroy=False,
    ssh_key_name=None,
    ssh_key_path=None,
    dependencies_dir=None,
    shutdown_period=320,
    stack_name="kernel-matrix-testing-system",
    vmconfig=None,
    local=False,
):
    args = [
        f"--instance-type-x86 {instance_type_x86}" if instance_type_x86 else "",
        f"--instance-type-arm {instance_type_arm}" if instance_type_arm else "",
        f"--x86-ami-id {x86_ami_id}" if x86_ami_id else "",
        f"--arm-ami-id {arm_ami_id}" if arm_ami_id else "",
        "--destroy" if destroy else "",
        f"--ssh-key-path {ssh_key_path}" if ssh_key_path else "",
        f"--ssh-key-name {ssh_key_name}" if ssh_key_name else "",
        f"--infra-env {infra_env}",
        f"--shutdown-period {shutdown_period}",
        f"--dependencies-dir {dependencies_dir}" if dependencies_dir else "",
        f"--name {stack_name}",
        f"--vmconfig {vmconfig}" if vmconfig else "",
        "--local" if local else "",
    ]

    go_args = ' '.join(filter(lambda x: x != "", args))
    ctx.run(
        f"cd ./test/new-e2e && go run ./scenarios/system-probe/main.go {go_args}",
    )

from __future__ import annotations

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

import requests
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.agent import BUNDLED_AGENTS
from tasks.agent import build as agent_build
from tasks.build_tags import UNIT_TEST_TAGS, get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.build.ninja import NinjaWriter
from tasks.libs.common.color import color_message
from tasks.libs.common.utils import (
    REPO_PATH,
    bin_name,
    environ,
    get_build_flags,
    get_common_test_args,
    get_gobin,
    get_version_numeric_only,
    parse_kernel_version,
)
from tasks.libs.types.arch import ALL_ARCHS, Arch, get_arch
from tasks.windows_resources import MESSAGESTRINGS_MC_PATH

BIN_DIR = os.path.join(".", "bin", "system-probe")
BIN_PATH = os.path.join(BIN_DIR, bin_name("system-probe"))

BPF_TAG = "linux_bpf"
BUNDLE_TAG = "ebpf_bindata"
NPM_TAG = "npm"

KITCHEN_DIR = os.getenv('DD_AGENT_TESTING_DIR') or os.path.normpath(os.path.join(os.getcwd(), "test", "kitchen"))
KITCHEN_ARTIFACT_DIR = os.path.join(KITCHEN_DIR, "site-cookbooks", "dd-system-probe-check", "files", "default", "tests")
TEST_PACKAGES_LIST = [
    "./pkg/ebpf/...",
    "./pkg/network/...",
    "./pkg/collector/corechecks/ebpf/...",
    "./pkg/process/monitor/...",
]
TEST_PACKAGES = " ".join(TEST_PACKAGES_LIST)
TEST_TIMEOUTS = {
    "pkg/network/tracer$": "0",
    "pkg/network/protocols/http$": "0",
    "pkg/network/protocols": "5m",
}
CWS_PREBUILT_MINIMUM_KERNEL_VERSION = (5, 8, 0)
EMBEDDED_SHARE_DIR = os.path.join("/opt", "datadog-agent", "embedded", "share", "system-probe", "ebpf")
EMBEDDED_SHARE_JAVA_DIR = os.path.join("/opt", "datadog-agent", "embedded", "share", "system-probe", "java")

is_windows = sys.platform == "win32"
is_macos = sys.platform == "darwin"

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

extra_cflags = []


def get_ebpf_build_dir(arch: Arch) -> Path:
    return Path("pkg/ebpf/bytecode/build") / arch.name


def ninja_define_windows_resources(ctx, nw: NinjaWriter, major_version):
    maj_ver, min_ver, patch_ver = get_version_numeric_only(ctx, major_version=major_version).split(".")
    nw.variable("maj_ver", maj_ver)
    nw.variable("min_ver", min_ver)
    nw.variable("patch_ver", patch_ver)
    nw.variable("windrestarget", "pe-x86-64")
    nw.rule(name="windmc", command="windmc --target $windrestarget -r $rcdir -h $rcdir $in")
    nw.rule(
        name="windres",
        command="windres --define MAJ_VER=$maj_ver --define MIN_VER=$min_ver --define PATCH_VER=$patch_ver "
        + "-i $in --target $windrestarget -O coff -o $out",
    )


def ninja_define_ebpf_compiler(
    nw: NinjaWriter, strip_object_files=False, kernel_release=None, with_unit_test=False, arch: Arch | None = None
):
    if arch is not None and arch.is_cross_compiling():
        # -target ARCH is important even if we're just emitting LLVM. If we're cross-compiling, clang
        # might fail to interpret cross-arch assembly code (e.g, the headers with arm64-specific ASM code
        # of the linux kernel will fail compilation in x64 hosts due to unknown register names).
        nw.variable("target", f"-target {arch.gcc_arch} -emit-llvm")
    else:
        nw.variable("target", "-emit-llvm")
    nw.variable("ebpfflags", get_ebpf_build_flags(with_unit_test, arch=arch))
    nw.variable("kheaders", get_kernel_headers_flags(kernel_release, arch=arch))
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


def ninja_define_co_re_compiler(nw: NinjaWriter, arch: Arch | None = None):
    nw.variable("ebpfcoreflags", get_co_re_build_flags(arch))

    nw.rule(
        name="ebpfcoreclang",
        command="clang -MD -MF $out.d -target bpf $ebpfcoreflags $flags -c $in -o $out",
        depfile="$out.d",
    )


def ninja_define_exe_compiler(nw: NinjaWriter):
    nw.rule(
        name="execlang",
        command="clang -MD -MF $out.d $exeflags $flags $in -o $out $exelibs",
        depfile="$out.d",
    )


def ninja_ebpf_program(nw: NinjaWriter, infile, outfile, variables=None):
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


def ninja_ebpf_co_re_program(nw: NinjaWriter, infile, outfile, variables=None):
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


def ninja_security_ebpf_programs(
    nw: NinjaWriter, build_dir: Path, debug: bool, kernel_release: str | None, arch: Arch | None = None
):
    security_agent_c_dir = os.path.join("pkg", "security", "ebpf", "c")
    security_agent_prebuilt_dir_include = os.path.join(security_agent_c_dir, "include")
    security_agent_prebuilt_dir = os.path.join(security_agent_c_dir, "prebuilt")

    kernel_headers = get_linux_header_dirs(
        kernel_release=kernel_release, minimal_kernel_release=CWS_PREBUILT_MINIMUM_KERNEL_VERSION, arch=arch
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


def ninja_network_ebpf_program(nw: NinjaWriter, infile, outfile, flags):
    ninja_ebpf_program(nw, infile, outfile, {"flags": flags})
    root, ext = os.path.splitext(outfile)
    ninja_ebpf_program(nw, infile, f"{root}-debug{ext}", {"flags": flags + " -DDEBUG=1"})


def ninja_network_ebpf_co_re_program(nw: NinjaWriter, infile, outfile, flags):
    ninja_ebpf_co_re_program(nw, infile, outfile, {"flags": flags})
    root, ext = os.path.splitext(outfile)
    ninja_ebpf_co_re_program(nw, infile, f"{root}-debug{ext}", {"flags": flags + " -DDEBUG=1"})


def ninja_network_ebpf_programs(nw: NinjaWriter, build_dir, co_re_build_dir):
    network_bpf_dir = os.path.join("pkg", "network", "ebpf")
    network_c_dir = os.path.join(network_bpf_dir, "c")

    network_flags = ["-Ipkg/network/ebpf/c", "-g"]
    network_programs = [
        "tracer",
        "prebuilt/usm",
        "prebuilt/usm_events_test",
        "prebuilt/shared-libraries",
        "prebuilt/conntrack",
    ]
    network_co_re_programs = [
        "tracer",
        "co-re/tracer-fentry",
        "runtime/usm",
        "runtime/shared-libraries",
        "runtime/conntrack",
    ]
    network_programs_wo_instrumentation = ["prebuilt/dns", "prebuilt/offset-guess"]

    for prog in network_programs:
        infile = os.path.join(network_c_dir, f"{prog}.c")
        outfile = os.path.join(build_dir, f"{os.path.basename(prog)}.o")
        ninja_network_ebpf_program(nw, infile, outfile, ' '.join(network_flags + extra_cflags))

    for prog in network_programs_wo_instrumentation:
        infile = os.path.join(network_c_dir, f"{prog}.c")
        outfile = os.path.join(build_dir, f"{os.path.basename(prog)}.o")
        ninja_network_ebpf_program(nw, infile, outfile, ' '.join(network_flags))

    for prog_path in network_co_re_programs:
        prog = os.path.basename(prog_path)
        src_dir = os.path.join(network_c_dir, os.path.dirname(prog_path))
        network_co_re_flags = [f"-I{src_dir}", "-Ipkg/network/ebpf/c"] + extra_cflags

        infile = os.path.join(src_dir, f"{prog}.c")
        outfile = os.path.join(co_re_build_dir, f"{prog}.o")
        ninja_network_ebpf_co_re_program(nw, infile, outfile, ' '.join(network_co_re_flags))


def ninja_test_ebpf_programs(nw: NinjaWriter, build_dir):
    ebpf_bpf_dir = os.path.join("pkg", "ebpf")
    ebpf_c_dir = os.path.join(ebpf_bpf_dir, "testdata", "c")
    test_flags = "-g -DDEBUG=1"

    test_programs = ["logdebug-test"]

    for prog in test_programs:
        infile = os.path.join(ebpf_c_dir, f"{prog}.c")
        outfile = os.path.join(build_dir, f"{os.path.basename(prog)}.o")
        ninja_ebpf_program(
            nw, infile, outfile, {"flags": test_flags}
        )  # All test ebpf programs are just for testing, so we always build them with debug symbols


def ninja_container_integrations_ebpf_programs(nw: NinjaWriter, co_re_build_dir):
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


def ninja_runtime_compilation_files(nw: NinjaWriter, gobin):
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

    nw.rule(
        name="headerincl", command="go generate -run=\"include_headers\" -mod=mod -tags linux_bpf $in", depfile="$out.d"
    )
    nw.rule(name="integrity", command="go generate -run=\"integrity\" -mod=mod -tags linux_bpf $in", depfile="$out.d")
    hash_dir = os.path.join(bc_dir, "runtime")
    rc_dir = os.path.join(build_dir, "runtime")
    for in_path, out_filename in runtime_compiler_files.items():
        c_file = os.path.join(rc_dir, f"{out_filename}.c")
        hash_file = os.path.join(hash_dir, f"{out_filename}.go")
        nw.build(
            inputs=[in_path],
            implicit=toolpaths,
            outputs=[c_file],
            rule="headerincl",
        )
        nw.build(
            inputs=[in_path],
            implicit=toolpaths + [c_file],
            outputs=[hash_file],
            rule="integrity",
        )


def ninja_cgo_type_files(nw: NinjaWriter):
    # TODO we could probably preprocess the input files to find out the dependencies
    nw.pool(name="cgo_pool", depth=1)
    if is_windows:
        go_platform = "windows"
        def_files = {
            "pkg/network/driver/types.go": [
                "pkg/network/driver/ddnpmapi.h",
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
            "pkg/network/protocols/postgres/types.go": [
                "pkg/network/ebpf/c/protocols/postgres/types.h",
            ],
            "pkg/ebpf/telemetry/types.go": [
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
    ctx: Context,
    ninja_path,
    major_version='7',
    arch: str | Arch = CURRENT_ARCH,
    debug=False,
    strip_object_files=False,
    kernel_release: str | None = None,
    with_unit_test=False,
):
    arch = get_arch(arch)
    build_dir = get_ebpf_build_dir(arch)
    co_re_build_dir = os.path.join(build_dir, "co-re")

    with open(ninja_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file, width=120)

        if is_windows:
            ninja_define_windows_resources(ctx, nw, major_version)
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
            ninja_define_ebpf_compiler(nw, strip_object_files, kernel_release, with_unit_test, arch=arch)
            ninja_define_co_re_compiler(nw, arch=arch)
            ninja_network_ebpf_programs(nw, build_dir, co_re_build_dir)
            ninja_test_ebpf_programs(nw, build_dir)
            ninja_security_ebpf_programs(nw, build_dir, debug, kernel_release, arch=arch)
            ninja_container_integrations_ebpf_programs(nw, co_re_build_dir)
            ninja_runtime_compilation_files(nw, gobin)

        ninja_cgo_type_files(nw)


@task
def build(
    ctx,
    race=False,
    incremental_build=True,
    major_version='7',
    python_runtimes='3',
    go_mod="mod",
    arch: str = CURRENT_ARCH,
    bundle_ebpf=False,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
    strip_binary=False,
    with_unit_test=False,
    bundle=True,
    instrument_trampoline=False,
):
    """
    Build the system-probe
    """
    if not is_macos:
        build_object_files(
            ctx,
            major_version=major_version,
            kernel_release=kernel_release,
            debug=debug,
            strip_object_files=strip_object_files,
            with_unit_test=with_unit_test,
            instrument_trampoline=instrument_trampoline,
        )

    build_sysprobe_binary(
        ctx,
        major_version=major_version,
        python_runtimes=python_runtimes,
        bundle_ebpf=bundle_ebpf,
        go_mod=go_mod,
        race=race,
        incremental_build=incremental_build,
        strip_binary=strip_binary,
        bundle=bundle,
        arch=arch,
    )


@task
def clean(
    ctx,
):
    clean_object_files(
        ctx,
    )
    ctx.run("go clean -cache")


@task
def build_sysprobe_binary(
    ctx,
    race=False,
    incremental_build=True,
    major_version='7',
    python_runtimes='3',
    go_mod="mod",
    arch: str = CURRENT_ARCH,
    binary=BIN_PATH,
    install_path=None,
    bundle_ebpf=False,
    strip_binary=False,
    bundle=True,
) -> None:
    if bundle and not is_windows:
        return agent_build(
            ctx,
            race=race,
            major_version=major_version,
            python_runtimes=python_runtimes,
            go_mod=go_mod,
            bundle_ebpf=bundle_ebpf,
            bundle=BUNDLED_AGENTS[AgentFlavor.base] + ["system-probe"],
        )

    arch_obj = get_arch(arch)

    ldflags, gcflags, env = get_build_flags(
        ctx, install_path=install_path, major_version=major_version, python_runtimes=python_runtimes, arch=arch_obj
    )

    build_tags = get_default_build_tags(build="system-probe")
    if bundle_ebpf:
        build_tags.append(BUNDLE_TAG)
    if strip_binary:
        ldflags += ' -s -w'

    if os.path.exists(binary):
        os.remove(binary)

    cmd = 'go build -mod={go_mod}{race_opt}{build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/system-probe'

    args = {
        "go_mod": go_mod,
        "race_opt": " -race" if race else "",
        "build_type": "" if incremental_build else " -a",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": binary,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)


def get_sysprobe_buildtags(is_windows, bundle_ebpf):
    build_tags = [NPM_TAG]
    build_tags.extend(UNIT_TEST_TAGS)
    if not is_windows:
        build_tags.append(BPF_TAG)
        if bundle_ebpf:
            build_tags.append(BUNDLE_TAG)
    return build_tags


@task
def test(
    ctx,
    packages=TEST_PACKAGES,
    bundle_ebpf=False,
    output_path=None,
    skip_object_files=False,
    run=None,
    failfast=False,
    kernel_release=None,
    timeout=None,
    extra_arguments="",
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

    if not skip_object_files:
        build_object_files(
            ctx,
            kernel_release=kernel_release,
            instrument_trampoline=False,
        )

    build_tags = get_sysprobe_buildtags(is_windows, bundle_ebpf)

    args = get_common_test_args(build_tags, failfast)
    args["output_params"] = f"-c -o {output_path}" if output_path else ""
    args["run"] = f"-run {run}" if run else ""
    args["go"] = "go"
    args["sudo"] = "sudo -E " if not is_windows and not output_path and not is_root() else ""
    args["extra_arguments"] = extra_arguments

    _, _, env = get_build_flags(ctx)
    env["DD_SYSTEM_PROBE_BPF_DIR"] = EMBEDDED_SHARE_DIR

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
        cmd = '{sudo}{go} test -mod=mod -v {failfast} {timeout} -tags "{build_tags}" {extra_arguments} {output_params} {dir} {run}'
        res = ctx.run(cmd.format(**args), env=env, warn=True)
        if res.exited is None or res.exited > 0:
            failed_pkgs.append(os.path.relpath(pdir, ctx.cwd))
            if failfast:
                break

    if len(failed_pkgs) > 0:
        print(color_message("failed packages:\n" + "\n".join(failed_pkgs), "red"))
        raise Exit(code=1, message="system-probe tests failed")


@task(
    help={
        "package": "The package to test. REQUIRED ",
        "skip_object_files": "Skip rebuilding the object files.",
        "run": "The name of the test to run. REQUIRED",
    }
)
def test_debug(
    ctx,
    package,
    run,
    bundle_ebpf=False,
    skip_object_files=False,
    failfast=False,
    kernel_release=None,
):
    """
    Run delve on a specific system-probe test.
    """

    if os.getenv("GOPATH") is None:
        raise Exit(
            code=1,
            message="GOPATH is not set, if you are running tests with sudo, you may need to use the -E option to "
            "preserve your environment",
        )

    if not skip_object_files:
        build_object_files(
            ctx,
            kernel_release=kernel_release,
            instrument_trampoline=False,
        )

    build_tags = [NPM_TAG]
    build_tags.extend(UNIT_TEST_TAGS)
    if not is_windows:
        build_tags.append(BPF_TAG)
        if bundle_ebpf:
            build_tags.append(BUNDLE_TAG)

    args = get_common_test_args(build_tags, failfast)
    args["run"] = run
    args["dlv"] = "dlv"
    args["sudo"] = "sudo -E " if not is_windows and not is_root() else ""
    args["dir"] = package

    _, _, env = get_build_flags(ctx)
    env["DD_SYSTEM_PROBE_BPF_DIR"] = EMBEDDED_SHARE_DIR

    cmd = '{sudo}{dlv} test {dir} --build-flags="-mod=mod -v {failfast} -tags={build_tags}" -- -test.run {run}'
    ctx.run(cmd.format(**args), env=env, pty=True, warn=True)


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
        dirs = (
            check_output(
                f"go list -find -f \"{{{{ .Dir }}}}\" -mod=mod -tags \"{','.join(build_tags)}\" {pkg}",
                shell=True,
            )
            .decode('utf-8')
            .strip()
            .split("\n")
        )
        # Some packages may not be available on all architectures, ignore them
        # instead of reporting empty path names
        target_packages += [dir for dir in dirs if len(dir) > 0]

    return target_packages


BUILD_COMMIT = os.path.join(KITCHEN_ARTIFACT_DIR, "build.commit")


def clean_build(ctx):
    if not os.path.exists(KITCHEN_ARTIFACT_DIR):
        return True

    if not os.path.exists(BUILD_COMMIT):
        return True

    # if this build happens on a new commit do it cleanly
    with open(BUILD_COMMIT) as f:
        build_commit = f.read().rstrip()
        curr_commit = ctx.run("git rev-parse HEAD", hide=True).stdout.rstrip()
        if curr_commit != build_commit:
            return True

    return False


def full_pkg_path(name):
    return os.path.join(os.getcwd(), name[name.index("pkg") :])


@task
def kitchen_prepare(ctx, kernel_release=None, ci=False, packages=""):
    """
    Compile test suite for kitchen
    """
    build_tags = [NPM_TAG]
    if not is_windows:
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
        target_path = os.path.join(KITCHEN_ARTIFACT_DIR, os.path.relpath(pkg, os.getcwd()))
        target_bin = "testsuite"
        if is_windows:
            target_bin = "testsuite.exe"

        test(
            ctx,
            packages=pkg,
            skip_object_files=(i != 0),
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

        for gobin in ["gotls_client", "grpc_external_server", "external_unix_proxy_server", "fmapper", "prefetch_file"]:
            src_file_path = os.path.join(pkg, f"{gobin}.go")
            if not is_windows and os.path.isdir(pkg) and os.path.isfile(src_file_path):
                binary_path = os.path.join(target_path, gobin)
                with chdir(pkg):
                    ctx.run(f"go build -o {binary_path} -tags=\"test\" -ldflags=\"-extldflags '-static'\" {gobin}.go")

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
    ignored_files = [
        "pkg/ebpf/c/bpf_builtins.h",
        "pkg/ebpf/c/bpf_core_read.h",
        "pkg/ebpf/c/bpf_cross_compile.h",
        "pkg/ebpf/c/bpf_endian.h",
        "pkg/ebpf/c/bpf_helpers.h",
        "pkg/ebpf/c/bpf_helper_defs.h",
        "pkg/ebpf/c/bpf_tracing.h",
        "pkg/ebpf/c/bpf_tracing_custom.h",
        "pkg/ebpf/c/compiler.h",
        "pkg/ebpf/c/map-defs.h",
        "pkg/ebpf/c/vmlinux_5_15_0.h",
        "pkg/ebpf/c/vmlinux_5_15_0_arm.h",
        "pkg/ebpf/compiler/clang-stdarg.h",
    ]
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
    network_checks = [
        "-readability-function-cognitive-complexity",
        "-readability-isolate-declaration",
        "-clang-analyzer-security.insecureAPI.bcmp",
    ]
    run_tidy(
        ctx,
        files=network_files,
        build_flags=network_flags,
        fix=fix,
        fail_on_issue=fail_on_issue,
        checks=network_checks,
    )

    security_agent_c_dir = os.path.join(".", "pkg", "security", "ebpf", "c")
    security_files = list(base_files)
    security_files.extend(glob.glob(f"{security_agent_c_dir}/**/*.c"))
    security_flags = list(build_flags)
    security_flags.append(f"-I{security_agent_c_dir}")
    security_flags.append(f"-I{security_agent_c_dir}/include")
    security_flags.append("-DUSE_SYSCALL_WRAPPER=0")
    security_checks = ["-readability-function-cognitive-complexity", "-readability-isolate-declaration"]
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


def get_kernel_arch() -> Arch:
    # Mapping used by the kernel, from https://elixir.bootlin.com/linux/latest/source/scripts/subarch.include
    kernel_arch = (
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

    return get_arch(kernel_arch)


def get_linux_header_dirs(
    kernel_release: str | None = None,
    minimal_kernel_release: tuple[int, int, int] | None = None,
    arch: Arch | None = None,
) -> list[Path]:
    if not kernel_release:
        os_info = os.uname()
        kernel_release = os_info.release
    kernel_release_vers = parse_kernel_version(kernel_release)

    if arch is None:
        arch = get_arch("local")

    kernels_path = Path("/usr/src/kernels")
    usr_src_path = Path("/usr/src")
    lib_modules_path = Path("/lib/modules")
    candidates: set[Path] = set()

    if kernels_path.is_dir():  # Doesn't always exist. For the others we do want exceptions as they should exist
        candidates.update(kernels_path.iterdir())
    candidates.update(d for d in usr_src_path.iterdir() if d.name.startswith("linux-"))
    candidates.update(lib_modules_path.glob("*/build"))
    candidates.update(lib_modules_path.glob("*/source"))
    candidates = {c.resolve() for c in candidates if c.is_dir()}

    paths_with_priority_and_sort_order: list[tuple[int, int, Path]] = []
    discarded_paths: list[tuple[str, Path]] = []
    for path in candidates:
        # Get the kernel name, discard when we cannot get a kernel version out of them
        candidate_kernel = path.name.removeprefix("linux-headers-").removeprefix("linux-kbuild-")
        try:
            candidate_kernel_vers = parse_kernel_version(candidate_kernel)
        except ValueError:
            discarded_paths.append(("no kernel version", path))
            continue

        priority = 0
        sort_order = 100
        if candidate_kernel_vers == kernel_release_vers:
            priority += 1

        # Completely discard kernels that don't match the minimal version
        if minimal_kernel_release is not None and candidate_kernel_vers < minimal_kernel_release:
            discarded_paths.append(
                (f"kernel version {candidate_kernel_vers} less than minimal {minimal_kernel_release}", path)
            )
            continue

        # Give more priority to kernels that match the desired architecture.
        matching_kernel_archs = {a for a in ALL_ARCHS if any(x in candidate_kernel for x in a.spellings)}
        if arch in matching_kernel_archs:
            sort_order = 0  # Matching architecture paths should be sorted the first
            priority += 1
        elif len(matching_kernel_archs) == 0:
            # If we find no match, assume it's a common path (e.g., -common folders in Debian)
            # which matches everything
            sort_order = 1
            priority += 1

        # Don't add duplicates
        if not any(p == path for _, _, p in paths_with_priority_and_sort_order):
            paths_with_priority_and_sort_order.append((priority, sort_order, path))

    if len(paths_with_priority_and_sort_order) == 0:
        raise ValueError(f"No kernel header path found. Discarded paths and reasons: {discarded_paths}")

    # Only get paths with maximum priority, those are the ones that match the best.
    # Note that there might be multiple of them (e.g., the arch-specific and the common path)
    max_priority = max(prio for prio, _, _ in paths_with_priority_and_sort_order)
    linux_headers = [(path, ord) for prio, ord, path in paths_with_priority_and_sort_order if prio == max_priority]

    # Include sort order is important, ensure we respect the sort order we defined while
    # discovering the paths
    linux_headers = [path for path, _ in sorted(linux_headers, key=lambda x: x[1])]

    # Now construct all subdirectories. Again, order is important, so keep the list
    subdirs = [
        "include",
        "include/uapi",
        "include/generated/uapi",
        f"arch/{arch.kernel_arch}/include",
        f"arch/{arch.kernel_arch}/include/uapi",
        f"arch/{arch.kernel_arch}/include/generated",
        f"arch/{arch.kernel_arch}/include/generated/uapi",
    ]

    dirs: list[Path] = []
    for d in linux_headers:
        for s in subdirs:
            dirs.append(d / s)

    return dirs


@task
def print_linux_include_paths(_: Context, arch: str | None = None):
    """
    Print the result of the linux header directories discovery. Useful for debugging the build process.
    """
    paths = get_linux_header_dirs(arch=get_arch(arch or "local"))
    print("\n".join(str(p) for p in paths))


def get_ebpf_build_flags(unit_test=False, arch: Arch | None = None):
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
    if arch is not None:
        if arch.kernel_arch is None:
            raise Exit(f"eBPF architecture not supported for {arch}")
        flags.append(f"-D__TARGET_ARCH_{arch.kernel_arch}")
        flags.append(f"-D__{arch.gcc_arch.replace('-', '_')}__")

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


def get_co_re_build_flags(arch: Arch | None = None):
    flags = get_ebpf_build_flags(arch=arch)

    flags.remove('-DCOMPILE_PREBUILT')
    flags.remove('-DCONFIG_64BIT')
    flags.remove('-include pkg/ebpf/c/asm_goto_workaround.h')

    flags.extend(
        [
            "-DCOMPILE_CORE",
            '-emit-llvm',
            '-g',
        ]
    )

    if arch is None:
        arch = get_kernel_arch()

    arch_define = f"-D__TARGET_ARCH_{arch.kernel_arch}"
    if arch_define not in flags:
        flags.append(arch_define)

    return flags


def get_kernel_headers_flags(kernel_release=None, minimal_kernel_release=None, arch: Arch | None = None):
    return [
        f"-isystem{d}"
        for d in get_linux_header_dirs(
            kernel_release=kernel_release, minimal_kernel_release=minimal_kernel_release, arch=arch
        )
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
    ctx: Context,
    task="",
    target="",
    explain=False,
    major_version='7',
    arch: str | Arch = CURRENT_ARCH,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
    with_unit_test=False,
) -> None:
    check_for_ninja(ctx)
    nf_path = os.path.join(ctx.cwd, 'system-probe.ninja')
    ninja_generate(ctx, nf_path, major_version, debug, strip_object_files, kernel_release, with_unit_test, arch=arch)
    explain_opt = "-d explain" if explain else ""
    if task:
        ctx.run(f"ninja {explain_opt} -f {nf_path} -t {task}")
    else:
        with open("compile_commands.json", "w") as compiledb:
            ctx.run(f"ninja -f {nf_path} -t compdb {target}", out_stream=compiledb)
        ctx.run(f"ninja {explain_opt} -f {nf_path} {target}")


def setup_runtime_clang(
    ctx: Context, arch: Arch | None = None, target_dir: Path | str = "/opt/datadog-agent/embedded/bin"
) -> None:
    target_dir = Path(target_dir)
    needs_sudo = not os.access(target_dir, os.W_OK)
    sudo = "sudo" if not is_root() and needs_sudo else ""

    if arch is None:
        arch = get_arch("local")

    clang_bpf_path = target_dir / "clang-bpf"
    llc_bpf_path = target_dir / "llc-bpf"
    needs_clang_download, needs_llc_download = True, True

    if not arch.is_cross_compiling():
        # We can check the version of clang and llc on the system, we have the same arch and can
        # execute the binaries. This way we can omit the download if the binaries exist and the version
        # matches the desired one
        clang_res = ctx.run(f"{sudo} {clang_bpf_path} --version", warn=True)
        if clang_res is not None and clang_res.ok:
            clang_version_str = clang_res.stdout.split("\n")[0].split(" ")[2].strip()
            needs_clang_download = clang_version_str != CLANG_VERSION_RUNTIME

        llc_res = ctx.run(f"{sudo} {llc_bpf_path} --version", warn=True)
        if llc_res is not None and llc_res.ok:
            llc_version_str = llc_res.stdout.split("\n")[1].strip().split(" ")[2].strip()
            needs_llc_download = llc_version_str != CLANG_VERSION_RUNTIME
    else:
        # If we're cross-compiling we cannot check the version of clang and llc on the system,
        # so we download them only if they don't exist
        needs_clang_download = not clang_bpf_path.exists()
        needs_llc_download = not llc_bpf_path.exists()

    if not target_dir.exists():
        ctx.run(f"{sudo} mkdir -p {target_dir}")

    if needs_clang_download:
        # download correct version from dd-agent-omnibus S3 bucket
        clang_url = f"https://dd-agent-omnibus.s3.amazonaws.com/llvm/clang-{CLANG_VERSION_RUNTIME}.{arch.name}"
        ctx.run(f"{sudo} wget -q {clang_url} -O {clang_bpf_path}")
        ctx.run(f"{sudo} chmod 0755 {clang_bpf_path}")

    if needs_llc_download:
        llc_url = f"https://dd-agent-omnibus.s3.amazonaws.com/llvm/llc-{CLANG_VERSION_RUNTIME}.{arch.name}"
        ctx.run(f"{sudo} wget -q {llc_url} -O {llc_bpf_path}")
        ctx.run(f"{sudo} chmod 0755 {llc_bpf_path}")


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
    major_version='7',
    arch: str = CURRENT_ARCH,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
    with_unit_test=False,
    instrument_trampoline=False,
) -> None:
    arch_obj = get_arch(arch)
    build_dir = get_ebpf_build_dir(arch_obj)

    if not is_windows:
        verify_system_clang_version(ctx)
        # if clang is missing, subsequent calls to ctx.run("clang ...") will fail silently
        setup_runtime_clang(ctx)

        if strip_object_files:
            print("checking for llvm-strip...")
            ctx.run("which llvm-strip")

        check_for_inline(ctx)
        ctx.run(f"mkdir -p -m 0755 {build_dir}/runtime")
        ctx.run(f"mkdir -p -m 0755 {build_dir}/co-re")

    global extra_cflags
    if instrument_trampoline:
        extra_cflags = extra_cflags + ["-pg", "-DINSTRUMENTATION_ENABLED"]

    run_ninja(
        ctx,
        explain=True,
        major_version=major_version,
        kernel_release=kernel_release,
        debug=debug,
        strip_object_files=strip_object_files,
        with_unit_test=with_unit_test,
        arch=arch,
    )

    if not is_windows:
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
    arch: str | Arch = CURRENT_ARCH,
    kernel_release=None,
    debug=False,
    strip_object_files=False,
    with_unit_test=False,
):
    run_ninja(
        ctx,
        target="cws",
        major_version=major_version,
        debug=debug,
        strip_object_files=strip_object_files,
        kernel_release=kernel_release,
        with_unit_test=with_unit_test,
    )


@task
def object_files(ctx, kernel_release=None, with_unit_test=False, arch: str | None = None):
    build_object_files(
        ctx, kernel_release=kernel_release, with_unit_test=with_unit_test, instrument_trampoline=False, arch=arch
    )


def clean_object_files(ctx, major_version='7', kernel_release=None, debug=False, strip_object_files=False):
    run_ninja(
        ctx,
        task="clean",
        major_version=major_version,
        debug=debug,
        strip_object_files=strip_object_files,
        kernel_release=kernel_release,
    )


@task
def generate_lookup_tables(ctx):
    if is_windows:
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
    ctx.run("ninja --version")


def is_bpftool_compatible(ctx):
    try:
        ctx.run("bpftool gen min_core_btf 2>&1 | grep -q \"'min_core_btf' needs at least 3 arguments, 0 found\"")
        return True
    except Exception:
        return False


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


@task(iterable=['bpf_programs'])
def generate_minimized_btfs(ctx, source_dir, output_dir, bpf_programs):
    """
    Given an input directory containing compressed full-sized BTFs, generates an identically-structured
    output directory containing compressed minimized versions of those BTFs, tailored to the given
    bpf program(s).
    """

    # If there are no input programs, we don't need to actually do anything; however, in order to
    # prevent CI jobs from failing, we'll create a dummy output directory
    if len(bpf_programs) == 0:
        ctx.run(f"mkdir -p {output_dir}/dummy_data")
        return

    if len(bpf_programs) == 1 and os.path.isdir(bpf_programs[0]):
        programs_dir = os.path.abspath(bpf_programs[0])
        print(f"using all object files from directory {programs_dir}")
        bpf_programs = glob.glob(f"{programs_dir}/*.o")

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
                os.makedirs(output_subdir, exist_ok=True)

            for file in files:
                if not file.endswith(".btf.tar.xz"):
                    continue

                btf_filename = file.removesuffix(".tar.xz")
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
                        "input_bpf_programs": bpf_programs,
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

    ctx.run(f"ninja -f {ninja_file_path}", env={"NINJA_STATUS": "(%r running) (%c/s) (%es) [%f/%t] "})


@task
def process_btfhub_archive(ctx, branch="main"):
    """
    process btfhub-archive repo to only select BTF tarball files of a single architecture
    :param ctx: invoke context
    :param branch: branch of DataDog/btfhub-archive to clone
    """
    output_dir = os.getcwd()
    with tempfile.TemporaryDirectory() as temp_dir:
        with ctx.cd(temp_dir):
            clone_cmd = (
                f"git clone --depth=1 --single-branch --branch={branch} https://github.com/DataDog/btfhub-archive.git"
            )
            retries = 2
            downloaded = False

            while not downloaded and retries > 0:
                res = ctx.run(clone_cmd, warn=True)
                downloaded = res is not None and res.ok

                if not downloaded:
                    retries -= 1
                    print(f"Failed to clone btfhub-archive. Remaining retries: {retries}")

            if not downloaded:
                raise Exit("Failed to clone btfhub-archive")

            with ctx.cd("btfhub-archive"):
                # iterate over all top-level directories, which are platforms (amzn, ubuntu, etc.)
                with os.scandir(ctx.cwd) as pit:
                    for pdir in pit:
                        if not pdir.is_dir() or pdir.name.startswith("."):
                            continue

                        # iterate over second-level directories, which are release versions (2, 20.04, etc.)
                        with os.scandir(pdir.path) as rit:
                            for rdir in rit:
                                if not rdir.is_dir() or rdir.is_symlink():
                                    continue

                                # iterate over arch directories
                                with os.scandir(rdir.path) as ait:
                                    for adir in ait:
                                        if not adir.is_dir() or adir.name not in {"x86_64", "arm64"}:
                                            continue

                                        print(f"{pdir.name}/{rdir.name}/{adir.name}")
                                        src_dir = adir.path
                                        # list BTF .tar.xz files in arch dir
                                        btf_files = os.listdir(src_dir)
                                        for file in btf_files:
                                            if not file.endswith(".tar.xz"):
                                                continue
                                            src_file = os.path.join(src_dir, file)

                                            # remove release and arch from destination
                                            btfs_dir = os.path.join(temp_dir, f"btfs-{adir.name}")
                                            dst_dir = os.path.join(btfs_dir, pdir.name)
                                            # ubuntu retains release version
                                            if pdir.name == "ubuntu":
                                                dst_dir = os.path.join(btfs_dir, pdir.name, rdir.name)

                                            os.makedirs(dst_dir, exist_ok=True)
                                            dst_file = os.path.join(dst_dir, file)
                                            if os.path.exists(dst_file):
                                                raise Exit(message=f"{dst_file} already exists")
                                            shutil.move(src_file, os.path.join(dst_dir, file))

        # generate both tarballs
        for arch in ["x86_64", "arm64"]:
            btfs_dir = os.path.join(temp_dir, f"btfs-{arch}")
            output_path = os.path.join(output_dir, f"btfs-{arch}.tar")
            # at least one file needs to be moved for directory to exist
            if os.path.exists(btfs_dir):
                with ctx.cd(temp_dir):
                    # include btfs-$ARCH as prefix for all paths
                    ctx.run(f"tar -cf {output_path} btfs-{arch}")


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
def save_test_dockers(ctx, output_dir, arch, use_crane=False):
    if is_windows:
        return

    # crane does not accept 'x86_64' as a valid architecture
    if arch == "x86_64":
        arch = "amd64"

    # only download images not present in preprepared vm disk
    resp = requests.get('https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/rootfs/docker.ls')
    docker_ls = {line for line in resp.text.split('\n') if line.strip()}

    images = _test_docker_image_list()
    for image in images - docker_ls:
        output_path = image.translate(str.maketrans('', '', string.punctuation))
        output_file = f"{os.path.join(output_dir, output_path)}.tar"
        if use_crane:
            ctx.run(f"crane pull --platform linux/{arch} {image} {output_file}")
        else:
            ctx.run(f"docker pull --platform linux/{arch} {image}")
            ctx.run(f"docker save {image} > {output_file}")


@task
def test_docker_image_list(_):
    images = _test_docker_image_list()
    print('\n'.join(images))


def _test_docker_image_list():
    import yaml

    docker_compose_paths = glob.glob("./pkg/network/protocols/**/*/docker-compose.yml", recursive=True)
    # Add relative docker-compose paths
    # For example:
    #   docker_compose_paths.append("./pkg/network/protocols/dockers/testdata/docker-compose.yml")

    images = set()
    for docker_compose_path in docker_compose_paths:
        with open(docker_compose_path) as f:
            docker_compose = yaml.safe_load(f.read())
        for component in docker_compose["services"]:
            images.add(docker_compose["services"][component]["image"])

    # Java tests have dynamic images in docker-compose.yml
    images.update(["menci/archlinuxarm:base"])

    # Special use-case in javatls
    images.remove("${IMAGE_VERSION}")
    # Temporary: GoTLS monitoring inside containers tests are flaky in the CI, so at the meantime, the tests are
    # disabled, so we can skip downloading a redundant image.
    images.remove("public.ecr.aws/b1o7r7e0/usm-team/go-httpbin:https")
    return images


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
    provision_instance=False,
    provision_microvms=False,
    run_agent=False,
    agent_version=None,
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
        "--run-agent" if run_agent else "",
        f"--agent-version {agent_version}" if agent_version else "",
        "--provision-instance" if provision_instance else "",
        "--provision-microvms" if provision_microvms else "",
    ]

    go_args = ' '.join(filter(lambda x: x != "", args))

    # building the binary improves start up time for local usage where we invoke this multiple times.
    ctx.run("cd ./test/new-e2e && go build -o start-microvms ./scenarios/system-probe/main.go")
    ctx.run(f"./test/new-e2e/start-microvms {go_args}")


@task
def save_build_outputs(ctx, destfile):
    ignored_extensions = {".bc"}
    ignored_files = {"cws", "integrity", "include_headers"}

    if not destfile.endswith(".tar.xz"):
        raise Exit(message="destfile must be a .tar.xz file")

    absdest = os.path.abspath(destfile)
    count = 0
    outfiles = []
    with tempfile.TemporaryDirectory() as stagedir:
        with open("compile_commands.json") as compiledb:
            for outputitem in json.load(compiledb):
                if "output" not in outputitem:
                    continue

                filedir, file = os.path.split(outputitem["output"])
                _, ext = os.path.splitext(file)
                if ext in ignored_extensions or file in ignored_files:
                    continue

                outdir = os.path.join(stagedir, filedir)
                ctx.run(f"mkdir -p {outdir}")
                ctx.run(f"cp {outputitem['output']} {outdir}/")
                outfiles.append(outputitem['output'])
                count += 1

        if count == 0:
            raise Exit(message="no build outputs captured")
        ctx.run(f"tar -C {stagedir} -cJf {absdest} .")

    outfiles.sort()
    for outfile in outfiles:
        ctx.run(f"sha256sum {outfile} >> {absdest}.sum")

"""Bazel rules for compiling eBPF programs (.c -> .bc -> .o)."""

load("@bazel_lib//lib:resource_sets.bzl", "resource_set_for")
load("@linux_headers//:defs.bzl", "KERNEL_ARCH", "KERNEL_HEADER_DIRS")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")

_TOOLCHAIN_TYPE = "@@//bazel/toolchains/llvm_bpf:llvm_bpf_toolchain_type"

_ARCH_DEFINES = {
    "x86_64": ["-D__TARGET_ARCH_x86", "-D__x86_64__"],
    "aarch64": ["-D__TARGET_ARCH_arm64", "-D__aarch64__"],
}

_COMMON_FLAGS = [
    "-D__KERNEL__",
    "-D__BPF_TRACING__",
    '-DKBUILD_MODNAME="ddsysprobe"',
    "-Wno-unused-value",
    "-Wno-pointer-sign",
    "-Wno-compare-distinct-pointer-types",
    "-Wunused",
    "-Wall",
    "-Werror",
    "-O2",
    "-fno-stack-protector",
    "-fno-color-diagnostics",
    "-fno-unwind-tables",
    "-fno-asynchronous-unwind-tables",
    "-fno-jump-tables",
    "-fmerge-all-constants",
]

_PREBUILT_FLAGS = [
    "-DCONFIG_64BIT",
    "-DCOMPILE_PREBUILT",
]

_CORE_FLAGS = [
    "-DCOMPILE_CORE",
    "-g",
]

def _get_arch_flags(target_arch):
    """Get architecture-specific defines."""
    if target_arch:
        return _ARCH_DEFINES.get(target_arch, [])

    # Default from detected kernel arch
    if KERNEL_ARCH == "arm64":
        return _ARCH_DEFINES.get("aarch64", [])
    return _ARCH_DEFINES.get("x86_64", [])

def _collect_includes(deps):
    """Collect include directories from cc_library dependencies."""
    dirs = []
    for dep in deps:
        if CcInfo in dep:
            cc_info = dep[CcInfo]
            for inc in cc_info.compilation_context.includes.to_list():
                dirs.append(inc)
            for inc in cc_info.compilation_context.system_includes.to_list():
                dirs.append(inc)
            for inc in cc_info.compilation_context.quote_includes.to_list():
                dirs.append(inc)
    return dirs

def _collect_headers(deps):
    """Collect header files from cc_library dependencies."""
    hdrs = []
    for dep in deps:
        if CcInfo in dep:
            cc_info = dep[CcInfo]
            hdrs.append(cc_info.compilation_context.headers)
    return depset(transitive = hdrs)

def _ebpf_prog_impl(ctx):
    tc = ctx.toolchains[_TOOLCHAIN_TYPE].llvm_bpf
    if not tc.valid:
        fail("LLVM BPF toolchain is not available")

    src = ctx.file.src
    include_dirs = _collect_includes(ctx.attr.deps)
    header_files = _collect_headers(ctx.attr.deps)

    # Build flags
    flags = list(_COMMON_FLAGS)

    if ctx.attr.core:
        flags.extend(_CORE_FLAGS)
    else:
        flags.extend(_PREBUILT_FLAGS)
        flags.extend(["-include", "pkg/ebpf/c/asm_goto_workaround.h"])

    # Architecture defines
    flags.extend(_get_arch_flags(ctx.attr.target_arch))

    if ctx.attr.debug:
        flags.append("-DDEBUG=1")

    flags.extend(ctx.attr.extra_flags)

    # --- Step 1: .c -> .bc (clang) ---
    bc_file = ctx.actions.declare_file(ctx.label.name + ".bc")

    clang_args = ctx.actions.args()
    if ctx.attr.core:
        clang_args.add("-target", "bpf")
    clang_args.add("-emit-llvm")
    clang_args.add_all(flags)

    # Include directories from deps
    for d in include_dirs:
        clang_args.add("-I", d)

    # Kernel headers for prebuilt programs
    kernel_header_inputs = []
    if not ctx.attr.core and KERNEL_HEADER_DIRS:
        kernel_header_files = ctx.files._linux_headers
        kernel_header_inputs = kernel_header_files

        # Resolve the external repo root from a file path.
        # Files are at external/<repo>/kernel_N/include/..., we need
        # the prefix up to (not including) "kernel_".
        if kernel_header_files:
            sample = kernel_header_files[0].path
            idx = sample.find("/kernel_")
            repo_root = sample[:idx] if idx >= 0 else sample.rsplit("/", 1)[0]
            for d in KERNEL_HEADER_DIRS:
                clang_args.add("-isystem", repo_root + "/" + d)

    clang_args.add("-c", src)
    clang_args.add("-o", bc_file)

    ctx.actions.run(
        inputs = depset(
            [src] + kernel_header_inputs,
            transitive = [header_files],
        ),
        outputs = [bc_file],
        executable = tc.clang_bpf,
        arguments = [clang_args],
        mnemonic = "EbpfClang",
        resource_set = resource_set_for(cpu_cores = 1, mem_mb = 1024),
        progress_message = "Compiling eBPF %{label} (.c -> .bc)",
    )

    # --- Step 2: .bc -> .o (llc) ---
    obj_file = ctx.actions.declare_file(ctx.label.name + ".o")

    llc_args = ctx.actions.args()
    llc_args.add("-march=bpf")
    llc_args.add("-filetype=obj")
    llc_args.add("-o", obj_file)
    llc_args.add(bc_file)

    ctx.actions.run(
        inputs = [bc_file],
        outputs = [obj_file],
        executable = tc.llc_bpf,
        arguments = [llc_args],
        mnemonic = "EbpfLlc",
        resource_set = resource_set_for(cpu_cores = 1, mem_mb = 1024),
        progress_message = "Linking eBPF %{label} (.bc -> .o)",
    )

    return [DefaultInfo(files = depset([obj_file]))]

_ebpf_prog = rule(
    implementation = _ebpf_prog_impl,
    attrs = {
        "src": attr.label(
            mandatory = True,
            allow_single_file = [".c"],
            doc = "The eBPF C source file.",
        ),
        "deps": attr.label_list(
            providers = [CcInfo],
            doc = "cc_library targets providing headers.",
        ),
        "core": attr.bool(
            default = False,
            doc = "CO-RE mode (no kernel headers, adds -DCOMPILE_CORE -g).",
        ),
        "debug": attr.bool(
            default = False,
            doc = "Include DEBUG=1 define.",
        ),
        "extra_flags": attr.string_list(
            doc = "Additional compiler flags.",
        ),
        "target_arch": attr.string(
            doc = "Target architecture: x86_64 or aarch64. Defaults to x86_64.",
        ),
        "_linux_headers": attr.label(
            default = "@linux_headers//:all",
            allow_files = True,
        ),
    },
    toolchains = [_TOOLCHAIN_TYPE],
)

def _ebpf_prog_macro_impl(name, visibility, src, deps, core, debug, extra_flags, target_arch):
    _ebpf_prog(
        name = name,
        visibility = visibility,
        src = src,
        deps = deps,
        core = core,
        debug = debug,
        extra_flags = extra_flags,
        target_arch = target_arch,
        target_compatible_with = ["@platforms//os:linux"],
    )

ebpf_prog = macro(
    doc = "Compile a single eBPF program (.c -> .o), Linux-only.",
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".c"], configurable = False),
        "deps": attr.label_list(default = [], configurable = False),
        "core": attr.bool(default = False, configurable = False),
        "debug": attr.bool(default = False, configurable = False),
        "extra_flags": attr.string_list(default = [], configurable = False),
        "target_arch": attr.string(default = "", configurable = False),
    },
    implementation = _ebpf_prog_macro_impl,
)

def _ebpf_program_suite_impl(name, visibility, src, deps, core, extra_flags, target_arch):
    _ebpf_prog(
        name = name,
        visibility = visibility,
        src = src,
        deps = deps,
        core = core,
        debug = False,
        extra_flags = extra_flags,
        target_arch = target_arch,
        target_compatible_with = ["@platforms//os:linux"],
    )
    _ebpf_prog(
        name = name + "-debug",
        visibility = visibility,
        src = src,
        deps = deps,
        core = core,
        debug = True,
        extra_flags = extra_flags,
        target_arch = target_arch,
        target_compatible_with = ["@platforms//os:linux"],
    )

ebpf_program_suite = macro(
    doc = """Create both normal and debug variants of an eBPF program.

    Generates:
      - {name}: normal build
      - {name}-debug: build with DEBUG=1
    """,
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".c"], configurable = False),
        "deps": attr.label_list(default = [], configurable = False),
        "core": attr.bool(default = False, configurable = False),
        "extra_flags": attr.string_list(default = [], configurable = False),
        "target_arch": attr.string(default = "", configurable = False),
    },
    implementation = _ebpf_program_suite_impl,
)

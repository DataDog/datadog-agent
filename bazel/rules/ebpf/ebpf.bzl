"""Bazel rules for compiling eBPF programs (.c -> .bc -> .o)."""

load("@bazel_lib//lib:resource_sets.bzl", "resource_set_for")
load("@linux_headers_aarch64//:defs.bzl", _KHD_AARCH64 = "KERNEL_HEADER_DIRS")
load("@linux_headers_x86_64//:defs.bzl", _KHD_X86_64 = "KERNEL_HEADER_DIRS")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("//bazel/rules/ebpf:cc_helpers.bzl", "collect_headers", "collect_include_dirs")

_TOOLCHAIN_TYPE = "@@//bazel/toolchains/llvm_bpf:llvm_bpf_toolchain_type"
_TOOLCHAIN = config_common.toolchain_type(_TOOLCHAIN_TYPE, mandatory = False)

# Bazel 9.3 leaves --incompatible_auto_exec_groups off, so the toolchain type
# is not itself an exec group. cfg = "exec" would follow the default platform.
_LLVM_BPF_EXEC_GROUP = "llvm_bpf"

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
    "-Wno-microsoft-anon-tag",
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
    "-fms-extensions",
]

_PREBUILT_FLAGS = [
    "-DCONFIG_64BIT",
    "-DCOMPILE_PREBUILT",
    "-fdebug-compilation-dir=.",
]

_CORE_FLAGS = [
    "-DCOMPILE_CORE",
    "-g",
    "-fdebug-compilation-dir=.",
]

def _target_arch(ctx):
    """Returns the CPU architecture of the target platform."""
    if ctx.target_platform_has_constraint(ctx.attr._aarch64_constraint[platform_common.ConstraintValueInfo]):
        return "aarch64"
    if ctx.target_platform_has_constraint(ctx.attr._x86_64_constraint[platform_common.ConstraintValueInfo]):
        return "x86_64"
    fail("eBPF programs can only be built for x86_64 or aarch64 target platforms")

def _compatible_with():
    return ["@platforms//os:linux"] + select({
        "@platforms//cpu:aarch64": [],
        "@platforms//cpu:x86_64": [],
        "//conditions:default": ["@platforms//:incompatible"],
    })

def _ebpf_prog_impl(ctx):
    toolchain = ctx.toolchains[_TOOLCHAIN_TYPE]
    if not toolchain:
        fail("LLVM BPF toolchain is not available")
    clang_bpf = toolchain.llvm_bpf.clang_bpf

    src = ctx.file.src
    inc = collect_include_dirs(ctx.attr.deps)
    header_files = collect_headers(ctx.attr.deps)

    target_arch = _target_arch(ctx)

    # Build flags
    flags = list(_COMMON_FLAGS)

    if ctx.attr.core:
        flags.extend(_CORE_FLAGS)
    else:
        flags.extend(_PREBUILT_FLAGS)
        flags.extend(["-include", "pkg/ebpf/c/asm_goto_workaround.h"])

    # Architecture defines
    flags.extend(_ARCH_DEFINES[target_arch])

    if ctx.attr.debug:
        flags.append("-DDEBUG=1")

    flags.extend(ctx.attr.extra_flags)

    # --- Step 1: .c -> .bc (clang) ---
    bc_file = ctx.actions.declare_file(ctx.label.name + ".bc")
    dep_file = ctx.actions.declare_file(ctx.label.name + ".d")
    unused_inputs_file = ctx.actions.declare_file(ctx.label.name + ".unused_inputs")

    clang_args = ctx.actions.args()
    if ctx.attr.core:
        clang_args.add("-target", "bpf")
    else:
        # The clang frontend needs the target arch to correctly parse
        # arch-specific inline ASM in kernel headers.
        clang_args.add("-target", target_arch)
    clang_args.add("-emit-llvm")
    clang_args.add_all(flags)

    for d in inc.includes:
        clang_args.add("-I", d)
    for d in inc.system_includes:
        clang_args.add("-isystem", d)
    for d in inc.quote_includes:
        clang_args.add("-iquote", d)

    if target_arch == "aarch64":
        kernel_header_dirs = _KHD_AARCH64
        kernel_header_files = ctx.files._linux_headers_aarch64
    else:
        kernel_header_dirs = _KHD_X86_64
        kernel_header_files = ctx.files._linux_headers_x86_64

    # Kernel headers for prebuilt programs
    kernel_header_inputs = []
    if not ctx.attr.core and kernel_header_dirs:
        kernel_header_inputs = kernel_header_files

        # Resolve the external repository root from the installed package layout.
        if kernel_header_files:
            sample = kernel_header_files[0].path
            idx = sample.find("/usr/src/")
            repo_root = sample[:idx] if idx >= 0 else sample.rsplit("/", 1)[0]
            for d in kernel_header_dirs:
                clang_args.add("-isystem", repo_root + "/" + d)

    clang_args.add("-MD")
    clang_args.add("-MF", dep_file)
    clang_args.add("-c", src)
    clang_args.add("-o", bc_file)

    action_inputs = depset(
        [src] + kernel_header_inputs,
        transitive = [header_files],
    )

    wrapper_args = ctx.actions.args()
    wrapper_args.add("--compiler", clang_bpf)
    wrapper_args.add("--depfile", dep_file)
    wrapper_args.add("--unused-inputs-list", unused_inputs_file)
    wrapper_args.add_all(action_inputs, before_each = "--declared-input")
    wrapper_args.add("--")
    wrapper_args.use_param_file("@%s", use_always = True)
    wrapper_args.set_param_file_format("multiline")

    ctx.actions.run(
        inputs = action_inputs,
        outputs = [bc_file, dep_file, unused_inputs_file],
        executable = ctx.executable._clang_with_unused_inputs,
        tools = [clang_bpf],
        arguments = [wrapper_args, clang_args],
        mnemonic = "EbpfClang",
        resource_set = resource_set_for(cpu_cores = 1, mem_mb = 1024),
        progress_message = "Compiling eBPF %{label} (.c -> .bc)",
        exec_group = _LLVM_BPF_EXEC_GROUP,
        toolchain = _TOOLCHAIN_TYPE,
        unused_inputs_list = unused_inputs_file,
    )

    # --- Step 2: .bc -> .o (reproduces `llc -march=bpf -filetype=obj` output verbatim) ---
    obj_file = ctx.actions.declare_file(ctx.label.name + ".o")

    llc_args = ctx.actions.args()
    llc_args.add_all(["-O2", "-Wno-override-module", "-Xclang", "-disable-llvm-optzns", "-c", "-fno-addrsig"])
    llc_args.add("-o", obj_file)
    llc_args.add_all(["-target", "bpf", "-x", "ir"])
    llc_args.add(bc_file)

    ctx.actions.run(
        inputs = [bc_file],
        outputs = [obj_file],
        executable = clang_bpf,
        arguments = [llc_args],
        mnemonic = "EbpfLlc",
        resource_set = resource_set_for(cpu_cores = 1, mem_mb = 1024),
        progress_message = "Linking eBPF %{label} (.bc -> .o)",
        toolchain = _TOOLCHAIN_TYPE,
    )

    return [DefaultInfo(files = depset([obj_file]))]

def _stripped_ebpf_impl(ctx):
    """Strip debug info, LBB symbols and .BTF.ext relocations from an eBPF object file."""
    toolchain = ctx.toolchains[_TOOLCHAIN_TYPE]
    if not toolchain:
        fail("LLVM BPF toolchain is not available")

    src = ctx.file.src

    # Original basename so the object can be shipped without renaming.
    out = ctx.actions.declare_file("stripped/" + src.basename)

    ctx.actions.run(
        inputs = [src],
        outputs = [out],
        executable = toolchain.llvm_bpf.llvm_strip,
        arguments = ["-g", "-w", "-N", "LBB*", "--remove-section=.rel.BTF.ext", "-o", out.path, src.path],
        mnemonic = "EbpfStrip",
        progress_message = "Stripping eBPF %{label}",
        toolchain = _TOOLCHAIN_TYPE,
    )

    return [DefaultInfo(files = depset([out]))]

_stripped_ebpf = rule(
    implementation = _stripped_ebpf_impl,
    attrs = {
        "src": attr.label(
            mandatory = True,
            allow_single_file = [".o"],
            doc = "The unstripped eBPF object file to strip.",
        ),
    },
    toolchains = [_TOOLCHAIN],
)

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
            # Headers only: build them for the host, as no C++ toolchain
            # cross-builds to every target platform an eBPF object can have.
            cfg = config.exec(exec_group = _LLVM_BPF_EXEC_GROUP),
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
        "_clang_with_unused_inputs": attr.label(
            default = "//bazel/rules/ebpf:clang_with_unused_inputs",
            executable = True,
            cfg = config.exec(exec_group = _LLVM_BPF_EXEC_GROUP),
            doc = "Clang wrapper that reports unused declared inputs.",
        ),
        "_aarch64_constraint": attr.label(default = "@platforms//cpu:aarch64"),
        "_x86_64_constraint": attr.label(default = "@platforms//cpu:x86_64"),
        "_linux_headers_x86_64": attr.label(
            default = "@linux_headers_x86_64//:all",
            allow_files = True,
        ),
        "_linux_headers_aarch64": attr.label(
            default = "@linux_headers_aarch64//:all",
            allow_files = True,
        ),
    },
    exec_groups = {
        _LLVM_BPF_EXEC_GROUP: exec_group(
            toolchains = [_TOOLCHAIN],
        ),
    },
    toolchains = [_TOOLCHAIN],
)

def _ebpf_prog_macro_impl(name, visibility, src, deps, core, debug, extra_flags):
    _ebpf_prog(
        name = name,
        visibility = visibility,
        src = src,
        deps = deps,
        core = core,
        debug = debug,
        extra_flags = extra_flags,
        target_compatible_with = _compatible_with(),
    )
    _stripped_ebpf(
        name = name + ".stripped",
        visibility = visibility,
        src = ":" + name,
        target_compatible_with = _compatible_with(),
    )

ebpf_prog = macro(
    doc = "Compile a single eBPF program (.c -> .o), Linux-only.",
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".c"], configurable = False),
        "deps": attr.label_list(default = [], configurable = False),
        "core": attr.bool(default = False, configurable = False),
        "debug": attr.bool(default = False, configurable = False),
        "extra_flags": attr.string_list(default = [], configurable = False),
    },
    implementation = _ebpf_prog_macro_impl,
)

def _ebpf_program_suite_impl(name, visibility, src, deps, core, extra_flags):
    _ebpf_prog(
        name = name,
        visibility = visibility,
        src = src,
        deps = deps,
        core = core,
        debug = False,
        extra_flags = extra_flags,
        target_compatible_with = _compatible_with(),
    )
    _stripped_ebpf(
        name = name + ".stripped",
        visibility = visibility,
        src = ":" + name,
        target_compatible_with = _compatible_with(),
    )
    _ebpf_prog(
        name = name + "-debug",
        visibility = visibility,
        src = src,
        deps = deps,
        core = core,
        debug = True,
        extra_flags = extra_flags,
        target_compatible_with = _compatible_with(),
    )
    _stripped_ebpf(
        name = name + "-debug.stripped",
        visibility = visibility,
        src = ":" + name + "-debug",
        target_compatible_with = _compatible_with(),
    )

ebpf_program_suite = macro(
    doc = """Create both normal and debug variants of an eBPF program.

    Generates:
      - {name}: normal build (unstripped)
      - {name}.stripped: stripped variant (debug info + LBB symbols removed)
      - {name}-debug: build with DEBUG=1 (unstripped)
      - {name}-debug.stripped: stripped debug variant
    """,
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".c"], configurable = False),
        "deps": attr.label_list(default = [], configurable = False),
        "core": attr.bool(default = False, configurable = False),
        "extra_flags": attr.string_list(default = [], configurable = False),
    },
    implementation = _ebpf_program_suite_impl,
)

"""Bazel rules for compiling eBPF programs."""

def _ebpf_program_impl(ctx):
    """Implementation of ebpf_program rule for non-CO-RE eBPF compilation."""

    # Get LLVM toolchain
    llvm_toolchain = ctx.toolchains["//bazel/toolchains/llvm:llvm_toolchain_type"]

    if not llvm_toolchain.clang_bpf or not llvm_toolchain.llc_bpf:
        fail("LLVM toolchain not properly configured")

    # Get the executable files from the toolchain
    clang = llvm_toolchain.clang_bpf
    llc = llvm_toolchain.llc_bpf
    llvm_strip = llvm_toolchain.llvm_strip

    # Intermediate LLVM IR file
    bc_file = ctx.actions.declare_file(ctx.label.name + ".bc")

    # Final eBPF object file
    out_file = ctx.outputs.out

    # Build clang command for .c → .bc
    clang_args = ctx.actions.args()
    clang_args.add("-MD")
    clang_args.add("-MF", bc_file.path + ".d")

    # Add target architecture
    if ctx.attr.co_re:
        clang_args.add("-target", "bpf")
    else:
        # For non-CO-RE, emit LLVM IR
        if ctx.attr.target_arch:
            clang_args.add("-target", ctx.attr.target_arch)
        clang_args.add("-emit-llvm")

    # Add include paths
    for include in ctx.attr.includes:
        clang_args.add("-I" + include)

    # Add compiler flags
    for flag in ctx.attr.copts:
        clang_args.add(flag)

    # Add source file
    clang_args.add("-c", ctx.file.src.path)
    clang_args.add("-o", bc_file.path)

    # Compile to LLVM IR
    ctx.actions.run(
        inputs = [ctx.file.src] + ctx.files.hdrs,
        outputs = [bc_file],
        executable = clang,
        arguments = [clang_args],
        mnemonic = "EbpfClang",
        progress_message = "Compiling eBPF program %s" % ctx.file.src.short_path,
    )

    # Build llc command for .bc → .o
    llc_args = ctx.actions.args()
    llc_args.add("-march=bpf")
    llc_args.add("-filetype=obj")
    llc_args.add("-o", out_file.path)
    llc_args.add(bc_file.path)

    outputs_to_generate = [out_file]

    # Compile LLVM IR to BPF object
    ctx.actions.run(
        inputs = [bc_file],
        outputs = outputs_to_generate,
        executable = llc,
        arguments = [llc_args],
        mnemonic = "EbpfLlc",
        progress_message = "Generating BPF object %s" % out_file.short_path,
    )

    # Optionally strip symbols
    if ctx.attr.strip and llvm_strip:
        strip_args = ctx.actions.args()
        strip_args.add("-g")
        strip_args.add(out_file.path)

        ctx.actions.run(
            inputs = [out_file],
            outputs = [],  # in-place modification
            executable = llvm_strip,
            arguments = [strip_args],
            mnemonic = "EbpfStrip",
            progress_message = "Stripping %s" % out_file.short_path,
        )

    return [DefaultInfo(files = depset([out_file]))]

def _ebpf_co_re_program_impl(ctx):
    """Implementation of ebpf_co_re_program rule for CO-RE eBPF compilation."""

    # Get LLVM toolchain
    llvm_toolchain = ctx.toolchains["//bazel/toolchains/llvm:llvm_toolchain_type"]

    if not llvm_toolchain.clang_bpf:
        fail("LLVM toolchain not properly configured")

    clang = llvm_toolchain.clang_bpf
    llvm_strip = llvm_toolchain.llvm_strip

    # Final eBPF object file
    out_file = ctx.outputs.out

    # Build clang command for .c → .o (CO-RE direct compilation)
    clang_args = ctx.actions.args()
    clang_args.add("-MD")
    clang_args.add("-MF", out_file.path + ".d")
    clang_args.add("-target", "bpf")

    # Add include paths
    for include in ctx.attr.includes:
        clang_args.add("-I" + include)

    # Add compiler flags
    for flag in ctx.attr.copts:
        clang_args.add(flag)

    # Add source file
    clang_args.add("-c", ctx.file.src.path)
    clang_args.add("-o", out_file.path)

    # Compile directly to BPF object (CO-RE)
    ctx.actions.run(
        inputs = [ctx.file.src] + ctx.files.hdrs,
        outputs = [out_file],
        executable = clang,
        arguments = [clang_args],
        mnemonic = "EbpfCoReClang",
        progress_message = "Compiling CO-RE eBPF program %s" % ctx.file.src.short_path,
    )

    # Optionally strip symbols
    if ctx.attr.strip and llvm_strip:
        strip_args = ctx.actions.args()
        strip_args.add("-g")
        strip_args.add(out_file.path)

        ctx.actions.run(
            inputs = [out_file],
            outputs = [],  # in-place modification
            executable = llvm_strip,
            arguments = [strip_args],
            mnemonic = "EbpfStrip",
            progress_message = "Stripping %s" % out_file.short_path,
        )

    return [DefaultInfo(files = depset([out_file]))]

ebpf_program = rule(
    implementation = _ebpf_program_impl,
    doc = "Compiles an eBPF program (non-CO-RE: .c → .bc → .o)",
    attrs = {
        "src": attr.label(
            doc = "Source .c file",
            allow_single_file = [".c"],
            mandatory = True,
        ),
        "hdrs": attr.label_list(
            doc = "Header files",
            allow_files = [".h"],
        ),
        "out": attr.output(
            doc = "Output .o file",
            mandatory = True,
        ),
        "includes": attr.string_list(
            doc = "Include directories",
            default = [],
        ),
        "copts": attr.string_list(
            doc = "Compiler options",
            default = [],
        ),
        "strip": attr.bool(
            doc = "Strip symbols from output",
            default = False,
        ),
        "co_re": attr.bool(
            doc = "Use CO-RE compilation",
            default = False,
        ),
        "target_arch": attr.string(
            doc = "Target architecture for cross-compilation",
            default = "",
        ),
    },
    toolchains = ["//bazel/toolchains/llvm:llvm_toolchain_type"],
)

ebpf_co_re_program = rule(
    implementation = _ebpf_co_re_program_impl,
    doc = "Compiles a CO-RE eBPF program (.c → .o direct)",
    attrs = {
        "src": attr.label(
            doc = "Source .c file",
            allow_single_file = [".c"],
            mandatory = True,
        ),
        "hdrs": attr.label_list(
            doc = "Header files",
            allow_files = [".h"],
        ),
        "out": attr.output(
            doc = "Output .o file",
            mandatory = True,
        ),
        "includes": attr.string_list(
            doc = "Include directories",
            default = [],
        ),
        "copts": attr.string_list(
            doc = "Compiler options",
            default = [],
        ),
        "strip": attr.bool(
            doc = "Strip symbols from output",
            default = False,
        ),
    },
    toolchains = ["//bazel/toolchains/llvm:llvm_toolchain_type"],
)

def ebpf_program_suite(name, src, hdrs = [], includes = [], copts = [], co_re = True, **kwargs):
    """
    Macro to generate both normal and debug versions of an eBPF program.

    Args:
        name: Base name for the targets
        src: Source .c file
        hdrs: Header dependencies
        includes: Include paths
        copts: Compiler flags
        co_re: Use CO-RE compilation
        **kwargs: Additional arguments passed to the rules
    """

    rule_func = ebpf_co_re_program if co_re else ebpf_program

    # Normal version
    rule_func(
        name = name,
        src = src,
        hdrs = hdrs,
        out = name + ".o",
        includes = includes,
        copts = copts,
        **kwargs
    )

    # Debug version
    debug_copts = copts + ["-DDEBUG=1"]
    rule_func(
        name = name + "-debug",
        src = src,
        hdrs = hdrs,
        out = name + "-debug.o",
        includes = includes,
        copts = debug_copts,
        **kwargs
    )

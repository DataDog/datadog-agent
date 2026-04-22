"""LLVM BPF toolchain rule and provider."""

LlvmBpfToolchainInfo = provider(
    doc = "LLVM/Clang toolchain for eBPF compilation.",
    fields = {
        "clang_bpf": "clang-bpf executable File, or None",
        "llc_bpf": "llc-bpf executable File, or None",
        "llvm_strip": "llvm-strip executable File, or None",
        "version": "LLVM version string",
        "valid": "Whether the toolchain is usable",
    },
)

def _llvm_bpf_toolchain_impl(ctx):
    clang = ctx.executable.clang_bpf if hasattr(ctx.executable, "clang_bpf") else None
    llc = ctx.executable.llc_bpf if hasattr(ctx.executable, "llc_bpf") else None
    strip = ctx.executable.llvm_strip if hasattr(ctx.executable, "llvm_strip") else None
    return [platform_common.ToolchainInfo(
        llvm_bpf = LlvmBpfToolchainInfo(
            clang_bpf = clang,
            llc_bpf = llc,
            llvm_strip = strip,
            version = ctx.attr.version,
            valid = clang != None,
        ),
    )]

llvm_bpf_toolchain = rule(
    implementation = _llvm_bpf_toolchain_impl,
    attrs = {
        "clang_bpf": attr.label(
            doc = "clang-bpf executable",
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "llc_bpf": attr.label(
            doc = "llc-bpf executable",
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "llvm_strip": attr.label(
            doc = "llvm-strip executable",
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "version": attr.string(default = "unknown"),
    },
)

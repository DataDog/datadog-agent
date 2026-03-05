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
    return [platform_common.ToolchainInfo(
        llvm_bpf = LlvmBpfToolchainInfo(
            clang_bpf = ctx.file.clang_bpf if ctx.file.clang_bpf else None,
            llc_bpf = ctx.file.llc_bpf if ctx.file.llc_bpf else None,
            llvm_strip = ctx.file.llvm_strip if ctx.file.llvm_strip else None,
            version = ctx.attr.version,
            valid = ctx.file.clang_bpf != None,
        ),
    )]

llvm_bpf_toolchain = rule(
    implementation = _llvm_bpf_toolchain_impl,
    attrs = {
        "clang_bpf": attr.label(
            doc = "clang-bpf executable",
            allow_single_file = True,
        ),
        "llc_bpf": attr.label(
            doc = "llc-bpf executable",
            allow_single_file = True,
        ),
        "llvm_strip": attr.label(
            doc = "llvm-strip executable",
            allow_single_file = True,
        ),
        "version": attr.string(default = "unknown"),
    },
)

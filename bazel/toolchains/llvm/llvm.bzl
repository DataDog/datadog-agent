"""LLVM toolchain for eBPF compilation."""

LlvmToolchainInfo = provider(
    doc = "Information about LLVM/Clang toolchain for eBPF compilation.",
    fields = {
        "clang_bpf": "clang-bpf executable file",
        "llc_bpf": "llc-bpf executable file",
        "llvm_strip": "llvm-strip executable file",
        "version": "LLVM version string",
    },
)

def _llvm_toolchain_impl(ctx):
    """Implementation of llvm_toolchain rule."""

    # Get executable files from labels
    clang_bpf_file = ctx.file.clang_bpf_path if ctx.file.clang_bpf_path else None
    llc_bpf_file = ctx.file.llc_bpf_path if ctx.file.llc_bpf_path else None
    llvm_strip_file = ctx.file.llvm_strip_path if ctx.file.llvm_strip_path else None

    return [
        platform_common.ToolchainInfo(
            clang_bpf = clang_bpf_file,
            llc_bpf = llc_bpf_file,
            llvm_strip = llvm_strip_file,
            version = ctx.attr.version,
        ),
    ]

llvm_toolchain = rule(
    implementation = _llvm_toolchain_impl,
    doc = "Defines an LLVM toolchain for eBPF compilation.",
    attrs = {
        "clang_bpf_path": attr.label(
            doc = "Path to clang-bpf executable",
            allow_single_file = True,
        ),
        "llc_bpf_path": attr.label(
            doc = "Path to llc-bpf executable",
            allow_single_file = True,
        ),
        "llvm_strip_path": attr.label(
            doc = "Path to llvm-strip executable",
            allow_single_file = True,
        ),
        "version": attr.string(
            doc = "LLVM version",
            default = "12.0.1",
        ),
    },
)

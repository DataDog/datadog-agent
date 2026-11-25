"""LLVM BPF toolchain rule and provider."""

LlvmBpfToolchainInfo = provider(
    doc = "LLVM/Clang toolchain for eBPF compilation.",
    fields = {
        "clang_bpf": "clang-bpf executable File",
        "llvm_strip": "llvm-strip executable File",
        "version": "LLVM version string",
    },
)

def _llvm_bpf_toolchain_impl(ctx):
    return [platform_common.ToolchainInfo(
        llvm_bpf = LlvmBpfToolchainInfo(
            clang_bpf = ctx.executable.clang_bpf,
            llvm_strip = ctx.executable.llvm_strip,
            version = ctx.attr.version,
        ),
    )]

llvm_bpf_toolchain = rule(
    implementation = _llvm_bpf_toolchain_impl,
    attrs = {
        "clang_bpf": attr.label(
            doc = "clang-bpf executable",
            mandatory = True,
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "llvm_strip": attr.label(
            doc = "llvm-strip executable",
            mandatory = True,
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "version": attr.string(default = "unknown"),
    },
)

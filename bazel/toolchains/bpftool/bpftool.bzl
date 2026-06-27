"""bpftool toolchain rule and provider for CO-RE BTF minimization."""

BpftoolToolchainInfo = provider(
    doc = "bpftool binary used to generate minimized CO-RE BTFs.",
    fields = {
        "bpftool": "bpftool executable File, or None",
        "version": "bpftool version string",
        "valid": "Whether the toolchain is usable",
    },
)

def _bpftool_toolchain_impl(ctx):
    tool = ctx.executable.bpftool if ctx.attr.bpftool else None
    return [platform_common.ToolchainInfo(
        bpftool = BpftoolToolchainInfo(
            bpftool = tool,
            version = ctx.attr.version,
            valid = tool != None,
        ),
    )]

bpftool_toolchain = rule(
    implementation = _bpftool_toolchain_impl,
    doc = "Collects a bpftool binary and exposes it as a ToolchainInfo. Creates no actions.",
    attrs = {
        "bpftool": attr.label(
            doc = "bpftool executable",
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "version": attr.string(default = "unknown"),
    },
)

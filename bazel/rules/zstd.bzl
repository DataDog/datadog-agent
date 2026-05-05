"""Zstd compression rule using the bazel-lib zstd toolchain."""

_TOOLCHAIN = "@bazel_lib//lib:zstd_toolchain_type"

def _impl(ctx):
    ctx.actions.run(
        # --force: Bazel sandbox exposes source files as symlinks, which `zstd` rejects by default
        # -o: there's alas no long option such as --output
        arguments = ["--force"] + ctx.attr.args + [ctx.file.src.path, "-o", ctx.outputs.out.path],
        executable = ctx.toolchains[_TOOLCHAIN].zstdinfo.binary,
        inputs = [ctx.file.src],
        mnemonic = "ZstdCompress",
        outputs = [ctx.outputs.out],
        toolchain = _TOOLCHAIN,
    )
    return [DefaultInfo(files = depset([ctx.outputs.out]))]

zstd_compress = rule(
    implementation = _impl,
    attrs = {
        "args": attr.string_list(),
        "out": attr.output(mandatory = True),
        "src": attr.label(allow_single_file = True, mandatory = True),
    },
    toolchains = [_TOOLCHAIN],
)

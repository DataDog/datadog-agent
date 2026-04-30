"""Zstd compression rule using the bazel-lib zstd toolchain."""

_TOOLCHAIN = "@bazel_lib//lib:zstd_toolchain_type"

def _impl(ctx):
    ctx.actions.run(
        arguments = [
            "--force",  # Bazel sandbox exposes source files as symlinks, which zstd rejects by default
            "--no-check",  # match DataDog/zstd Go library behavior: no XXH64 frame checksum
            "-5",  # match DataDog/zstd: DefaultCompression = 5
            ctx.file.src.path,
            "-o",
            ctx.outputs.out.path,
        ],
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
        "src": attr.label(allow_single_file = True, mandatory = True),
        "out": attr.output(mandatory = True),
    },
    toolchains = [_TOOLCHAIN],
)

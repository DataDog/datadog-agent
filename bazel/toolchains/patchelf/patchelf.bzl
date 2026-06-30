"""Toolchain to provide a hermetic patchelf binary."""

load("@@//bazel/toolchains:toolchain_info.bzl", "ToolInfo")

def _patchelf_toolchain_impl(ctx):
    toolchain_info = platform_common.ToolchainInfo(
        patchelf = ToolInfo(
            name = str(ctx.label),
            valid = True,
            label = ctx.attr.label,
            path = "",
            version = ctx.attr.version,
        ),
    )
    return [toolchain_info]

patchelf_toolchain = rule(
    implementation = _patchelf_toolchain_impl,
    attrs = {
        "label": attr.label(
            doc = "A valid label of a target to build or a prebuilt binary.",
            cfg = "exec",
            executable = True,
            allow_files = True,
            mandatory = True,
        ),
        "version": attr.string(
            doc = "The version string of the executable. This should be manually set.",
        ),
    },
)

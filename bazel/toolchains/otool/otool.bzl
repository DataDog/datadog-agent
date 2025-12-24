"""toolchain to provide an otool binary."""

load("@@//bazel/toolchains:toolchain_info.bzl", "ToolInfo")

def _otool_toolchain_impl(ctx):
    if ctx.attr.label and ctx.attr.path:
        fail("otool_toolchain must not specify both label and path.")
    valid = bool(ctx.attr.label) or bool(ctx.attr.path)
    toolchain_info = platform_common.ToolchainInfo(
        otool = ToolInfo(
            name = str(ctx.label),
            valid = valid,
            label = ctx.attr.label,
            path = ctx.attr.path,
            version = ctx.attr.version,
        ),
    )
    return [toolchain_info]

otool_toolchain = rule(
    implementation = _otool_toolchain_impl,
    attrs = {
        "label": attr.label(
            doc = "A valid label of a target to build or a prebuilt binary. Mutually exclusive with path.",
            cfg = "exec",
            executable = True,
            allow_files = True,
        ),
        "path": attr.string(
            doc = "The path to the executable. Mutually exclusive with label.",
        ),
        "version": attr.string(
            doc = "The version string of the executable. This should be manually set.",
        ),
    },
)

# Expose the presence of otool in the resolved toolchain as a flag.
def _is_otool_available_impl(ctx):
    toolchain = ctx.toolchains["@@//bazel/toolchains/otool:otool_toolchain_type"].otool
    return [config_common.FeatureFlagInfo(
        value = ("1" if toolchain.valid else "0"),
    )]

is_otool_available = rule(
    implementation = _is_otool_available_impl,
    attrs = {},
    toolchains = ["@@//bazel/toolchains/otool:otool_toolchain_type"],
)

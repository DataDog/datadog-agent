"""toolchain to provide the {TOOL_NAME} binary."""

load("@@//bazel/toolchains:toolchain_info.bzl", "ToolInfo")

def _{TOOL_NAME}_toolchain_impl(ctx):
    if ctx.attr.label and ctx.attr.path:
        fail("{TOOL_NAME}_toolchain must not specify both label and path.")
    valid = bool(ctx.attr.label) or bool(ctx.attr.path)
    toolchain_info = platform_common.ToolchainInfo(
        {TOOL_NAME} = ToolInfo(
            name = str(ctx.label),
            valid = valid,
            label = ctx.attr.label,
            path = ctx.attr.path,
            version = ctx.attr.version,
        ),
    )
    return [toolchain_info]

{TOOL_NAME}_toolchain = rule(
    implementation = _{TOOL_NAME}_toolchain_impl,
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

# Expose the presence of {TOOL_NAME} in the resolved toolchain as a flag.
# The signal is to look at the default toolchain of the {TOOL_NAME}_toolchan_type
# and see that it is valid (that is, it has a path).
def _is_{TOOL_NAME}_available_impl(ctx):
    toolchain = ctx.toolchains["@{TOOL_NAME}//:{TOOL_NAME}_toolchain_type"].{TOOL_NAME}
    return [config_common.FeatureFlagInfo(
        value = ("1" if toolchain.valid else "0"),
    )]

is_{TOOL_NAME}_available = rule(
    implementation = _is_{TOOL_NAME}_available_impl,
    attrs = {},
    toolchains = ["@{TOOL_NAME}//:{TOOL_NAME}_toolchain_type"],
)

"""Adapts the host runtime to the DotSlash toolchain interface."""

def _host_dotslash_toolchain_impl(ctx):
    if ctx.attr.error:
        fail(ctx.attr.error)
    return [platform_common.ToolchainInfo(
        executable = ctx.attr.dotslash,
        inputs = depset([ctx.file.discovery]),
        tools = [],
        env = ctx.attr.environment,
        execution_requirements = {"local": "1"},
        use_default_shell_env = True,
    )]

host_dotslash_toolchain = rule(
    implementation = _host_dotslash_toolchain_impl,
    attrs = {
        "dotslash": attr.string(mandatory = True),
        "environment": attr.string_dict(mandatory = True),
        "error": attr.string(),
        "discovery": attr.label(mandatory = True, allow_single_file = True),
    },
)

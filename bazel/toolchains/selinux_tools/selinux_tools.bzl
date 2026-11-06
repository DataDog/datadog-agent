"""Toolchain providing checkmodule and semodule_package built from source."""

def _selinux_tools_toolchain_impl(ctx):
    return [platform_common.ToolchainInfo(
        checkmodule = ctx.attr.checkmodule[DefaultInfo].files_to_run,
        semodule_package = ctx.attr.semodule_package[DefaultInfo].files_to_run,
    )]

selinux_tools_toolchain = rule(
    implementation = _selinux_tools_toolchain_impl,
    attrs = {
        "checkmodule": attr.label(
            cfg = "exec",
            executable = True,
            mandatory = True,
        ),
        "semodule_package": attr.label(
            cfg = "exec",
            executable = True,
            mandatory = True,
        ),
    },
)

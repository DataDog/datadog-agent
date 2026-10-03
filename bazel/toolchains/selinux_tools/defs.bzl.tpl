"""toolchain to provide the checkmodule and semodule_package binaries."""

load("@@//bazel/toolchains:toolchain_info.bzl", "ToolInfo")

def _tool_info(name, path):
    return ToolInfo(
        name = name,
        valid = bool(path),
        label = None,
        path = path,
        version = "<unknown>",
    )

def _selinux_tools_toolchain_impl(ctx):
    toolchain_info = platform_common.ToolchainInfo(
        checkmodule = _tool_info("checkmodule", ctx.attr.checkmodule_path),
        semodule_package = _tool_info("semodule_package", ctx.attr.semodule_package_path),
    )
    return [toolchain_info]

selinux_tools_toolchain = rule(
    implementation = _selinux_tools_toolchain_impl,
    attrs = {
        "checkmodule_path": attr.string(doc = "The path to the checkmodule executable."),
        "semodule_package_path": attr.string(doc = "The path to the semodule_package executable."),
    },
)

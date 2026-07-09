def _rpath_rewriter_toolchain_impl(ctx):
    return [
        platform_common.ToolchainInfo(
            patch_file = ctx.attr.patch_file[DefaultInfo].files_to_run,
        ),
    ]

rpath_rewriter_toolchain = rule(
    implementation = _rpath_rewriter_toolchain_impl,
    attrs = {
        "patch_file": attr.label(
            doc = """An executable accepting <input> <new-rpath> <output> arguments, able to rewrite rpaths for individual targets.""",
            cfg = "exec",
            executable = True,
            allow_files = True,
            mandatory = True,
        ),
    },
)

def _noop_toolchain_impl(_ctx):
    return [
        platform_common.ToolchainInfo(
            patch_file = None,
        ),
    ]

noop_toolchain = rule(
    implementation = _noop_toolchain_impl,
    doc = """A no-op rpath rewriter toolchain.

    A None patch_file signals that rpath rewriting is not applicable for the target platform.""",
)

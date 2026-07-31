def _rpath_rewriter_toolchain_impl(ctx):
    return [
        platform_common.ToolchainInfo(
            rewriter_tool = ctx.attr.rewriter_tool[DefaultInfo].files_to_run,
            tree_rewriter_tool = ctx.attr.tree_rewriter_tool[DefaultInfo].files_to_run,
        ),
    ]

rpath_rewriter_toolchain = rule(
    implementation = _rpath_rewriter_toolchain_impl,
    attrs = {
        "rewriter_tool": attr.label(
            doc = """An executable accepting <input> <new-rpath> <output> arguments, able to rewrite rpaths for individual targets.""",
            cfg = "exec",
            executable = True,
            allow_files = True,
            mandatory = True,
        ),
        "tree_rewriter_tool": attr.label(
            doc = """An executable accepting <input> <new-rpath> <output> arguments, able to rewrite rpaths for tree artifact (directory) targets.""",
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
            rewriter_tool = None,
            tree_rewriter_tool = None,
        ),
    ]

noop_toolchain = rule(
    implementation = _noop_toolchain_impl,
    doc = """A no-op rpath rewriter toolchain.

    A None rewriter_tool signals that rpath rewriting is not applicable for the target platform.""",
)

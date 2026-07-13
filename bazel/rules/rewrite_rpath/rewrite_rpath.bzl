"""Set a binary's rpath to the provided value."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

def rewrite_rpaths_for_files(ctx, inputs, rpath):
    """Creates actions to apply an rpath rewriter to the inputs.

    Args:
      ctx: the rule context.
      inputs: the files to patch.
      rpath: the rpath to set.

    Returns:
      A list of the generated outputs
    """
    toolchain = ctx.toolchains["//bazel/toolchains/rpath_rewriter"]

    # No-op: just pass the inputs through.
    if toolchain.rewriter_tool == None:
        return inputs

    outputs = []
    for input in inputs:
        output = ctx.actions.declare_file("patched/" + input.basename)
        args = ctx.actions.args()
        args.add(input)
        args.add(rpath)
        args.add(output)
        ctx.actions.run(
            inputs = [input],
            outputs = [output],
            arguments = [args],
            executable = toolchain.rewriter_tool,
            toolchain = "//bazel/toolchains/rpath_rewriter",
        )
        outputs.append(output)

    return outputs

def rewrite_rpaths_for_trees(ctx, inputs, rpath):
    """Creates actions to apply an rpath rewriter to TreeArtifact (directory) inputs.
    """
    toolchain = ctx.toolchains["//bazel/toolchains/rpath_rewriter"]

    # No-op: just pass the inputs through.
    if toolchain.tree_rewriter_tool == None:
        return inputs

    outputs = []
    for input in inputs:
        output = ctx.actions.declare_directory("patched_dirs/" + input.basename)
        args = ctx.actions.args()
        args.add(input.path)
        args.add(rpath)
        args.add(output.path)
        ctx.actions.run(
            inputs = [input],
            outputs = [output],
            arguments = [args],
            executable = toolchain.tree_rewriter_tool,
            toolchain = "//bazel/toolchains/rpath_rewriter",
        )
        outputs.append(output)

    return outputs

def _rewrite_rpath_impl(ctx):
    rpath = ctx.attr.rpath.format(install_dir = ctx.attr._install_dir[BuildSettingInfo].value)
    return DefaultInfo(files = depset(rewrite_rpaths_for_files(ctx, inputs = ctx.files.inputs, rpath = rpath)))

rewrite_rpath = rule(
    implementation = _rewrite_rpath_impl,
    doc = """Set a binary's rpath to the provided value.

    If no rpath is provided, this defaults to <@@//:install_dir>/embedded/lib.""",
    attrs = {
        "inputs": attr.label_list(
            doc = "The binaries to patch",
            mandatory = True,
        ),
        "rpath": attr.string(
            doc = """The new rpath. Defaults to <@@//:install_dir>/embedded/lib.
            Supports '{install_dir}' variable.""",
            default = "{install_dir}/embedded/lib",
        ),
        "_install_dir": attr.label(
            doc = "Private label used for the default rpath",
            default = "@@//:install_dir",
        ),
    },
    toolchains = ["//bazel/toolchains/rpath_rewriter"],
)

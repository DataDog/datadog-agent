"""Set a binary's rpath to the provided value."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

_RPATH_REWRITER_TOOLCHAIN = "//bazel/toolchains/rpath_rewriter"

def rewrite_rpaths(ctx, inputs, rpath):
    """Creates actions to apply an rpath rewriter to files and TreeArtifacts.

    The selected rpath rewriter toolchain provides separate tools for regular
    files and TreeArtifacts. A `None` tool means rpath rewriting is not
    applicable for that artifact kind on the target platform; in that case, the
    original input is returned unchanged and no copy action is registered.

    Args:
      ctx: the rule context.
      inputs: the files or TreeArtifacts to patch.
      rpath: the rpath to set.

    Returns:
      A list of rewritten outputs, in the same order as inputs. Rewritten
      artifacts preserve the input basename. On platforms where rewriting is a
      no-op, some or all entries may be the original inputs.
    """
    toolchain = ctx.toolchains[_RPATH_REWRITER_TOOLCHAIN]

    outputs = []
    for input in inputs:
        tool = toolchain.tree_rewriter_tool if input.is_directory else toolchain.rewriter_tool

        # No-op: just pass this input through.
        if tool == None:
            outputs.append(input)
            continue

        if input.is_directory:
            output = ctx.actions.declare_directory("patched_dirs/" + input.basename)
        else:
            output = ctx.actions.declare_file("patched/" + input.basename)

        args = ctx.actions.args()
        args.add(input.path)
        args.add(rpath)
        args.add(output.path)
        ctx.actions.run(
            inputs = [input],
            outputs = [output],
            arguments = [args],
            executable = tool,
            toolchain = _RPATH_REWRITER_TOOLCHAIN,
        )
        outputs.append(output)

    return outputs

def _rewrite_rpath_impl(ctx):
    rpath = ctx.attr.rpath.format(install_dir = ctx.attr._install_dir[BuildSettingInfo].value)
    return DefaultInfo(files = depset(rewrite_rpaths(ctx, inputs = ctx.files.inputs, rpath = rpath)))

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
    toolchains = [_RPATH_REWRITER_TOOLCHAIN],
)

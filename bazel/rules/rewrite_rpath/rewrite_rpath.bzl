"""Set a binary's rpath to the provided value."""

load("@bazel_lib//lib:paths.bzl", "relative_file")
load("@bazel_skylib//lib:paths.bzl", "paths")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

_RPATH_REWRITER_TOOLCHAIN = "//bazel/toolchains/rpath_rewriter"

def _relative_rpath(destination, rpath):
    """Returns the rpath relative to destination's containing directory."""
    destination_dir = paths.dirname(destination)

    if not (paths.is_absolute(rpath) and paths.is_absolute(destination_dir)):
        fail("Cannot compute relative rpath between '{}' and '{}', they both must be absolute paths".format(destination_dir, rpath))

    # bazel_lib's relative_file assumes files; append a dummy filename to both
    # directories, then remove it from the computed file-relative path.
    relative = paths.dirname(relative_file(paths.join(rpath, "_"), paths.join(destination_dir, "_")))
    return "./" + paths.normalize(relative)

def rewrite_rpaths(ctx, inputs, rpath, relative = False):
    """Creates actions to apply an rpath rewriter to files and TreeArtifacts.

    The selected rpath rewriter toolchain provides separate tools for regular
    files and TreeArtifacts. A `None` tool means rpath rewriting is not
    applicable for that artifact kind on the target platform; in that case, the
    original input is returned unchanged and no copy action is registered.

    Args:
      ctx: the rule context.
      inputs: list of structs with:
         - target: the file or TreeArtifact to patch.
         - destination: the path where the file/TreeArtifact will be shipped.
           This is used to calculate the relative path when `relative` is True.
           For TreeArtifacts, this is the destination directory of the tree root;
           the rewriter tool adjusts for each file's depth within the tree.
      rpath: the rpath to set. When `relative` is True, this must be expressed
        in the same path namespace as each input's destination.
      relative: whether the rpath must be set relative to the patched file.
        Relative rpaths are passed to toolchain scripts with a leading `./`;
        those scripts substitute the platform-specific origin token.

    Returns:
      A list of rewritten outputs, in the same order as inputs. Rewritten
      artifacts preserve the input basename. On platforms where rewriting is a
      no-op, some or all entries may be the original inputs.
    """
    toolchain = ctx.toolchains[_RPATH_REWRITER_TOOLCHAIN]

    outputs = []
    for input in inputs:
        target = input.target
        tool = toolchain.tree_rewriter_tool if target.is_directory else toolchain.rewriter_tool

        # No-op: just pass this input through.
        if tool == None:
            outputs.append(target)
            continue

        if target.is_directory:
            output = ctx.actions.declare_directory("patched_dirs/" + target.basename)
        else:
            output = ctx.actions.declare_file("patched/" + target.basename)

        resolved_rpath = _relative_rpath(input.destination, rpath) if relative else rpath

        args = ctx.actions.args()
        args.add(target.path)
        args.add(resolved_rpath)
        args.add(output.path)
        ctx.actions.run(
            inputs = [target],
            outputs = [output],
            arguments = [args],
            executable = tool,
            toolchain = _RPATH_REWRITER_TOOLCHAIN,
        )
        outputs.append(output)

    return outputs

def _rewrite_rpath_impl(ctx):
    install_dir = ctx.attr._install_dir[BuildSettingInfo].value
    rpath = ctx.attr.rpath.format(install_dir = install_dir)
    destination = ctx.attr.destination.format(install_dir = install_dir)
    inputs = [
        struct(
            target = input,
            destination = paths.join(destination, input.basename),
        )
        for input in ctx.files.inputs
    ]
    return DefaultInfo(files = depset(rewrite_rpaths(
        ctx,
        inputs = inputs,
        rpath = rpath,
        relative = ctx.attr._relative_rpaths[BuildSettingInfo].value,
    )))

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
        "destination": attr.string(
            doc = """Directory where the inputs will be installed.
            Supports '{install_dir}' variable and is used only when relative rpaths are enabled.""",
            default = "{install_dir}/embedded/lib",
        ),
        "_install_dir": attr.label(
            doc = "Private label used for the default rpath and destination",
            default = "@@//:install_dir",
        ),
        "_relative_rpaths": attr.label(
            doc = "Private label used to decide whether rpaths should be relative",
            default = "@@//:relative_rpaths",
        ),
    },
    toolchains = [_RPATH_REWRITER_TOOLCHAIN],
)

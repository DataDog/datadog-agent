"""Set a binary's rpath to the provided value."""

load("@bazel_skylib//lib:paths.bzl", "paths")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

_RPATH_REWRITER_TOOLCHAIN = "//bazel/toolchains/rpath_rewriter"

def _path_segments(path):
    """Returns normalized path segments, ignoring the leading slash."""
    return [segment for segment in paths.normalize(path).split("/") if segment]

def _relative_dir(to_dir, from_dir):
    """Returns the relative path from from_dir to to_dir."""
    if not (paths.is_absolute(to_dir) and paths.is_absolute(from_dir)):
        fail("Cannot compute relative path between '{}' and '{}', they both must be absolute paths".format(from_dir, to_dir))

    to_segments = _path_segments(to_dir)
    from_segments = _path_segments(from_dir)

    common_segments = 0
    for i in range(min(len(to_segments), len(from_segments))):
        if to_segments[i] != from_segments[i]:
            break
        common_segments += 1

    relative = [".."] * (len(from_segments) - common_segments) + to_segments[common_segments:]
    return "/".join(relative) if relative else "."

def _relative_rpath(input, rpath):
    """Returns the appropriate relative rpath from the input to the given rpath."""
    from_dir = input.destination if input.target.is_directory else paths.dirname(input.destination)
    return "./" + _relative_dir(rpath, from_dir)

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

        resolved_rpath = _relative_rpath(input, rpath) if relative else rpath

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

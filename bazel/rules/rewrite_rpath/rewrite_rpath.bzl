"""Set a binary's rpath to the provided value."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

def patchelf_dir_action(ctx, input_dir, output_dir, rpath):
    """Registers a patchelf action to rewrite the rpath of all shared libraries inside a directory.

    Args:
      ctx: the rule context.
      input_dir: the source directory artifact to patch.
      output_dir: the output directory artifact to write.
      rpath: the rpath string to set.
    """
    toolchain = ctx.toolchains["@@//bazel/toolchains/patchelf:patchelf_toolchain_type"].patchelf
    patchelf = toolchain.label[DefaultInfo].files_to_run
    ctx.actions.run_shell(
        inputs = [input_dir],
        tools = [patchelf],
        outputs = [output_dir],
        # /. copies the contents of input rather than nesting it under output
        # (Bazel pre-creates output via declare_directory). chmod restores
        # owner-write so patchelf can rewrite files installed as 0555.
        command = (
            "cp -rL '{input}/.' '{output}' && " +
            "chmod -R u+w '{output}' && " +
            "find '{output}' -type f \\( -name '*.so' -o -name '*.so.*' \\) " +
            "-exec '{patchelf}' --set-rpath '{rpath}' --force-rpath {{}} \\;"
        ).format(
            input = input_dir.path,
            output = output_dir.path,
            patchelf = patchelf.executable.path,
            rpath = rpath,
        ),
    )

def otool_dir_action(ctx, input_dir, output_dir, rpath):
    """Registers install_name_tool actions to rewrite the rpath of all dylibs inside a directory.

    Args:
      ctx: the rule context.
      input_dir: the source directory artifact to patch.
      output_dir: the output directory artifact to write.
      rpath: the rpath string to set.
    """
    otool = ctx.toolchains["@@//bazel/toolchains/otool:otool_toolchain_type"].otool
    args = ctx.actions.args()
    args.add(ctx.file._script.path)
    args.add(ctx.executable._install_name_tool.path)
    args.add(otool.path)
    args.add(rpath)
    args.add(input_dir.path)
    args.add(output_dir.path)
    ctx.actions.run(
        inputs = [input_dir, ctx.file._script],
        tools = [ctx.executable._install_name_tool],
        outputs = [output_dir],
        executable = ctx.file._dir_script,
        arguments = [args],
    )

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

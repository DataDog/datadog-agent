"""Set a binary's rpath to the provided value.

If no rpath is provided, this defaults to <@@//:install_dir>/embedded/lib.
"""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

def _is_os(ctx, constraint):
    return ctx.target_platform_has_constraint(constraint[platform_common.ConstraintValueInfo])

def patchelf_file_action(ctx, input_file, output_file, rpath):
    """Registers a patchelf action to rewrite the rpath of a single file.

    Args:
      ctx: the rule context.
      input_file: the source File to patch.
      output_file: the output File to write.
      rpath: the rpath string to set.
    """
    toolchain = ctx.toolchains["@@//bazel/toolchains/patchelf:patchelf_toolchain_type"].patchelf
    args = ctx.actions.args()
    args.add("--set-rpath", rpath)
    args.add("--force-rpath")
    args.add(input_file.path)
    args.add("--output", output_file.path)
    ctx.actions.run(
        inputs = [input_file],
        outputs = [output_file],
        arguments = [args],
        executable = toolchain.path,
    )

def otool_file_action(ctx, input_file, output_file, rpath):
    """Registers an install_name_tool action to rewrite the rpath of a single file.

    Args:
      ctx: the rule context.
      input_file: the source File to patch.
      output_file: the output File to write.
      rpath: the rpath string to set.
    """
    toolchain = ctx.toolchains["@@//bazel/toolchains/otool:otool_toolchain_type"].otool
    args = ctx.actions.args()
    args.add(toolchain.path)
    args.add(rpath)
    args.add(input_file.path)
    args.add(output_file.path)
    ctx.actions.run(
        inputs = [input_file],
        outputs = [output_file],
        executable = ctx.file._script,
        arguments = [args],
    )

def patchelf_dir_action(ctx, input_dir, output_dir, rpath):
    """Registers a patchelf action to rewrite the rpath of all shared libraries inside a directory."""
    toolchain = ctx.toolchains["@@//bazel/toolchains/patchelf:patchelf_toolchain_type"].patchelf
    ctx.actions.run_shell(
        inputs = [input_dir],
        outputs = [output_dir],
        command = (
            'cp -rL "{input}" "{output}" && ' +
            'find "{output}" -type f \\( -name "*.so" -o -name "*.so.*" \\) ' +
            '-exec "{patchelf}" --set-rpath "{rpath}" --force-rpath {{}} \\;'
        ).format(
            input = input_dir.path,
            output = output_dir.path,
            patchelf = toolchain.path,
            rpath = rpath,
        ),
    )

def otool_dir_action(ctx, input_dir, output_dir, rpath):
    """Registers install_name_tool actions to rewrite the rpath of all dylibs inside a directory."""
    toolchain = ctx.toolchains["@@//bazel/toolchains/otool:otool_toolchain_type"].otool
    ctx.actions.run_shell(
        inputs = [input_dir],
        outputs = [output_dir],
        command = (
            'cp -rL "{input}" "{output}" && ' +
            'find "{output}" -type f -name "*.dylib" | while read -r f; do ' +
            '  install_name_tool -add_rpath "{rpath}" "$f" 2>/dev/null || true; ' +
            '  install_name_tool -id "{rpath}/$(basename "$f")" "$f"; ' +
            '  "{otool}" -L "$f" | tail -n +2 | awk \'{{print $1}}\' | while read -r dep; do ' +
            '    case "$dep" in *sandbox*|*bazel-out*) ' +
            '      install_name_tool -change "$dep" "{rpath}/$(basename "$dep")" "$f" 2>/dev/null || true ;; ' +
            "    esac; " +
            "  done; " +
            "done"
        ).format(
            otool = toolchain.path,
            input = input_dir.path,
            output = output_dir.path,
            rpath = rpath,
        ),
    )

def _rewrite_rpath_impl(ctx):
    is_linux = _is_os(ctx, ctx.attr._linux_constraint)
    is_macos = _is_os(ctx, ctx.attr._macos_constraint)

    if not is_linux and not is_macos:
        return DefaultInfo(files = depset(ctx.files.inputs))

    processed_files = []
    rpath = ctx.attr.rpath.format(install_dir = ctx.attr._install_dir[BuildSettingInfo].value)
    for input in ctx.files.inputs:
        processed_file = ctx.actions.declare_file("patched/" + input.basename)
        if is_linux:
            patchelf_file_action(ctx, input, processed_file, rpath)
        else:
            otool_file_action(ctx, input, processed_file, rpath)
        processed_files.append(processed_file)

    return DefaultInfo(files = depset(processed_files))

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
        "_linux_constraint": attr.label(
            default = "@platforms//os:linux",
        ),
        "_macos_constraint": attr.label(
            default = "@platforms//os:macos",
        ),
        "_script": attr.label(
            default = "@@//bazel/rules/rewrite_rpath:macos.sh",
            allow_single_file = True,
            cfg = "exec",
        ),
        "_install_dir": attr.label(
            doc = "Private label used for the default rpath",
            default = "@@//:install_dir",
        ),
    },
    toolchains = [
        "@@//bazel/toolchains/patchelf:patchelf_toolchain_type",
        "@@//bazel/toolchains/otool:otool_toolchain_type",
    ],
)

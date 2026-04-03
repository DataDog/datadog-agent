"""Set a binary's rpath to the provided value.

If no rpath is provided, this defaults to <@@//:install_dir>/embedded/lib.
"""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

def _is_os(ctx, constraint):
    return ctx.target_platform_has_constraint(constraint[platform_common.ConstraintValueInfo])

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
            toolchain = ctx.toolchains["@@//bazel/toolchains/patchelf:patchelf_toolchain_type"].patchelf
            args = ctx.actions.args()
            args.add("--set-rpath", rpath)
            args.add("--force-rpath")
            args.add(input.path)
            args.add("--output", processed_file.path)
            ctx.actions.run(
                inputs = [input],
                outputs = [processed_file],
                arguments = [args],
                executable = toolchain.path,
            )
        else:
            toolchain = ctx.toolchains["@@//bazel/toolchains/otool:otool_toolchain_type"].otool
            args = ctx.actions.args()
            args.add(toolchain.path)
            args.add(rpath)
            args.add(input.path)
            args.add(processed_file.path)
            ctx.actions.run(
                inputs = [input],
                outputs = [processed_file],
                executable = ctx.file._script,
                arguments = [args],
            )
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

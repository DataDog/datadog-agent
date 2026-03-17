"""Set a binary's rpath to the provided value.

If no rpath is provided, this defaults to <@@//:install_dir>/embedded/lib.
"""

load("@@//bazel/rules/rewrite_rpath:defs.bzl", "PathFixedThingInfo")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")

def _is_os(ctx, constraint):
    return ctx.target_platform_has_constraint(constraint[platform_common.ConstraintValueInfo])

def _rewrite_rpath_impl(ctx):
    is_linux = _is_os(ctx, ctx.attr._linux_constraint)
    is_macos = _is_os(ctx, ctx.attr._macos_constraint)

    if not is_linux and not is_macos:
        return input[DefaultInfo]

    processed_files = []
    replacements = []
    rpath = ctx.attr.rpath.format(install_dir = ctx.attr._install_dir[BuildSettingInfo].value)
    input = ctx.attr.input

    # Get the file to patch.
    print(dir(input))
    files_set = input[DefaultInfo].files
    if not files_set:
        fail("Input must be a file")
    files = files_set.to_list()
    if len(files) != 1:
        fail("Input must be a single file to patch")
    if is_linux:
        toolchain = ctx.toolchains["@@//bazel/toolchains/patchelf:patchelf_toolchain_type"].patchelf
        args = ctx.actions.args()
        args.add("--set-rpath", rpath)
        args.add("--force-rpath")
        args.add(files[0].path)
        args.add("--output", ctx.outputs.out.path)
        ctx.actions.run(
            inputs = [files[0]],
            outputs = [ctx.outputs.out],
            arguments = [args],
            executable = toolchain.path,
        )
    else:
        toolchain = ctx.toolchains["@@//bazel/toolchains/otool:otool_toolchain_type"].otool
        args = ctx.actions.args()
        args.add(toolchain.path)
        args.add(rpath)
        args.add(files[0].path)
        args.add(ctx.outputs.out.path)
        ctx.actions.run(
            inputs = [files[0]],
            outputs = [ctx.outputs.out],
            executable = ctx.file._script,
            arguments = [args],
        )

    ret = [input[DefaultInfo]]
    if CcSharedLibraryInfo in input:
        ret.append(input[CcSharedLibraryInfo])
    ret.append(PathFixedThingInfo(original = input, fixed = ctx.attr.out))
    return ret

patch_rpath = rule(
    implementation = _rewrite_rpath_impl,
    doc = """Set a binary's rpath to the provided value.

    If no rpath is provided, this defaults to <@@//:install_dir>/embedded/lib.""",
    attrs = {
        "input": attr.label(
            doc = "The binary to patch",
            mandatory = True,
        ),
        "out": attr.output(
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

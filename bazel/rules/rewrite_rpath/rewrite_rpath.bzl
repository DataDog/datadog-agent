load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

def _rewrite_rpath_impl(ctx):
    input = ctx.file.input
    if ctx.attr.os == "unsupported":
        return DefaultInfo(files = depset([input]))
    rpath = ctx.attr.rpath.format(install_dir=ctx.attr._install_dir[BuildSettingInfo].value)
    processed_file = ctx.actions.declare_file("patched/" + input.basename)
    if ctx.attr.os == "linux":
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
            executable = ctx.file.script,
            arguments = [args],
        )

    return DefaultInfo(files = depset([processed_file]))

_rewrite_rpath = rule(
    implementation = _rewrite_rpath_impl,
    attrs = {
        "input": attr.label(
            allow_single_file = True,
            doc = "The binary to patch",
        ),
        "os": attr.string(
            mandatory = True,
            doc = "Private attribute to dispatch based on the target OS",
        ),
        "rpath": attr.string(
            doc = """
            The new rpath. Defaults to <@@//:install_dir>/embedded/lib
            This supports '{install_dir}' variable
            """,
            default = "{install_dir}/embedded/lib",
        ),
        "script": attr.label(
            doc = "A script that will wrap the native tool to update rpath",
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

def rewrite_rpath(name, input, rpath = None):
    """
    Set a binary's rpath to the provided value.

    If no rpath is provided, this defaults to <@@//:install_dir>/embedded
    Args:
        - input: The binary to patch the rpath for
        - rpath: (optional) The rpath to use f& this binary
    """
    _rewrite_rpath(
        name = name,
        input = input,
        rpath = rpath,
        script = select({
            "@platforms//os:macos": ":macos.sh",
            "//conditions:default": None,
        }),
        os = select({
            "@platforms//os:linux": "linux",
            "@platforms//os:macos": "macos",
            "//conditions:default": "unsupported",
        }),
    )

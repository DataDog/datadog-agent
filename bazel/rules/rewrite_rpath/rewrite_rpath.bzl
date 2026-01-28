load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

def _replace_prefix_impl(ctx):
    "Set binaries rpath the configured install directory"

    if BuildSettingInfo not in ctx.attr.prefix:
        fail("The provided prefix label doesn't provide BuildSettingInfo")
    input = ctx.file.input
    if ctx.attr.os == "unsupported":
        return DefaultInfo(files = depset([input]))
    prefix = ctx.attr.prefix[BuildSettingInfo].value
    processed_file = ctx.actions.declare_file("patched/" + input.basename)
    if ctx.attr.os == "linux":
        ctx.actions.run(
            inputs = [input],
            outputs = [processed_file],
            arguments = ["--set-rpath", prefix, input.path, "--output", processed_file.path],
            executable = ctx.executable._patchelf,
        )
    else:
        ctx.actions.run_shell(
            inputs = [input],
            outputs = [processed_file],
            tools = [":macos.sh"],
            command = "$(location :macos.sh) {} {}".format(prefix, input),
        )

    return DefaultInfo(files = depset([processed_file]))

_replace_prefix = rule(
    implementation = _replace_prefix_impl,
    attrs = {
        "input": attr.label(
            allow_single_file = True,
            doc = "The binary to patch",
        ),
        "os": attr.string(
            mandatory = True,
            doc = "Private attribute to dispatch based on the target OS",
        ),
        "prefix": attr.label(),
        "_patchelf": attr.label(
            cfg = "exec",
            executable = True,
            default = Label("@patchelf"),
        ),
    },
)

def rewrite_rpath(name, input, prefix):
    _replace_prefix(
        name = name,
        input = input,
        prefix = prefix,
        os = select({
            "@platforms//os:linux": "linux",
            "@platforms//os:macos": "macos",
            "//conditions:default": "unsupported",
        }),
    )

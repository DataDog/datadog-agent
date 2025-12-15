load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

def _replace_prefix_impl(ctx):
    inputs = ctx.files.inputs
    if BuildSettingInfo not in ctx.attr.prefix:
        fail("The provided prefix label doesn't provide BuildSettingInfo")
    prefix = ctx.attr.prefix[BuildSettingInfo].value
    processed_files = []
    for input in inputs:
        processed_file = ctx.actions.declare_file("patched_" + input.basename)
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
        processed_files.append(processed_file)

    return DefaultInfo(files = depset(processed_files))

_replace_prefix = rule(
    implementation = _replace_prefix_impl,
    attrs = {
        "inputs": attr.label_list(allow_files = True),
        "prefix": attr.label(),
        "os": attr.string(),
        "_patchelf": attr.label(
            cfg = "exec",
            executable = True,
            default = Label("@patchelf")
        ),
    },
)

def rewrite_rpath(name, inputs, prefix):
    _replace_prefix(
        name = name,
        inputs = inputs,
        prefix = prefix,
        os = select({
            "@platforms//os:linux": "linux",
            "@platforms//os:macos": "macos",
        })
    )

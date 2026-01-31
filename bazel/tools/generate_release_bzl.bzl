"""Generates release.bzl from release.json."""

def _generate_release_bzl_impl(ctx):
    """Implementation of generate_release_bzl rule."""

    args = ctx.actions.args()
    args.add(ctx.file.release_json)
    args.add(ctx.outputs.out.path)
    ctx.actions.run(
        executable = ctx.executable._tool,
        arguments = [args],
        outputs = [ctx.outputs.out],
        inputs = [ctx.file.release_json],
        mnemonic = "GenerateReleaseBzl",
    )
    return [DefaultInfo(files = depset([ctx.outputs.out]))]

generate_release_bzl = rule(
    implementation = _generate_release_bzl_impl,
    doc = "Generate a MODULE.bazel file from overlay directory structure",
    attrs = {
        "release_json": attr.label(
            mandatory = True,
            allow_single_file = True,
            doc = "release.json",
        ),
        "out": attr.output(
            mandatory = True,
            doc = "output file",
        ),
        "_tool": attr.label(
            executable = True,
            cfg = "exec",
            default = "//bazel/tools:generate_release_bzl",
        ),
    },
)

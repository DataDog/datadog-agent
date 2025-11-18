# buildifier: disable=module-docstring
# buildifier: disable=bzl-visibility
load("//foreign_cc/private/framework:helpers.bzl", "convert_shell_script")

def _impl(ctx):
    text = convert_shell_script(ctx, ctx.attr.script)
    out = ctx.actions.declare_file(ctx.attr.out)
    ctx.actions.write(
        output = out,
        content = text,
    )
    return [DefaultInfo(files = depset([out]))]

shell_script_helper_test_rule = rule(
    implementation = _impl,
    attrs = {
        "out": attr.string(mandatory = True),
        "script": attr.string_list(mandatory = True),
    },
    toolchains = [
        "@rules_foreign_cc//foreign_cc/private/framework:shell_toolchain",
    ],
)

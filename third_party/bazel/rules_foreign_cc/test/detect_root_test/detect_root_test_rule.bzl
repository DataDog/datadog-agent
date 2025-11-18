"""A helper rule for testing detect_root function."""

# buildifier: disable=bzl-visibility
load("@rules_foreign_cc//foreign_cc/private:detect_root.bzl", "detect_root")

def _impl(ctx):
    detected_root = detect_root(ctx.attr.srcs)
    out = ctx.actions.declare_file(ctx.attr.out)
    ctx.actions.write(
        output = out,
        content = detected_root,
    )
    return [DefaultInfo(files = depset([out]))]

detect_root_test_rule = rule(
    implementation = _impl,
    attrs = {
        "out": attr.string(mandatory = True),
        "srcs": attr.label(mandatory = True),
    },
)

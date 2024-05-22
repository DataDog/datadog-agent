# Rule for generating the Agent config file from the template
def _generate_config_impl(ctx):
    build_type = ctx.attr.build_type
    out = ctx.actions.declare_file(ctx.attr.out)
    template = ctx.files._template

    args = ctx.actions.args()
    args.add(build_type)
    args.add_all(template)
    args.add(out)
    ctx.actions.run(
        mnemonic = "GenerateConfig",
        executable = ctx.executable._renderer,
        arguments = [args],
        inputs = template,
        outputs = [out],
    )

    return DefaultInfo(files = depset([out]))


generate_config = rule(
    implementation = _generate_config_impl,
    attrs = {
        "build_type": attr.string(mandatory = True),
        "out": attr.string(default = "datadog.yaml"),
        "_renderer": attr.label(
            default = Label("//pkg/config:render_config"),
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "_template": attr.label(
            default = Label("//pkg/config:template"),
            allow_single_file = True,
        ),
    },
)

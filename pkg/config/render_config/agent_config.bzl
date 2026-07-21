# Rule for generating the Agent config file from the template

def _agent_config_impl(ctx):
    build_type = ctx.attr.build_type
    template = ctx.file.template

    args = ctx.actions.args()
    args.add(build_type)
    args.add(template.path)
    args.add(ctx.outputs.out.path)
    ctx.actions.run(
        mnemonic = "GenerateConfig",
        executable = ctx.executable._renderer,
        arguments = [args],
        inputs = [template],
        outputs = [ctx.outputs.out],
    )
    return DefaultInfo(files = depset([ctx.outputs.out]))

agent_config = rule(
    implementation = _agent_config_impl,
    attrs = {
        "build_type": attr.string(
            doc = """One of agent-py3, iot-agent, dogstatsd, dca, dcacf, security-agent, or system-probe.""",
            mandatory = True,
        ),
        "out": attr.output(mandatory = True),
        "template": attr.label(
            default = Label("//pkg/config:config_template.yaml"),
            allow_single_file = True,
            mandatory = True,
        ),
        "_renderer": attr.label(
            default = Label("//pkg/config/render_config:render_config"),
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
    },
)

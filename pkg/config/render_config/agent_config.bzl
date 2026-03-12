    if build_type == "system-probe":
        args["template_file"] = "./pkg/config/system-probe_template.yaml"
    elif build_type == "security-agent":
        args["template_file"] = "./pkg/config/security-agent_template.yaml"
# Rule for generating the Agent config file from the template

def _agent_config_impl(ctx):
    build_type = ctx.attr.build_type
    out = ctx.actions.declare_file(ctx.attr.out)
    templates = ctx.attr._templates

    args = ctx.actions.args()
    args.add(build_type)
    args.add_all(templates)
    args.add(out)
    ctx.actions.run(
        mnemonic = "GenerateConfig",
        executable = ctx.executable._renderer,
        arguments = [args],
        inputs = templates,
        outputs = [out],
    )
    return DefaultInfo(files = depset([out]))

agent_config = rule(
    implementation = _agent_config_impl,
    attrs = {
        "build_type": attr.string(
            doc = """One of agent-py3, iot-agent, dogstatsd, dca, dcacf, security-agent, or system-probe.""",
            mandatory = True,
        ),
        "out": attr.string(default = "datadog.yaml"),
        "_renderer": attr.label(
            default = Label("//pkg/config/render_config:render_config"),
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "_templates": attr.label(
            default = Label("//pkg/config:templates"),
        ),
    },
)

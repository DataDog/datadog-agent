"""Bazel rule for rendering a config example YAML from an enriched schema.

Inputs (per rule invocation):
  - top_schema: the top-level schema file (core_schema.yaml or
    system-probe_schema.yaml). For the core agent flavors this entry
    references per-section sub-files via $ref, so those sub-files must
    also be supplied via `srcs`.
  - srcs: additional schema sub-files that the top_schema references.
  - build_type: agent-py3, iot-agent, dogstatsd, dca, dcacf, or
    system-probe.
  - os_target: linux, windows, or darwin.
  - out: the rendered example yaml.
"""

def _schema_template_config_impl(ctx):
    args = ctx.actions.args()
    args.add(ctx.file.top_schema.path)
    args.add(ctx.attr.build_type)
    args.add(ctx.attr.os_target)
    args.add(ctx.outputs.out.path)

    ctx.actions.run(
        mnemonic = "SchemaTemplateConfig",
        executable = ctx.executable._template,
        arguments = [args],
        inputs = [ctx.file.top_schema] + ctx.files.srcs,
        outputs = [ctx.outputs.out],
        progress_message = "Rendering %s (%s, %s)" % (
            ctx.outputs.out.short_path,
            ctx.attr.build_type,
            ctx.attr.os_target,
        ),
    )
    return DefaultInfo(files = depset([ctx.outputs.out]))

schema_template_config = rule(
    implementation = _schema_template_config_impl,
    attrs = {
        "build_type": attr.string(
            doc = "One of agent-py3, iot-agent, dogstatsd, dca, dcacf, system-probe.",
            mandatory = True,
        ),
        "os_target": attr.string(
            doc = "Target OS: one of linux, windows, darwin. Typically populated via `select()` on `@platforms//os`.",
            mandatory = True,
        ),
        "top_schema": attr.label(
            doc = "The top-level enriched schema YAML file.",
            allow_single_file = [".yaml"],
            mandatory = True,
        ),
        "srcs": attr.label_list(
            doc = "Additional schema sub-files referenced by the top schema via $ref. Empty for system-probe (single-file schema).",
            allow_files = [".yaml"],
            default = [],
        ),
        "out": attr.output(
            doc = "Path of the rendered example yaml file.",
            mandatory = True,
        ),
        "_template": attr.label(
            default = Label("//tasks/schema:schema_template"),
            executable = True,
            cfg = "exec",
        ),
    },
)

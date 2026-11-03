"""Run the Windows CWS schema generator under rules_wine."""

load("@rules_wine//wine:actions.bzl", "windows_actions")

def _windows_schema_gen_impl(ctx):
    out = ctx.actions.declare_file(ctx.label.name + "/backend_windows.schema.json")
    runner = windows_actions.resolve(ctx)
    args = ctx.actions.args()
    args.add("-serializers-dir=pkg/security/serializers")
    args.add("-output")
    windows_actions.add_path(runner, args, out)
    windows_actions.run(
        ctx,
        runner = runner,
        executable = ctx.attr.tool[DefaultInfo].files_to_run,
        arguments = [args],
        inputs = ctx.files.srcs,
        outputs = [out],
        mnemonic = "WindowsSchemaGen",
        progress_message = "Generating Windows schema for %{label}",
    )
    return [DefaultInfo(files = depset([out]))]

windows_schema_gen = rule(
    implementation = _windows_schema_gen_impl,
    doc = "Runs the Windows backend_doc binary through the rules_wine toolchain.",
    attrs = {
        "srcs": attr.label_list(
            allow_files = True,
            doc = "Serializer sources the generator reads from the execroot.",
        ),
        "tool": attr.label(
            cfg = "exec",
            executable = True,
            mandatory = True,
            doc = "Windows amd64 backend_doc binary.",
        ),
    },
    toolchains = [windows_actions.toolchain_type],
)

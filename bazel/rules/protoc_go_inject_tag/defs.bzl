"""Apply `protoc-go-inject-tag` to a go_proto_library's outputs.

Relies on `add-outputdir-flag.patch` (pending https://github.com/favadi/protoc-go-inject-tag/pull/73): the tool gains
`-outputdir=DIR`, so the rule runs a single hermetic action that writes tagged copies to the rule's output directory.
"""

def _impl(ctx):
    inputs = ctx.attr.go_proto_library[OutputGroupInfo].go_generated_srcs.to_list()
    outputs = [ctx.actions.declare_file(f.basename) for f in inputs]
    ctx.actions.run(
        arguments = [
            "-input={}/*.go".format(inputs[0].dirname),
            "-outputdir={}".format(outputs[0].dirname),
        ],
        executable = ctx.executable._tool,
        inputs = inputs,
        mnemonic = "ProtocGoInjectTag",
        outputs = outputs,
    )
    return [OutputGroupInfo(go_generated_srcs = depset(outputs))]

protoc_go_inject_tag = rule(
    implementation = _impl,
    attrs = {
        "go_proto_library": attr.label(mandatory = True),
        "_tool": attr.label(cfg = "exec", default = "@com_github_favadi_protoc_go_inject_tag//:protoc-go-inject-tag", executable = True),
    },
)

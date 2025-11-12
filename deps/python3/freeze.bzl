def _python_freeze_impl(name, visibility, input, module, output):
    native.genrule(
        name = name,
        srcs = [input],
        tools = [":freeze_module"],
        outs = [output],
        cmd = "$(location :freeze_module) {} $(location {}) $@".format(module, input),
        visibility = visibility,
    )

python_freeze = macro(
    implementation = _python_freeze_impl,
    attrs = {
        "input": attr.label(mandatory=True, allow_single_file=True, configurable=False),
        "module": attr.string(mandatory=True, configurable=False),
        "output": attr.string(mandatory=True, configurable=False),
    }
)

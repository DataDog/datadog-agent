def _gen_pkgconfig_impl(ctx):
    out = ctx.actions.declare_file(ctx.file.template.basename.replace(".pc.in", ".pc"))
    ctx.actions.expand_template(
        output = out,
        template = ctx.file.template,
        substitutions = {
            "{{VERSION}}": ctx.attr.version
        }
    )
    return [DefaultInfo(files = depset([out]))]

gen_pkgconfig = rule(
    implementation = _gen_pkgconfig_impl,
    attrs = {
        "version": attr.string(mandatory = True),
        "template": attr.label(
            allow_single_file = [".pc.in"],
            mandatory = True
        ),
    }
)

def _replace_shebang_impl(ctx):
    input = ctx.file.input
    output = ctx.actions.declare_file(input.basename)
    ctx.actions.run_shell(
        outputs = [output],
        inputs = [input],
        command = "{{ echo '#!{program}' ; tail -n +{nb_lines} {input} ;}} > {output}".format(
            program=ctx.attr.program,
            input=input.path,
            output=output.path,
            nb_lines=ctx.attr.nb_lines,
        ),
    )
    return [DefaultInfo(files = depset([output]))]

replace_shebang = rule(
    implementation = _replace_shebang_impl,
    attrs = {
        "input": attr.label(allow_single_file=True, mandatory=True),
        "program": attr.string(mandatory=True),
        # Some python shebangs are scattered on multiple lines
        "nb_lines": attr.int(default=1)
    },
)

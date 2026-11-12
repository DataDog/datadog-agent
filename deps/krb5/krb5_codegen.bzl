"""Code generation rules mirroring the krb5 upstream Makefiles, implemented with gawk."""

_GAWK = attr.label(
    default = Label("@gawk"),
    executable = True,
    cfg = "exec",
    doc = "gawk interpreter used to run the generators.",
)

def _krb5_compile_et_impl(ctx):
    for script, out in [(ctx.file._et_h, ctx.outputs.out_h), (ctx.file._et_c, ctx.outputs.out_c)]:
        args = ctx.actions.args()
        args.add("-f", script)
        args.add(out, format = "outfile=%s")
        args.add(out.basename, format = "displayname=%s")
        args.add(ctx.file.src)
        ctx.actions.run(
            executable = ctx.executable._gawk,
            arguments = [args],
            inputs = [script, ctx.file.src],
            outputs = [out],
            mnemonic = "Krb5CompileEt",
            progress_message = "Compiling error table {} into %{{output}}".format(ctx.file.src.short_path),
            toolchain = None,
        )
    return [DefaultInfo(files = depset([ctx.outputs.out_h, ctx.outputs.out_c]))]

krb5_compile_et = rule(
    implementation = _krb5_compile_et_impl,
    doc = "Compiles a com_err error table (.et) into a C source and header, like upstream compile_et.",
    attrs = {
        "src": attr.label(
            allow_single_file = [".et"],
            mandatory = True,
            doc = "Error table source.",
        ),
        "out_h": attr.output(
            mandatory = True,
            doc = "Generated header.",
        ),
        "out_c": attr.output(
            mandatory = True,
            doc = "Generated C source.",
        ),
        "_et_h": attr.label(
            default = Label("@krb5//src/util:et/et_h.awk"),
            allow_single_file = True,
        ),
        "_et_c": attr.label(
            default = Label("@krb5//src/util:et/et_c.awk"),
            allow_single_file = True,
        ),
        "_gawk": _GAWK,
    },
)

_CONCAT_PROGRAM = """\
BEGIN { if (prologue != "") print prologue > out }
{
    n = split(line_format, parts, "&")
    s = parts[1]
    for (i = 2; i <= n; i++)
        s = s $0 parts[i]
    print s > out
}
END { if (epilogue != "") print epilogue > out }
"""

def _krb5_concat_impl(ctx):
    args = ctx.actions.args()
    args.add("-v", "out=" + ctx.outputs.out.path)
    args.add("-v", "prologue=" + ctx.attr.prologue)
    args.add("-v", "epilogue=" + ctx.attr.epilogue)
    args.add("-v", "line_format=" + ctx.attr.line_format)
    args.add(_CONCAT_PROGRAM)
    args.add_all(ctx.files.inputs)
    ctx.actions.run(
        executable = ctx.executable._gawk,
        arguments = [args],
        inputs = ctx.files.inputs,
        outputs = [ctx.outputs.out],
        mnemonic = "Krb5Concat",
        progress_message = "Generating %{output}",
        toolchain = None,
    )
    return [DefaultInfo(files = depset([ctx.outputs.out]))]

krb5_concat = rule(
    implementation = _krb5_concat_impl,
    doc = "Concatenates files in order, with optional prologue, epilogue and per-line format, like the upstream cat/sed recipes.",
    attrs = {
        "inputs": attr.label_list(
            allow_files = True,
            mandatory = True,
            doc = "Files to concatenate, in order. Not named srcs so buildifier keeps the order.",
        ),
        "out": attr.output(
            mandatory = True,
            doc = "Concatenated output.",
        ),
        "prologue": attr.string(
            doc = "Text written before the first file. Awk escape sequences such as \\\\n are interpreted.",
        ),
        "epilogue": attr.string(
            doc = "Text written after the last file. Awk escape sequences such as \\\\n are interpreted.",
        ),
        "line_format": attr.string(
            default = "&",
            doc = "Format applied to every input line, where each & is replaced by the line (like sed s/.*/.../).",
        ),
        "_gawk": _GAWK,
    },
)

_PERL_TOOLCHAIN = "@rules_perl//perl:exec_toolchain_type"

def _krb5_perl_gen_impl(ctx):
    perl = ctx.toolchains[_PERL_TOOLCHAIN].perl_runtime
    args = ctx.actions.args()
    args.add_all(ctx.attr.perl_args)
    args.add("-I" + ctx.file.script.dirname)
    args.add(ctx.file.script)
    args.add_all([a.format(out = ctx.outputs.out.path) for a in ctx.attr.script_args])
    ctx.actions.run(
        executable = perl.interpreter,
        arguments = [args],
        inputs = depset([ctx.file.script] + ctx.files.modules),
        tools = perl.runtime,
        outputs = [ctx.outputs.out],
        mnemonic = "Krb5PerlGen",
        progress_message = "Generating %{output}",
        toolchain = _PERL_TOOLCHAIN,
    )
    return [DefaultInfo(files = depset([ctx.outputs.out]))]

krb5_perl_gen = rule(
    implementation = _krb5_perl_gen_impl,
    doc = "Runs one of the krb5 util/*.pl template generators, like the upstream Makefiles do.",
    attrs = {
        "script": attr.label(
            allow_single_file = [".pl"],
            mandatory = True,
            doc = "Generator script. Its directory is added to the module search path.",
        ),
        "modules": attr.label_list(
            allow_files = [".pm"],
            doc = "Perl modules the script loads from its directory.",
        ),
        "perl_args": attr.string_list(
            doc = "Interpreter flags, passed before the script.",
        ),
        "script_args": attr.string_list(
            doc = "Script arguments. {out} is replaced by the output path.",
        ),
        "out": attr.output(
            mandatory = True,
            doc = "Generated file.",
        ),
    },
    toolchains = [_PERL_TOOLCHAIN],
)

_VERSION_SCRIPT_PROGRAM = """\
BEGIN { print node " {" > out }
{ print $0 ";" > out }
END {
    print "};" > out
    print "HIDDEN { local: __*; _rest*; _save*; *; };" > out
}
"""

def _krb5_version_script_impl(ctx):
    args = ctx.actions.args()
    args.add("-v", "out=" + ctx.outputs.out.path)
    args.add("-v", "node={}_{}_MIT".format(ctx.attr.libbase.replace("-", "_", 1), ctx.attr.libmajor))
    args.add(_VERSION_SCRIPT_PROGRAM)
    args.add(ctx.file.exports)
    ctx.actions.run(
        executable = ctx.executable._gawk,
        arguments = [args],
        inputs = [ctx.file.exports],
        outputs = [ctx.outputs.out],
        mnemonic = "Krb5VersionScript",
        progress_message = "Generating %{output}",
        toolchain = None,
    )
    return [DefaultInfo(files = depset([ctx.outputs.out]))]

krb5_version_script = rule(
    implementation = _krb5_version_script_impl,
    doc = "Generates a GNU ld version script from a krb5 .exports file, like upstream binutils.versions.",
    attrs = {
        "exports": attr.label(
            allow_single_file = [".exports"],
            mandatory = True,
            doc = "List of exported symbols, one per line.",
        ),
        "libbase": attr.string(
            mandatory = True,
            doc = "LIBBASE from the upstream Makefile.in.",
        ),
        "libmajor": attr.string(
            mandatory = True,
            doc = "LIBMAJOR from the upstream Makefile.in.",
        ),
        "out": attr.output(
            mandatory = True,
            doc = "Generated version script.",
        ),
        "_gawk": _GAWK,
    },
)

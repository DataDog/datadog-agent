"""Rules for compiling Windows resource files (.mc -> .rc -> .syso)."""

_DEFAULT_MINGW_PATH = "C:/tools/msys64/mingw64"

def _win_messagetable_impl(ctx):
    src = ctx.file.src
    basename = src.basename.replace(".mc", "")

    rc_out = ctx.actions.declare_file(basename + ".rc")
    h_out = ctx.actions.declare_file(basename + ".h")
    bin_out = ctx.actions.declare_file("MSG00409.bin")

    windmc = ctx.attr._mingw_path + "/bin/windmc"
    ctx.actions.run_shell(
        outputs = [rc_out, h_out, bin_out],
        inputs = [src],
        command = '"{windmc}" --target pe-x86-64 -r "{outdir}" -h "{outdir}" "{src}"'.format(
            windmc = windmc,
            outdir = rc_out.dirname,
            src = src.path,
        ),
        mnemonic = "WindMC",
        progress_message = "Compiling message table %s" % src.short_path,
    )

    syso_out = ctx.actions.declare_file("rsrc.syso")
    windres = ctx.attr._mingw_path + "/bin/windres"
    ctx.actions.run_shell(
        outputs = [syso_out],
        inputs = [rc_out, bin_out],
        command = '"{windres}" --target pe-x86-64 -i "{rc}" -O coff -o "{out}"'.format(
            windres = windres,
            rc = rc_out.path,
            out = syso_out.path,
        ),
        mnemonic = "WindRes",
        progress_message = "Linking message resource %s" % rc_out.short_path,
    )

    return [DefaultInfo(files = depset([syso_out, h_out]))]

win_messagetable = rule(
    implementation = _win_messagetable_impl,
    doc = "Compiles a .mc message file into a .syso resource and .h header via windmc + windres.",
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".mc"]),
        "_mingw_path": attr.string(default = _DEFAULT_MINGW_PATH),
    },
)

def _win_resource_impl(ctx):
    src = ctx.file.src
    syso_out = ctx.actions.declare_file("rsrc.syso")

    windres = ctx.attr._mingw_path + "/bin/windres"

    defines = " ".join([
        "--define %s=%s" % (k, v)
        for k, v in ctx.attr.defines.items()
    ])

    # windres resolves #include relative to -I paths;
    # resource paths (ICON etc.) resolve relative to the .rc file's directory.
    include_dirs = {src.dirname: True}
    for dep in ctx.files.deps:
        include_dirs[dep.dirname] = True
    includes = " ".join(['-I "%s"' % d for d in include_dirs])

    ctx.actions.run_shell(
        outputs = [syso_out],
        inputs = [src] + ctx.files.deps,
        command = '"{windres}" --target pe-x86-64 {defines} {includes} -i "{rc}" -O coff -o "{out}"'.format(
            windres = windres,
            defines = defines,
            includes = includes,
            rc = src.path,
            out = syso_out.path,
        ),
        mnemonic = "WindRes",
        progress_message = "Linking resource %s" % src.short_path,
    )

    return [DefaultInfo(files = depset([syso_out]))]

win_resource = rule(
    implementation = _win_resource_impl,
    doc = "Compiles a .rc resource file into a .syso object via windres.",
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".rc"]),
        "deps": attr.label_list(allow_files = True),
        "defines": attr.string_dict(),
        "_mingw_path": attr.string(default = _DEFAULT_MINGW_PATH),
    },
)

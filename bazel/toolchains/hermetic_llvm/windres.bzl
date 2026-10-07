"""llvm-windres with the mingw-w64 headers, as a drop-in for GNU windres."""

def _runfiles_path(ctx, file):
    if file.short_path.startswith("../"):
        return file.short_path[len("../"):]
    return ctx.workspace_name + "/" + file.short_path

def _llvm_windres_impl(ctx):
    script = ctx.actions.declare_file(ctx.label.name + ".sh")
    generated = _runfiles_path(ctx, ctx.file._mingw_generated_crt_h)
    ctx.actions.write(
        output = script,
        content = "\n".join([
            "#!/usr/bin/env bash",
            "r=\"$0.runfiles\"",
            "exec \"$r/{windres}\" -I\"$r/{include}\" -I\"$r/{crt}\" -I\"$r/{generated}\" \"$@\"".format(
                windres = _runfiles_path(ctx, ctx.executable._llvm_windres),
                include = _runfiles_path(ctx, ctx.file._mingw_include),
                crt = _runfiles_path(ctx, ctx.file._mingw_crt),
                generated = generated[:generated.rindex("/")],
            ),
            "",
        ]),
        is_executable = True,
    )
    runfiles = ctx.runfiles(files = [
        ctx.executable._llvm_windres,
        ctx.file._clang,
        ctx.file._mingw_include,
        ctx.file._mingw_crt,
        ctx.file._mingw_generated_crt_h,
    ])
    return DefaultInfo(executable = script, runfiles = runfiles)

llvm_windres = rule(
    implementation = _llvm_windres_impl,
    doc = "Wraps llvm-windres so it preprocesses with the mingw-w64 headers.",
    executable = True,
    attrs = {
        "_clang": attr.label(default = "@llvm//tools:clang", allow_single_file = True, cfg = "exec"),
        "_llvm_windres": attr.label(default = "@llvm//tools:llvm-windres", executable = True, allow_single_file = True, cfg = "exec"),
        "_mingw_crt": attr.label(default = "@mingw//:mingw_w64_headers_crt_directory", allow_single_file = True),
        "_mingw_generated_crt_h": attr.label(default = "@mingw//:mingw-w64-headers/crt/_mingw.h", allow_single_file = True),
        "_mingw_include": attr.label(default = "@mingw//:mingw_w64_headers_include_directory", allow_single_file = True),
    },
)

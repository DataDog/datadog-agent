"""Bazel rule and macro generating a compatibility shim package via aliasgen."""

load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")

def _go_alias_gen_impl(ctx):
    out = ctx.actions.declare_file(ctx.label.name + ".go")

    go_bin = ctx.file._go_bin
    go_root = ctx.file._go_root
    go_files = ctx.attr._go_files[DefaultInfo].files

    args = ctx.actions.args()
    args.add("-src", ctx.attr.src)
    args.add("-pkg", ctx.attr.pkg_name)
    args.add("-out", out)
    args.add("-go", go_bin)
    args.add("-goroot", go_root.dirname)
    if ctx.attr.alias:
        args.add("-alias", ctx.attr.alias)

    ctx.actions.run(
        executable = ctx.executable._tool,
        arguments = [args],
        inputs = depset(
            [go_bin, go_root] + ctx.files.module_files,
            transitive = [go_files],
        ),
        outputs = [out],
        mnemonic = "GoAlias",
        progress_message = "Generating alias shim for %s" % ctx.attr.src,
        use_default_shell_env = False,
    )

    return [DefaultInfo(files = depset([out]))]

_go_alias_gen = rule(
    implementation = _go_alias_gen_impl,
    attrs = {
        "src": attr.string(mandatory = True, doc = "Import path of the source package to forward to."),
        "pkg_name": attr.string(mandatory = True, doc = "Package name for the generated shim."),
        "alias": attr.string(default = "", doc = "Identifier for the imported source package in generated code."),
        "module_files": attr.label_list(allow_files = True, doc = "go.mod/go.sum/go.work files needed to resolve src."),
        "_tool": attr.label(default = "//bazel/rules/go_alias", executable = True, cfg = "exec"),
        "_go_bin": attr.label(default = "@go_work_sdk//:bin/go", allow_single_file = True, cfg = "exec"),
        "_go_root": attr.label(default = "@go_work_sdk//:ROOT", allow_single_file = True, cfg = "exec"),
        "_go_files": attr.label(default = "@go_work_sdk//:files", cfg = "exec"),
    },
)

def _go_alias_macro_impl(name, visibility, src, pkg_name, alias, out_file, module_files):
    gen = name + "_gen"
    _go_alias_gen(
        name = gen,
        src = src,
        pkg_name = pkg_name,
        alias = alias,
        module_files = module_files,
    )
    native.exports_files([out_file], visibility = visibility)
    write_source_file(
        name = name,
        visibility = visibility,
        in_file = ":" + gen,
        out_file = out_file,
        check_that_out_file_exists = False,
    )

go_alias = macro(
    doc = """Generate a compatibility shim package that forwards every exported symbol of `src` via type/value aliases.

    The main target (`name`) verifies the generated output matches the committed file.
    Use `bazel test` to verify, `bazel run` to update.
    """,
    attrs = {
        "src": attr.string(mandatory = True, configurable = False),
        "pkg_name": attr.string(mandatory = True, configurable = False),
        "alias": attr.string(default = "", configurable = False),
        "out_file": attr.string(mandatory = True, configurable = False),
        "module_files": attr.label_list(default = [], configurable = False),
    },
    implementation = _go_alias_macro_impl,
)

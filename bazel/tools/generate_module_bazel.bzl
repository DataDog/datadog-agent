"""Generates http_archive target from overlay directory structure."""

def _generate_module_bazel_impl(ctx):
    """Implementation of generate_module_bazel rule."""

    overlay_files = ctx.actions.declare_file("_%s_overlay_files" % ctx.label.name)
    ctx.actions.write(overlay_files, "\n".join([f.path for f in ctx.files.overlay_files]))
    args = ctx.actions.args()
    args.add("--files", overlay_files.path)
    args.add("--module", ctx.attr.module)
    args.add("--package", ctx.label.package)
    args.add_all(ctx.attr.urls, before_each = "--url")
    args.add("--sha256", ctx.attr.sha256)
    args.add("--strip_prefix", ctx.attr.strip_prefix)
    args.add("--output", ctx.outputs.out.path)
    ctx.actions.run(
        executable = ctx.executable._tool,
        arguments = [args],
        outputs = [ctx.outputs.out],
        inputs = [overlay_files],
        mnemonic = "GenerateModuleBazel",
    )
    return [DefaultInfo(files = depset([ctx.outputs.out]))]

_generate_module_bazel = rule(
    implementation = _generate_module_bazel_impl,
    doc = "Generate a MODULE.bazel file from overlay directory structure",
    attrs = {
        "module": attr.string(
            mandatory = True,
            doc = "Name of the module",
        ),
        "urls": attr.string_list(
            mandatory = True,
            doc = "URLs for http_archive",
        ),
        "sha256": attr.string(
            mandatory = True,
            doc = "SHA256 hash",
        ),
        "strip_prefix": attr.string(
            mandatory = True,
            doc = "Strip prefix for archive",
        ),
        "out": attr.output(
            mandatory = True,
            doc = "Output MODULE.bazel file",
        ),
        "overlay_files": attr.label_list(
            allow_files = True,
        ),
        "_tool": attr.label(
            executable = True,
            cfg = "exec",
            default = "//bazel/tools:generate_module_bazel",
        ),
    },
)

def generate_module_bazel(name = None, module = None, out = None, sha256 = None, strip_prefix = None, url = None, urls = [], **kwargs):
    _generate_module_bazel(
        name = name,
        module = module,
        out = out,
        overlay_files = native.glob(["overlay/**"]),
        sha256 = sha256,
        strip_prefix = strip_prefix,
        urls = urls or [url],
        **kwargs
    )

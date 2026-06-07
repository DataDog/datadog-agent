"""`gomock` source mode with a stable `// Source:` header.

`@rules_go//extras:gomock` stages the source into a sandbox path that ends up in the generated `// Source:` header and
therefore varies by CPU/compilation_mode, breaking reproducibility: this wrapper rewrites it to a repo-relative path.
"""

load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")
load("@rules_go//extras:gomock.bzl", "gomock")

def _strip_sandbox_path_impl(ctx):
    ctx.actions.expand_template(
        output = ctx.outputs.out,
        substitutions = {"{}/gopath/src/{}/".format(ctx.file.raw.dirname, ctx.attr.prefix): ""},
        template = ctx.file.raw,
    )

_strip_sandbox_path = rule(
    implementation = _strip_sandbox_path_impl,
    attrs = {
        "out": attr.output(),
        "prefix": attr.string(),
        "raw": attr.label(allow_single_file = True),
    },
)

def _impl(name, importpath, out, prefix, src, visibility):
    raw = "{}_raw".format(name)
    gomock(
        name = raw,
        out = "{}.raw".format(out),
        source = src,
        source_importpath = importpath,
        mockgen_tool = "@org_uber_go_mock//mockgen",
        mockgen_args = ["-build_constraint=test"],
    )
    stripped = "{}_stripped".format(name)
    _strip_sandbox_path(
        name = stripped,
        out = "{}.stripped".format(out),
        prefix = prefix,
        raw = ":{}".format(raw),
    )
    native.exports_files([out], visibility)
    write_source_file(name = name, in_file = ":{}".format(stripped), out_file = out, check_that_out_file_exists = False, visibility = ["//visibility:public"])

go_mockgen = macro(
    implementation = _impl,
    attrs = {
        "importpath": attr.string(mandatory = True),
        "out": attr.string(mandatory = True, configurable = False),
        "prefix": attr.string(mandatory = True),
        "src": attr.label(mandatory = True),
    },
)

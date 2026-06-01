"""`msgp` marshaller generation with optional in-pipeline patches, diff-tested against the source tree.

Each call runs `msgp` on `src` to produce `out` with any `patches` applied in order, and `out_test` if specified.
"""

load("@bazel_lib//lib:run_binary.bzl", "run_binary")
load("@bazel_lib//lib:write_source_files.bzl", "write_source_files")
load("@bazel_skylib//rules:select_file.bzl", "select_file")

def _impl(name, directives, io, out, out_test, patch_strip, patches, src, src_file, visibility):
    if src_file:
        select_file(name = "{}_src".format(name), srcs = src, subpath = src_file)
        src = ":{}_src".format(name)
    files = {out: "{}_msgp/{}".format(name, out)}
    if out_test:
        files[out_test] = "{}_msgp/{}".format(name, out_test)
    run_binary(
        name = "{}_msgp".format(name),
        args = [
            "-file=$(execpath {})".format(src),
            "-io={}".format(io),
            "-o=$(execpath :{})".format(files[out]),
            "-tests={}".format(out_test in files),
        ] + ["-d='{}'".format(d) for d in directives],
        outs = files.values(),
        srcs = [src],
        tool = "@com_github_tinylib_msgp//:msgp",
    )
    if patches:
        orig = files[out]
        files[out] = "{}_patched/{}".format(name, out)
        run_binary(
            name = "{}_patched".format(name),
            args = [
                "-p{}".format(patch_strip),
                "$(execpath :{})".format(orig),
                "$@",
            ] + ["$(execpath {})".format(p) for p in patches],
            outs = [files[out]],
            srcs = [":{}".format(orig)] + patches,
            tool = "//bazel/rules/patch",
        )
    native.exports_files(files.keys(), visibility)
    write_source_files(name = name, files = files, check_that_out_file_exists = False, visibility = ["//visibility:public"])

go_msgp = macro(
    implementation = _impl,
    attrs = {
        "directives": attr.string_list(configurable = False),
        "io": attr.bool(configurable = False),
        "out": attr.string(mandatory = True, configurable = False),
        "out_test": attr.string(configurable = False),
        "patch_strip": attr.int(configurable = False),
        "patches": attr.label_list(configurable = False),
        "src": attr.label(mandatory = True, allow_single_file = True, configurable = False),
        "src_file": attr.string(configurable = False),
    },
)

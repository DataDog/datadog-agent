load("@bazel_lib//lib:run_binary.bzl", "run_binary")
load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")

def _impl(name, build_tags, linecomment, output, src, trimprefix, types, visibility):
    args = []
    if build_tags:
        args.append("-tags={}".format(" ".join(build_tags)))
    args.append("-type={}".format(",".join(types)))
    if linecomment:
        args.append("-linecomment")
    if trimprefix:
        args.append("-trimprefix={}".format(trimprefix))
    args.append("-output=$@")
    args.append(native.package_name())
    gen = "{}_gen".format(name)
    run_binary(
        name = gen,
        args = args,
        outs = ["{}/{}".format(name, output)],
        srcs = [src],
        tool = "@go_stringer",
    )
    native.exports_files([output], visibility)
    write_source_file(name = name, in_file = ":{}".format(gen), out_file = output, check_that_out_file_exists = False)

go_stringer = macro(
    implementation = _impl,
    attrs = {
        "build_tags": attr.string_list(configurable = False),
        "linecomment": attr.bool(configurable = False),
        "output": attr.string(mandatory = True, configurable = False),
        "src": attr.label(mandatory = True, allow_single_file = [".go"], configurable = False),
        "trimprefix": attr.string(configurable = False),
        "types": attr.string_list(mandatory = True, configurable = False),
    },
)

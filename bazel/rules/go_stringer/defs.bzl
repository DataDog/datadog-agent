load("@bazel_lib//lib:run_binary.bzl", "run_binary")
load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")

def _impl(name, linecomment, mod, output, src, tags, trimprefix, types, **kwargs):
    args = []
    if tags:
        args += ["-tags", " ".join(tags)]
    args += ["-type", ",".join(types)]
    if linecomment:
        args.append("-linecomment")
    if trimprefix:
        args += ["-trimprefix", trimprefix]
    if output != types[0].lower() + "_string.go":
        args += ["-output", output]
    runner_name = "{}_run".format(name)
    run_binary(
        name = runner_name,
        args = args,
        env = dict(
            GO = "$(execpath @rules_go//go)",
            OUT = "$@",
            SRC = "$(execpath {})".format(src),
            STRINGER = "$(execpath @org_golang_x_tools//cmd/stringer)",
            STRINGER_OUT = output,
        ),
        srcs = ["@org_golang_x_tools//cmd/stringer", "@rules_go//go", mod, src],
        tool = "//bazel/rules/go_stringer:runner",
        outs = ["{}.gen".format(output)],
    )
    native.exports_files([output], **kwargs)
    write_source_file(name = name, in_file = runner_name, out_file = output, check_that_out_file_exists = False)

go_stringer = macro(
    implementation = _impl,
    attrs = {
        "linecomment": attr.bool(default = False, configurable = False),
        "mod": attr.label(mandatory = True, configurable = False),
        "output": attr.string(mandatory = True, configurable = False),
        "src": attr.label(mandatory = True, configurable = False),
        "tags": attr.string_list(configurable = False),
        "trimprefix": attr.string(configurable = False),
        "types": attr.string_list(mandatory = True, configurable = False),
    },
)

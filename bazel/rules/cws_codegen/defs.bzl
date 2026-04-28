"""CWS code generation macros.

These macros wrap the per-generator binaries under //pkg/security/generators/...
into hermetic Bazel actions whose outputs are written back into the source tree
via `write_source_file`. They follow the same shape as
//bazel/rules/go_stringer:defs.bzl.
"""

load("@bazel_lib//lib:run_binary.bzl", "run_binary")
load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")

def _operators_impl(name, output, visibility):
    gen = "{}_gen".format(name)
    run_binary(
        name = gen,
        args = ["-output=$(execpath {}/{})".format(name, output)],
        outs = ["{}/{}".format(name, output)],
        tool = "//pkg/security/generators/operators",
    )
    native.exports_files([output], visibility)
    write_source_file(
        name = name,
        in_file = ":{}".format(gen),
        out_file = output,
        check_that_out_file_exists = False,
    )

operators = macro(
    implementation = _operators_impl,
    doc = "Generate eval operators using //pkg/security/generators/operators and write the result back to the source tree.",
    attrs = {
        "output": attr.string(mandatory = True, configurable = False, doc = "Name of the generated .go file (e.g. eval_operators.go)."),
    },
)

"""CWS code generation macros.

These macros wrap the per-generator binaries under //pkg/security/generators/...
into hermetic Bazel actions whose outputs are written back into the source tree
via `write_source_file`. They follow the same shape as
//bazel/rules/go_stringer:defs.bzl.
"""

load("@bazel_lib//lib:run_binary.bzl", "run_binary")
load("@bazel_lib//lib:write_source_files.bzl", "write_source_file", "write_source_files")

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

def _bpf_maps_generator_impl(name, header, output, package_name, visibility):
    gen = "{}_gen".format(name)
    run_binary(
        name = gen,
        srcs = [header],
        args = [
            "-runtime-path=$(execpath {})".format(header),
            "-output=$(execpath {}/{})".format(name, output),
            "-pkg-name={}".format(package_name),
        ],
        outs = ["{}/{}".format(name, output)],
        tool = "//pkg/security/secl/model/bpf_maps_generator",
    )
    native.exports_files([output], visibility)
    write_source_file(
        name = name,
        in_file = ":{}".format(gen),
        out_file = output,
        check_that_out_file_exists = False,
    )

bpf_maps_generator = macro(
    implementation = _bpf_maps_generator_impl,
    doc = "Scan a BPF maps header file and generate the matching []string of map names via //pkg/security/secl/model/bpf_maps_generator.",
    attrs = {
        "header": attr.label(mandatory = True, configurable = False, allow_single_file = [".h"], doc = "Label of the BPF maps header file to scan (e.g. //pkg/security/ebpf/c/include:maps.h)."),
        "output": attr.string(mandatory = True, configurable = False, doc = "Name of the generated .go file (e.g. consts_map_names_linux.go)."),
        "package_name": attr.string(mandatory = True, configurable = False, doc = "Go package name to write into the generated file."),
    },
)

# `accessors` is intentionally a legacy macro (regular def): it needs
# `write_source_files` to bundle three .go writebacks under one umbrella target
# (so `bazel run //...:<name>` works), and that helper calls `native.glob`,
# which symbolic macros forbid. The single-output siblings above (operators,
# bpf_maps_generator) stay symbolic — only this multi-output case requires the
# legacy form. See ABLD-420.
def accessors(name, tags, model, types_file, output, field_handlers, field_accessors_output, doc_output, package_path, srcs = [], visibility = None):
    gen = "{}_gen".format(name)
    out_dir = name
    run_binary(
        name = gen,
        srcs = [model, types_file] + srcs,
        args = [
            "-tags={}".format(tags),
            "-input=$(execpath {})".format(model),
            "-types-file=$(execpath {})".format(types_file),
            "-package={}".format(package_path),
            "-module={}".format(package_path.rsplit("/", 1)[-1]),
            "-output=$(execpath {}/{})".format(out_dir, output),
            "-field-handlers=$(execpath {}/{})".format(out_dir, field_handlers),
            "-field-accessors-output=$(execpath {}/{})".format(out_dir, field_accessors_output),
            "-doc=$(execpath {}/{})".format(out_dir, doc_output),
        ],
        outs = [
            "{}/{}".format(out_dir, output),
            "{}/{}".format(out_dir, field_handlers),
            "{}/{}".format(out_dir, field_accessors_output),
            "{}/{}".format(out_dir, doc_output),
        ],
        tool = "//pkg/security/generators/accessors",
        visibility = visibility,
    )

    # Single umbrella target so `bazel run //...:<name>` refreshes all three
    # Go outputs at once. The JSON doc file is consumed across packages by
    # docs/cloud-workload-security/BUILD.bazel via the same gen subdir.
    write_source_files(
        name = name,
        files = {
            output: ":{}/{}".format(out_dir, output),
            field_handlers: ":{}/{}".format(out_dir, field_handlers),
            field_accessors_output: ":{}/{}".format(out_dir, field_accessors_output),
        },
    )

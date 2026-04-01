"""Macro for generating runtime compilation bundles (flattened .c + integrity hash .go).

Each bundle produces a flattened .c file (build artifact, not committed) and an
integrity hash .go file.  When out_go_file is set, a write_source_file target
is emitted so ``bazel test //<pkg>:<name>_verify_test`` checks the committed
hash file and ``bazel run //<pkg>:<name>_verify`` updates it.
"""

load("@bazel_lib//lib:run_binary.bzl", "run_binary")
load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")

def _runtime_compilation_bundle_impl(name, visibility, src_c, out_name, include_dirs, header_deps, out_go_file):
    flat_name = "{}_flat".format(name)
    run_binary(
        name = flat_name,
        visibility = visibility,
        tool = "//pkg/ebpf:include_headers",
        srcs = [src_c] + header_deps,
        outs = ["{}/{}.c".format(name, out_name)],
        args = [
            "$(location {})".format(src_c),
            "$@",
        ] + include_dirs,
    )

    raw_name = "{}_raw".format(name)
    run_binary(
        name = raw_name,
        tool = "//pkg/ebpf/bytecode/runtime:integrity",
        srcs = [":{}".format(flat_name)],
        outs = ["{}/{}_raw.go".format(name, out_name)],
        args = [
            "$(location :{})".format(flat_name),
            "$@",
            "runtime",
        ],
    )

    # The integrity tool uses runtime.Caller(0) to detect whether its output
    # lands in the same package as itself; if paths differ it emits a
    # redundant `import "…/runtime"` and qualifies newAsset as runtime.newAsset.
    # Inside a Bazel sandbox the paths never match, so strip the bogus import
    # and prefix to keep the output identical to the ninja/go-generate build.
    gen_name = "{}_gen".format(name)
    native.genrule(
        name = gen_name,
        srcs = [":{}".format(raw_name)],
        outs = ["{}/{}.go".format(name, out_name)],
        cmd = "sed -e '/^import \"github.com\\/DataDog\\/datadog-agent\\/pkg\\/ebpf\\/bytecode\\/runtime\"/d' -e 's/runtime\\.newAsset/newAsset/g' $< > $@",
        visibility = visibility,
    )

    if out_go_file:
        write_source_file(
            name = "{}_verify".format(name),
            visibility = visibility,
            in_file = ":{}".format(gen_name),
            out_file = out_go_file,
            check_that_out_file_exists = False,
            # Out files are .gitignored; tag as manual so bazel test //... skips them.
            tags = ["manual"],
        )

runtime_compilation_bundle = macro(
    doc = "Chains include_headers and integrity to produce a runtime compilation bundle.",
    attrs = {
        "src_c": attr.label(mandatory = True, allow_single_file = [".c"], configurable = False),
        "out_name": attr.string(mandatory = True, configurable = False),
        "include_dirs": attr.string_list(mandatory = True, configurable = False),
        "header_deps": attr.label_list(mandatory = True, configurable = False),
        "out_go_file": attr.label(mandatory = False, allow_single_file = [".go"], configurable = False),
    },
    implementation = _runtime_compilation_bundle_impl,
)

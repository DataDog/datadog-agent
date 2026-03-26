"""Macro for generating runtime compilation bundles (flattened .c + integrity hash .go)."""

load("@bazel_lib//lib:run_binary.bzl", "run_binary")

def _runtime_compilation_bundle_impl(name, visibility, src_c, out_name, include_dirs, header_deps):
    flat_name = "{}_flat".format(name)
    run_binary(
        name = flat_name,
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
    native.genrule(
        name = name,
        srcs = [":{}".format(raw_name)],
        outs = ["{}/{}.go".format(name, out_name)],
        cmd = "sed -e '/^import \"github.com\\/DataDog\\/datadog-agent\\/pkg\\/ebpf\\/bytecode\\/runtime\"/d' -e 's/runtime\\.newAsset/newAsset/g' $< > $@",
        visibility = visibility,
    )

runtime_compilation_bundle = macro(
    doc = "Chains include_headers and integrity to produce a runtime compilation bundle.",
    attrs = {
        "src_c": attr.label(mandatory = True, allow_single_file = [".c"], configurable = False),
        "out_name": attr.string(mandatory = True, configurable = False),
        "include_dirs": attr.string_list(mandatory = True, configurable = False),
        "header_deps": attr.label_list(mandatory = True, configurable = False),
    },
    implementation = _runtime_compilation_bundle_impl,
)

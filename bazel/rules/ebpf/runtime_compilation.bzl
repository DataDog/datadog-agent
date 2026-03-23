"""Macro for generating runtime compilation bundles (flattened .c + integrity hash .go)."""

load("@bazel_lib//lib:run_binary.bzl", "run_binary")

def runtime_compilation_bundle(name, src_c, out_name, include_dirs, header_deps):
    """Chains include_headers and integrity to produce a runtime compilation bundle.

    Args:
        name: target name (also used as the integrity output target)
        src_c: label of the source .c file
        out_name: base name for outputs (e.g. "usm" -> usm.c + usm.go)
        include_dirs: list of repo-relative directory strings for header search
        header_deps: list of labels whose files must be available for include resolution
    """
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

    run_binary(
        name = name,
        tool = "//pkg/ebpf/bytecode/runtime:integrity",
        srcs = [":{}".format(flat_name)],
        outs = ["{}/{}.go".format(name, out_name)],
        args = [
            "$(location :{})".format(flat_name),
            "$@",
            "runtime",
        ],
    )

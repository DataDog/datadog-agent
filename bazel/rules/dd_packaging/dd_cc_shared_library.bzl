"""dd_cc_shared_library — packaging-aware wrapper around cc_shared_library.

Automatically applies rpath patching, then forwards CcSharedLibraryInfo + CcInfo
from the wrapped target unchanged, so the wrapper is transparent to the CC build
graph.
"""

load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")
load("//bazel/rules/rewrite_rpath:rewrite_rpath.bzl", "rewrite_rpath")

def _dd_cc_shared_library_rule_impl(ctx):
    patched_file = ctx.file.patched
    return [
        ctx.attr.input[CcSharedLibraryInfo],
        DefaultInfo(files = depset([patched_file])),
    ]

_dd_cc_shared_library_rule = rule(
    implementation = _dd_cc_shared_library_rule_impl,
    doc = """Private inner rule backing the dd_cc_shared_library macro.

    Forwards CcSharedLibraryInfo + CcInfo from `input` unchanged and exposes
    the rpath-patched file via DefaultInfo.
    """,
    attrs = {
        "input": attr.label(
            doc = "The cc_shared_library to wrap.",
            mandatory = True,
            providers = [CcSharedLibraryInfo],
        ),
        "patched": attr.label(
            doc = "The rpath-patched output produced internally by the macro.",
            mandatory = True,
            allow_single_file = True,
        ),
    },
)

def _dd_cc_shared_library_impl(name, input, **kwargs):
    patched_name = "{}_patched".format(name)
    rewrite_rpath(
        name = patched_name,
        inputs = [input],
    )
    _dd_cc_shared_library_rule(
        name = name,
        input = input,
        patched = ":{}".format(patched_name),
        **kwargs,
    )

dd_cc_shared_library = macro(
    doc = """Packaging-aware wrapper around cc_shared_library.

    Automatically applies rpath patching via rewrite_rpath, then forwards
    CcSharedLibraryInfo and CcInfo from `input` unchanged so the wrapper is
    transparent to the CC
    build graph (usable in dynamic_deps of downstream targets).

    To add a future post-processing step (e.g. strip debug symbols), extend
    this macro: one change here applies to every wrapped library automatically.

    Args:
        name:  Name of the public target.
        input: Label of the cc_shared_library (or foreign_cc_shared_wrapper) to wrap.
    """,
    attrs = {
        "input": attr.label(
            doc = "The cc_shared_library (or foreign_cc_shared_wrapper) to wrap.",
            mandatory = True,
            configurable = False,
        ),
    },
    implementation = _dd_cc_shared_library_impl,
)

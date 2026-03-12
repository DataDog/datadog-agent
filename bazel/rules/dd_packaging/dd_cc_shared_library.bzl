"""dd_cc_shared_library — packaging-aware wrapper around cc_shared_library."""

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
    attrs = {
        "input": attr.label(
            mandatory = True,
            providers = [CcSharedLibraryInfo],
        ),
        "patched": attr.label(
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
    attrs = {
        "input": attr.label(
            mandatory = True,
            configurable = False,
        ),
    },
    implementation = _dd_cc_shared_library_impl,
)

"""A narrow wrapper for building one dependency on a different platform."""

load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")

def _platform_transition_impl(_settings, attr):
    return {"//command_line_option:platforms": str(attr.platform)}

_platform_transition = transition(
    implementation = _platform_transition_impl,
    inputs = [],
    outputs = ["//command_line_option:platforms"],
)

def _platform_transition_wrapper_impl(ctx):
    # A Starlark transition makes a label attribute a list even when the
    # transition has exactly one output configuration.
    target = ctx.attr.actual[0]
    return [
        target[DefaultInfo],
        target[CcInfo],
    ]

platform_transition_wrapper = rule(
    implementation = _platform_transition_wrapper_impl,
    attrs = {
        "actual": attr.label(
            cfg = _platform_transition,
            mandatory = True,
            providers = [CcInfo],
        ),
        "platform": attr.label(mandatory = True),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
    },
)

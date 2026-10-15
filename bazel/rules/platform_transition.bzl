"""A narrow wrapper for building one dependency on a different platform."""

load("@rules_cc//cc:defs.bzl", "cc_common")
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
    cc_info = target[CcInfo]

    if ctx.attr.forward_own_linker_inputs_only:
        # A shared library already incorporates the link requirements used to
        # build it. Forwarding those transitive inputs across an ABI transition
        # can leak producer-specific flags into the consumer. For example, an
        # MSVC Rust cdylib carries `advapi32.lib`-style Rust stdlib flags that a
        # MinGW cgo host neither needs nor understands. Retain the DLL/import
        # library owned by the transitioned target and its compilation context.
        own_linker_inputs = [
            linker_input
            for linker_input in cc_info.linking_context.linker_inputs.to_list()
            if linker_input.owner == target.label
        ]
        if not own_linker_inputs:
            fail("No linker input owned by {} found".format(target.label))
        cc_info = CcInfo(
            compilation_context = cc_info.compilation_context,
            linking_context = cc_common.create_linking_context(
                linker_inputs = depset(own_linker_inputs),
            ),
        )

    return [
        target[DefaultInfo],
        cc_info,
    ]

platform_transition_wrapper = rule(
    implementation = _platform_transition_wrapper_impl,
    attrs = {
        "actual": attr.label(
            cfg = _platform_transition,
            mandatory = True,
            providers = [CcInfo],
        ),
        "forward_own_linker_inputs_only": attr.bool(
            default = False,
            doc = "Forward only linker inputs owned by the transitioned target.",
        ),
        "platform": attr.label(mandatory = True),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
    },
)

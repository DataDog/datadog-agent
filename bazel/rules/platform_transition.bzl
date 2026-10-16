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

def _platform_transition_file_impl(ctx):
    # Forward one regular file from the transitioned target without copying it.
    # This is useful for runtime artifacts produced in a split ABI configuration:
    # consumers see a normal single-file target while the file remains owned by
    # the configuration that built it.
    target = ctx.attr.actual[0]
    matches = [
        file
        for file in target[DefaultInfo].files.to_list()
        if file.basename == ctx.attr.basename
    ]
    if len(matches) != 1:
        fail("Expected one file named {} from {}, found: {}".format(
            ctx.attr.basename,
            target.label,
            [file.path for file in matches],
        ))
    return [DefaultInfo(
        files = depset(matches),
        runfiles = ctx.runfiles(files = matches),
    )]

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

platform_transition_file = rule(
    implementation = _platform_transition_file_impl,
    attrs = {
        "actual": attr.label(
            cfg = _platform_transition,
            mandatory = True,
        ),
        "basename": attr.string(mandatory = True),
        "platform": attr.label(mandatory = True),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
    },
)

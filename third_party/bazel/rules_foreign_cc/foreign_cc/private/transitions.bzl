"""This file contains rules for configuration transitions"""

load("@rules_cc//cc:defs.bzl", "CcInfo")
load("//foreign_cc:providers.bzl", "ForeignCcDepsInfo")

def _extra_toolchains_transition_impl(settings, attrs):
    return {"//command_line_option:extra_toolchains": [attrs.extra_toolchain] + settings["//command_line_option:extra_toolchains"]}

_extra_toolchains_transition = transition(
    implementation = _extra_toolchains_transition_impl,
    inputs = ["//command_line_option:extra_toolchains"],
    outputs = ["//command_line_option:extra_toolchains"],
)

def _extra_toolchains_transitioned_foreign_cc_target_impl(ctx):
    # Return the providers from the transitioned foreign_cc target
    return [
        ctx.attr.target[DefaultInfo],
        ctx.attr.target[CcInfo],
        ctx.attr.target[ForeignCcDepsInfo],
        ctx.attr.target[OutputGroupInfo],
    ]

extra_toolchains_transitioned_foreign_cc_target = rule(
    doc = "A rule for adding an extra toolchain to consider when building the given target",
    implementation = _extra_toolchains_transitioned_foreign_cc_target_impl,
    cfg = _extra_toolchains_transition,
    attrs = {
        # This attr is singular to make it selectable when used for add make toolchain variant.
        "extra_toolchain": attr.string(
            doc = "Additional toolchain to consider. Note, this is singular.",
            mandatory = True,
        ),
        "target": attr.label(
            doc = "The target to build after considering the extra toolchains",
            providers = [ForeignCcDepsInfo],
            mandatory = True,
        ),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
    },
)

def foreign_cc_rule_variant(name, rule, toolchain, **kwargs):
    """ Wrapper macro around foreign cc rules to force usage of the given  toolchain.

    Args:
        name: The target name
        rule: The foreign cc rule to instantiate, e.g. configure_make
        toolchain: The desired make variant toolchain to use, e.g. @rules_foreign_cc//toolchains:preinstalled_nmake_toolchain
        **kwargs: Remaining keyword arguments
    """

    foreign_cc_rule_target_name = name + "_"

    tags = kwargs.pop("tags", [])
    visibility = kwargs.pop("visibility", [])

    rule(
        name = foreign_cc_rule_target_name,
        tags = tags + ["manual"],
        visibility = visibility,
        **kwargs
    )

    extra_toolchains_transitioned_foreign_cc_target(
        name = name,
        extra_toolchain = toolchain,
        target = foreign_cc_rule_target_name,
        tags = tags,
        visibility = visibility,
    )

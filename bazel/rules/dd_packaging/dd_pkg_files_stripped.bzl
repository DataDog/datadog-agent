"""Helper for referencing a dd_strip_debug target's split outputs from pkg_files.

dd_strip_debug's DefaultInfo intentionally stays unstripped (see
//bazel/rules/dd_strip:dd_strip.bzl), so a plain `pkg_files(srcs = [":foo"])`
would package the unstripped binary. dd_pkg_files_stripped creates two small
sibling targets, "<name>.stripped" and "<name>.debug", that expose a
dd_strip_debug target's OutputGroupInfo(stripped = ...)/OutputGroupInfo(debug
= ...) outputs through DefaultInfo, so they can be referenced directly as
pkg_files srcs, e.g.:

    dd_strip_debug(name = "agent", input = ":agent_unstripped")
    dd_pkg_files_stripped(name = "agent")
    pkg_files(srcs = ["//cmd/agent:agent.stripped"], ...)
"""

def _dd_output_group_file_impl(ctx):
    group = getattr(ctx.attr.dep[OutputGroupInfo], ctx.attr.group)
    return [DefaultInfo(files = group)]

_dd_output_group_file = rule(
    implementation = _dd_output_group_file_impl,
    doc = "Re-exposes one named OutputGroupInfo group of `dep` as this target's DefaultInfo.",
    attrs = {
        "dep": attr.label(
            doc = "A target providing OutputGroupInfo, typically a dd_strip_debug target.",
            providers = [OutputGroupInfo],
            mandatory = True,
        ),
        "group": attr.string(
            doc = "The OutputGroupInfo group to expose: 'stripped' or 'debug'.",
            mandatory = True,
            values = ["stripped", "debug"],
        ),
    },
)

def dd_pkg_files_stripped(name, dep = None, visibility = None):
    """Creates "<name>.stripped" and "<name>.debug" sibling labels for a dd_strip_debug target.

    Args:
      name: the base name used for the sibling labels ("<name>.stripped", "<name>.debug").
      dep: the dd_strip_debug target to read from. Defaults to ":<name>" (i.e. call this
           right after `dd_strip_debug(name = name, ...)`).
      visibility: visibility applied to both sibling targets.
    """
    dep = dep or (":" + name)
    _dd_output_group_file(
        name = name + ".stripped",
        dep = dep,
        group = "stripped",
        visibility = visibility,
    )
    _dd_output_group_file(
        name = name + ".debug",
        dep = dep,
        group = "debug",
        visibility = visibility,
    )

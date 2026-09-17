load("@rules_go//go:def.bzl", "go_binary")

# Bazel's default --strip=sometimes strips rules_go binaries in fastbuild mode.
# These fixture binaries deliberately exercise stripped vs. unstripped Go ELF
# handling, so build each go_binary with --strip=never and let the go linker
# flags on the individual fixture targets decide whether symbols are present.
def _strip_never_transition_impl(_settings, _attr):
    return {"//command_line_option:strip": "never"}

_strip_never_transition = transition(
    implementation = _strip_never_transition_impl,
    inputs = [],
    outputs = ["//command_line_option:strip"],
)

def _go_labels_fixture_impl(ctx):
    return [DefaultInfo(
        files = depset([ctx.file.src]),
        runfiles = ctx.runfiles(files = [ctx.file.src]),
    )]

_go_labels_fixture = rule(
    implementation = _go_labels_fixture_impl,
    attrs = {
        "src": attr.label(
            allow_single_file = True,
            cfg = _strip_never_transition,
            mandatory = True,
        ),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
    },
)

def go_labels_fixture(name, tags = None, **kwargs):
    """Builds a Go binary fixture with Bazel stripping disabled.

    Args:
        name: Name of the fixture target consumed by tests.
        tags: Additional tags for the private raw go_binary target. The raw
            target is always tagged manual so broad target expansion only sees
            the fixture wrapper.
        **kwargs: Attributes forwarded to rules_go's go_binary.
    """
    raw_name = name + "_raw"
    target_compatible_with = kwargs.get("target_compatible_with")
    go_binary(
        name = raw_name,
        tags = (tags or []) + ["manual"],
        **kwargs
    )

    fixture_kwargs = {}
    if target_compatible_with != None:
        fixture_kwargs["target_compatible_with"] = target_compatible_with
    _go_labels_fixture(
        name = name,
        src = ":" + raw_name,
        **fixture_kwargs
    )

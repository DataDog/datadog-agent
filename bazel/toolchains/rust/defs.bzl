"""Wrapper rules for size-optimized Rust binaries.

This module provides the size_optimized_rust_binary rule, which applies
size optimization transitions to Rust binaries when enabled via the
rust_size_optimized build setting.
"""

load("@rules_rust//rust:defs.bzl", "rust_binary")
load("//bazel/toolchains/rust:transitions.bzl", "size_optimized_transition")

def _size_optimized_rust_binary_impl(ctx):
    """Implementation for size_optimized_rust_binary wrapper rule.

    This rule simply forwards the actual rust_binary target built with
    the size optimization transition applied.

    Args:
        ctx: Rule context

    Returns:
        Providers from the wrapped rust_binary target
    """
    # Transitions produce a list of targets (one per configuration)
    # In our case, we always have exactly one configuration, so we take the first element
    actual = ctx.attr.actual[0] if type(ctx.attr.actual) == type([]) else ctx.attr.actual

    # Create a symlink to the actual executable
    output_file = ctx.actions.declare_file(ctx.label.name)
    ctx.actions.symlink(
        output = output_file,
        target_file = actual[DefaultInfo].files_to_run.executable,
        is_executable = True,
    )

    return [
        DefaultInfo(
            files = depset([output_file]),
            runfiles = actual[DefaultInfo].default_runfiles,
            executable = output_file,
        ),
        actual[OutputGroupInfo],
    ]

_size_optimized_rust_binary = rule(
    implementation = _size_optimized_rust_binary_impl,
    attrs = {
        "actual": attr.label(
            doc = "The actual rust_binary target to wrap with size optimization",
            cfg = size_optimized_transition,
            mandatory = True,
        ),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
            doc = "Allowlist for Bazel configuration transitions",
        ),
    },
    doc = """Internal rule that applies size optimization transition to a rust_binary.

    This rule wraps a rust_binary target and applies the size_optimized_transition
    to it, which modifies rules_rust settings to enable aggressive size optimizations
    when the rust_size_optimized flag is enabled.
    """,
    executable = True,
)

def size_optimized_rust_binary(name, **kwargs):
    """Creates a Rust binary with size optimization transition support.

    This macro creates two targets:
    1. A private rust_binary target (name + "_actual")
    2. A wrapper target (name) that applies size optimization transition

    When rust_size_optimized=true (e.g., via --config=release), the binary
    is built with aggressive size optimizations:
    - LTO (Link-Time Optimization) with proper linker configuration
    - Optimization level 'z' (optimize for size)
    - Single codegen unit for maximum optimization
    - Symbol stripping
    - Panic abort strategy

    Args:
        name: Name of the target
        **kwargs: All arguments forwarded to rust_binary
            Common arguments:
            - srcs: Source files
            - deps: Dependencies
            - edition: Rust edition (e.g., "2021")
            - visibility: Target visibility
            - tags: Target tags
            - And all other rust_binary attributes

    Example:
        ```python
        size_optimized_rust_binary(
            name = "my_binary",
            srcs = ["main.rs"],
            deps = ["//some:dep"],
            edition = "2021",
        )
        ```

        Build with size optimization:
        ```bash
        bazel build //:my_binary --config=release
        ```

        Build without optimization (faster compilation):
        ```bash
        bazel build //:my_binary
        ```
    """
    actual_name = name + "_actual"

    # Extract visibility and tags to handle them separately
    visibility = kwargs.pop("visibility", None)
    tags = kwargs.get("tags", [])

    # Create the actual rust_binary target with private visibility
    # The internal target should not be directly accessible
    rust_binary(
        name = actual_name,
        visibility = ["//visibility:private"],
        **kwargs
    )

    # Create the wrapper with transition, forwarding visibility and tags
    _size_optimized_rust_binary(
        name = name,
        actual = ":" + actual_name,
        visibility = visibility,
        tags = tags,
    )

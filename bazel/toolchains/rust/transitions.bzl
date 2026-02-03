"""Bazel transitions for Rust size optimization.

This module implements configuration transitions that apply size optimization
flags specifically to Rust binaries without affecting the entire build graph.
"""

def _size_optimized_transition_impl(settings, _attr):
    """Transition implementation for size-optimized Rust binaries.

    This transition modifies rules_rust build settings to apply aggressive
    size optimization flags (LTO, opt-level=z, strip, etc.) when the
    rust_size_optimized flag is enabled.

    Args:
        settings: Dict of current build setting values
        _attr: Attributes of the rule invoking the transition (unused)

    Returns:
        Dict mapping build settings to their new values
    """

    # Read custom build settings
    size_optimized = settings["//bazel/toolchains/rust:rust_size_optimized"]

    # If size optimization is not enabled, return unchanged settings
    if not size_optimized:
        return {}

    # Read optimization parameter settings
    opt_level = settings["//bazel/toolchains/rust:rust_opt_level"]
    codegen_units = settings["//bazel/toolchains/rust:rust_codegen_units"]
    panic = settings["//bazel/toolchains/rust:rust_panic"]
    strip = settings["//bazel/toolchains/rust:rust_strip"]
    lto = settings["//bazel/toolchains/rust:rust_lto"]

    # Build rustc flags for size optimization
    rustc_flags = [
        "-Copt-level={}".format(opt_level),
        "-Ccodegen-units={}".format(codegen_units),
        "-Cpanic={}".format(panic),
        "-Cstrip={}".format(strip),
    ]

    # Return modified rules_rust settings
    # Note: LTO is handled separately via rules_rust's dedicated setting,
    # which properly configures the linker (lld/mold) with LTO support
    return {
        "@rules_rust//rust/settings:lto": lto,
        "@rules_rust//rust/settings:extra_rustc_flags": rustc_flags,
    }

size_optimized_transition = transition(
    implementation = _size_optimized_transition_impl,
    inputs = [
        "//bazel/toolchains/rust:rust_size_optimized",
        "//bazel/toolchains/rust:rust_opt_level",
        "//bazel/toolchains/rust:rust_codegen_units",
        "//bazel/toolchains/rust:rust_panic",
        "//bazel/toolchains/rust:rust_strip",
        "//bazel/toolchains/rust:rust_lto",
    ],
    outputs = [
        "@rules_rust//rust/settings:lto",
        "@rules_rust//rust/settings:extra_rustc_flags",
    ],
)

"""Incoming transition for rules_cc's cc_toolchain, see bazel/patches/rules_cc-*.patch.

The cc toolchain is configured in its consumer's configuration, so settings it never
reads (Go build tags, pure, linkmode, ...) give it, and everything it builds such as
hermetic-llvm runtimes, one configuration per distinct value.
"""

load("@rules_go//go/private/rules:transition.bzl", "non_go_tool_transition")  # buildifier: disable=bzl-visibility

reset_cc_toolchain_settings = non_go_tool_transition

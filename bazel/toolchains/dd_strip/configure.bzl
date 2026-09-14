"""macOS instantiation of the dd_strip toolchain.

macOS ships `strip` and `dsymutil` as part of Xcode's command-line tools, not
as part of any Bazel cc_toolchain, so there is nothing to source these paths
from — they are hardcoded absolute paths, the same way
//bazel/rules/rewrite_rpath hardcodes /usr/bin/otool for macOS rpath
rewriting. This intentionally avoids the auto-detecting
`make_toolchain_repository_rule` pattern used by
//bazel/toolchains/codesign: that generator also emits a `config_setting`
per tool (`have_<tool>`), and new config_settings require a separate design
review per .claude/rules/bazel.md — this toolchain doesn't need one.
"""

load(":toolchain.bzl", "DdStripToolchainInfo")

_STRIP_PATH = "/usr/bin/strip"
_DSYMUTIL_PATH = "/usr/bin/dsymutil"

def _dd_strip_macos_toolchain_impl(ctx):
    return [platform_common.ToolchainInfo(
        dd_strip_info = DdStripToolchainInfo(
            strip_path = _STRIP_PATH,
            objcopy_path = None,
            dsymutil_path = _DSYMUTIL_PATH,
            tool_files = depset(),
        ),
    )]

dd_strip_macos_toolchain = rule(
    implementation = _dd_strip_macos_toolchain_impl,
    doc = "Hardcodes the macOS strip/dsymutil paths for the dd_strip toolchain type.",
)

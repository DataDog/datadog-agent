"""dd_strip toolchain: strip/objcopy/dsymutil tools for debug-symbol splitting.

Type: //bazel/toolchains/dd_strip:toolchain_type

Toolchains:
- dd_strip_cc_toolchain — sources `strip`/`objcopy` from the resolved
  `cc_toolchain` (Linux and Windows/mingw both already declare these tools;
  see bazel/toolchains/gcc/toolchain.bzl and bazel/toolchains/mingw/toolchain.bzl).
- dd_strip_macos_toolchain (configure.bzl) — hardcodes /usr/bin/strip and
  /usr/bin/dsymutil, the same way //bazel/rules/rewrite_rpath hardcodes
  /usr/bin/otool for macOS. macOS ships neither tool as part of a Bazel
  cc_toolchain, so there is nothing to source them from.

All fields are plain path strings (not File objects) because that's what
`cc_common.get_tool_for_action`-style tool sourcing and hardcoded system
paths both produce. `tool_files` carries whatever File objects must be added
to action `inputs` for the path to resolve (empty for hardcoded system
paths, since those live outside the exec root entirely).
"""

load("@rules_cc//cc:defs.bzl", "cc_common")
load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cc_toolchain", "use_cc_toolchain")

DdStripToolchainInfo = provider(
    doc = "Paths to the strip/objcopy/dsymutil tools used to split debug symbols from a binary.",
    fields = {
        "strip_path": "string path to the `strip` executable, or None if unavailable.",
        "objcopy_path": "string path to the `objcopy` executable (Linux only), or None.",
        "dsymutil_path": "string path to `dsymutil` (macOS only), or None.",
        "tool_files": "depset of File that must be included as action inputs for the paths above to resolve.",
    },
)

def _dd_strip_cc_toolchain_impl(ctx):
    cc_toolchain = find_cc_toolchain(ctx)
    feature_configuration = cc_common.configure_features(
        ctx = ctx,
        cc_toolchain = cc_toolchain,
        requested_features = ctx.features,
        unsupported_features = ctx.disabled_features,
    )

    # Silence the unused-variable warning; kept for parity with other
    # cc_toolchain-sourced rules and in case a future tool needs
    # get_tool_for_action-style resolution instead of the plain executable
    # path fields below.
    _ = feature_configuration

    strip_path = cc_toolchain.strip_executable
    objcopy_path = cc_toolchain.objcopy_executable
    return [platform_common.ToolchainInfo(
        dd_strip_info = DdStripToolchainInfo(
            strip_path = strip_path if strip_path else None,
            objcopy_path = objcopy_path if objcopy_path else None,
            dsymutil_path = None,
            tool_files = cc_toolchain.all_files,
        ),
    )]

dd_strip_cc_toolchain = rule(
    implementation = _dd_strip_cc_toolchain_impl,
    doc = """Sources strip/objcopy paths from the resolved cc_toolchain.

    Used to register the dd_strip toolchain on Linux and Windows, where the
    cc_toolchain already wires up both tools (gcc-toolchain ships objcopy and
    strip; the mingw toolchain declares strip via tool_paths, and Windows has
    no split-DWARF story so objcopy is simply left unset there).""",
    attrs = CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain(),
    fragments = ["cpp"],
)

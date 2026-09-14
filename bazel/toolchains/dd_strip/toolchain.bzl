"""toolchain to wrap platform-specific debug-symbol strip/split drivers.

Type: //bazel/toolchains/dd_strip:dd_strip_toolchain_type

Toolchains:
- //bazel/toolchains/dd_strip:linux_toolchain (objcopy --only-keep-debug + strip,
  sourced from the exec platform's cc_toolchain)
- //bazel/toolchains/dd_strip:windows_toolchain (mingw strip; debug artifact is
  the unstripped original, no split DWARF)
- //bazel/toolchains/dd_strip:macos_toolchain (hardcoded /usr/bin/strip +
  /usr/bin/dsymutil, the way rewrite_rpath hardcodes /usr/bin/otool)
- //bazel/toolchains/dd_strip:missing_toolchain: fallback for exec platforms
  where no strip/split driver is available. dd_strip_debug treats this the
  same as DdStripInfo.excluded = True: it passes the original file through
  unmodified rather than failing the build.

The _toolchain rule only collects an already-built driver executable (per the
"toolchain rules must not create actions" convention); the per-OS driver
scripts that DO create actions (to embed cc_toolchain tool paths) live in
configure.bzl.
"""

DD_STRIP_TOOLCHAIN_TYPE = "//bazel/toolchains/dd_strip:dd_strip_toolchain_type"

def _dd_strip_toolchain_impl(ctx):
    return [
        platform_common.ToolchainInfo(
            available = True,
            driver = ctx.attr.driver[DefaultInfo].files_to_run,
            debug_is_directory = ctx.attr.debug_is_directory,
        ),
    ]

dd_strip_toolchain = rule(
    implementation = _dd_strip_toolchain_impl,
    doc = """Wraps a driver executable implementing the strip/split contract:

    driver <input-file> <stripped-output> <debug-output>

    On platforms where the debug artifact is a directory (macOS .dSYM
    bundles), debug_is_directory must be True so dd_strip_debug declares a
    directory output instead of a file.""",
    attrs = {
        "driver": attr.label(
            doc = "An executable accepting <input> <stripped-out> <debug-out> arguments.",
            cfg = "exec",
            executable = True,
            allow_files = True,
            mandatory = True,
        ),
        "debug_is_directory": attr.bool(
            doc = "Whether the driver's debug-out argument is a directory (macOS .dSYM) rather than a file.",
            default = False,
        ),
    },
)

def _dd_strip_missing_toolchain_impl(_ctx):
    return [
        platform_common.ToolchainInfo(
            available = False,
            driver = None,
            debug_is_directory = False,
        ),
    ]

dd_strip_missing_toolchain = rule(
    implementation = _dd_strip_missing_toolchain_impl,
    doc = """Fallback toolchain for exec platforms with no strip/split driver.

    dd_strip_debug detects available = False and passes the original file
    through unchanged instead of failing the build.""",
)

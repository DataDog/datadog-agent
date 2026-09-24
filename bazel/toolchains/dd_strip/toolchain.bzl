"""toolchain to wrap platform-specific debug-symbol strip/split drivers.

Type: //bazel/toolchains/dd_strip:dd_strip_toolchain_type

Toolchains:
- //bazel/toolchains/dd_strip:linux_toolchain (objcopy --only-keep-debug + strip,
  sourced from the exec platform's cc_toolchain)
- //bazel/toolchains/dd_strip:windows_toolchain (mingw strip; debug artifact is
  the unstripped original, no split DWARF)
- //bazel/toolchains/dd_strip:macos_toolchain (hardcoded /usr/bin/strip +
  /usr/bin/dsymutil, the way rewrite_rpath hardcodes /usr/bin/otool)
"""

def _dd_strip_toolchain_impl(ctx):
    return [
        platform_common.ToolchainInfo(
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

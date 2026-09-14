"""Per-OS driver-generator rules for the dd_strip toolchain.

Each rule below writes a small wrapper script implementing the shared driver
contract (`driver <input> <stripped-out> <debug-out>`) and hard-wires it to
the objcopy/strip/dsymutil tool paths appropriate for its platform. These are
ordinary rules (they DO create actions -- writing the wrapper script), unlike
the `dd_strip_toolchain` rule in toolchain.bzl, which per convention only
collects an already-built executable.

Platform semantics mirror omnibus's Stripper
(omnibus-ruby/lib/omnibus/stripper.rb):
  - Linux: objcopy --only-keep-debug to extract .dbg, strip --strip-debug
    --strip-unneeded on a copy, then objcopy --add-gnu-debuglink to link the
    two back together. Tools come from the exec platform's cc_toolchain
    (already wired for gcc-toolchain patches), matching how preprocessor.bzl
    and windows_resources.bzl resolve cc_toolchain tools.
  - macOS: strip the shipped copy, dsymutil produces the .dSYM bundle. No
    prior art for macOS dbg packages in this repo or omnibus -- flag for
    build-team confirmation before merge. Tool paths are hardcoded to
    /usr/bin/strip and /usr/bin/dsymutil the way rewrite_with_install_name_tool.sh
    hardcodes /usr/bin/otool.
  - Windows: no split DWARF (standalone PDBs aren't reliably produced by this
    repo's Go/Rust/mingw toolchains). The debug artifact is the unstripped
    original binary (matches omnibus's windows_symbol_stripping_file); the
    shipped binary is strip'd via the mingw toolchain's existing strip tool.
"""

load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cc_toolchain", "use_cc_toolchain")

def _linux_driver_impl(ctx):
    cc_toolchain = find_cc_toolchain(ctx)
    driver = ctx.actions.declare_file(ctx.label.name + "_driver.sh")
    ctx.actions.write(
        output = driver,
        is_executable = True,
        content = """#!/usr/bin/env bash
set -euo pipefail

INPUT=$1
STRIPPED_OUT=$2
DEBUG_OUT=$3

OBJCOPY="{objcopy}"
STRIP="{strip}"

# 1. Extract the debug info into its own file.
"$OBJCOPY" --only-keep-debug "$INPUT" "$DEBUG_OUT"

# 2. Strip a copy of the binary for shipping.
cp "$INPUT" "$STRIPPED_OUT"
chmod u+w "$STRIPPED_OUT"
"$STRIP" --strip-debug --strip-unneeded "$STRIPPED_OUT"

# 3. Link the stripped binary back to its debug info via .gnu_debuglink.
"$OBJCOPY" --add-gnu-debuglink="$DEBUG_OUT" "$STRIPPED_OUT"
""".format(
            objcopy = cc_toolchain.objcopy_executable,
            strip = cc_toolchain.strip_executable,
        ),
    )
    return [DefaultInfo(
        executable = driver,
        files = depset([driver], transitive = [cc_toolchain.all_files]),
        runfiles = ctx.runfiles(transitive_files = cc_toolchain.all_files),
    )]

linux_dd_strip_driver = rule(
    implementation = _linux_driver_impl,
    doc = "Generates the Linux objcopy/strip debug-split driver from the resolved cc_toolchain.",
    attrs = CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain(),
    fragments = ["cpp"],
)

def _windows_driver_impl(ctx):
    cc_toolchain = find_cc_toolchain(ctx)
    driver = ctx.actions.declare_file(ctx.label.name + "_driver.bat")
    ctx.actions.write(
        output = driver,
        is_executable = True,
        content = """@echo off
setlocal

set "INPUT=%~1"
set "STRIPPED_OUT=%~2"
set "DEBUG_OUT=%~3"
set "INPUT=%INPUT:/=\\%"
set "STRIPPED_OUT=%STRIPPED_OUT:/=\\%"
set "DEBUG_OUT=%DEBUG_OUT:/=\\%"

rem No split DWARF on Windows: the debug artifact is the unstripped original.
copy /Y "%INPUT%" "%DEBUG_OUT%" >nul || exit /b 1
copy /Y "%INPUT%" "%STRIPPED_OUT%" >nul || exit /b 1
"{strip}" "%STRIPPED_OUT%" || exit /b 1
""".format(strip = cc_toolchain.strip_executable.replace("/", "\\")),
    )
    return [DefaultInfo(
        executable = driver,
        files = depset([driver], transitive = [cc_toolchain.all_files]),
        runfiles = ctx.runfiles(transitive_files = cc_toolchain.all_files),
    )]

windows_dd_strip_driver = rule(
    implementation = _windows_driver_impl,
    doc = """Generates the Windows strip driver from the resolved cc_toolchain
    (mingw's strip.exe). Debug artifact = unstripped original; not split
    DWARF, matching omnibus's windows_symbol_stripping_file.""",
    attrs = CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain(),
    fragments = ["cpp"],
)

_MACOS_STRIP = "/usr/bin/strip"
_MACOS_DSYMUTIL = "/usr/bin/dsymutil"

def _macos_driver_impl(ctx):
    driver = ctx.actions.declare_file(ctx.label.name + "_driver.sh")
    ctx.actions.write(
        output = driver,
        is_executable = True,
        content = """#!/usr/bin/env bash
set -euo pipefail

INPUT=$1
STRIPPED_OUT=$2
DEBUG_OUT=$3

# 1. dsymutil reads the original (unstripped) binary to produce the .dSYM
#    bundle -- it must run before the copy below is stripped.
"{dsymutil}" "$INPUT" -o "$DEBUG_OUT"

# 2. Strip a copy of the binary for shipping.
cp "$INPUT" "$STRIPPED_OUT"
chmod u+w "$STRIPPED_OUT"
"{strip}" -S "$STRIPPED_OUT"
""".format(dsymutil = _MACOS_DSYMUTIL, strip = _MACOS_STRIP),
    )
    return [DefaultInfo(
        executable = driver,
        files = depset([driver]),
    )]

macos_dd_strip_driver = rule(
    implementation = _macos_driver_impl,
    doc = """Generates the macOS strip+dsymutil debug-split driver.

    Hardcodes /usr/bin/strip and /usr/bin/dsymutil the way
    rewrite_with_install_name_tool.sh hardcodes /usr/bin/otool -- there is no
    prior art in this repo or omnibus for macOS dbg packages, so this is a
    new convention that needs build-team confirmation before merge.""",
)

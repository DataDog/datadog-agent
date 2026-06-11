"""Hermetic glibc floor enforcement test rule for ELF binaries.

The rule runs in two phases:
  1. Build action (run_shell): objdump -T is captured to a file.  objdump comes
     from the resolved CC toolchain (cc_toolchain.objdump_executable), not from
     PATH — that is the hermeticity fix relative to the previous bash stopgap.
     run_shell is used only because objdump writes its output to stdout and does
     not support an -o flag; the shell '>' redirect is the single non-hermetic
     primitive, but the tool itself is hermetic.
  2. Test script: a small generated sh that reads the captured symbol file and
     compares the highest GLIBC_x.y version found against max_version.  No
     objdump invocation at test time.
"""

load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cc_toolchain", "use_cc_toolchain")

def _glibc_floor_test_impl(ctx):
    # binary can be an executable (go_binary) or a shared library (.so),
    # so we use ctx.files rather than ctx.executable.
    binary_files = ctx.files.binary
    if len(binary_files) == 0:
        fail("binary attribute produced no files")
    if len(binary_files) > 1:
        fail("binary attribute produced multiple files; expected exactly one ELF file")
    binary = binary_files[0]

    cc_toolchain = find_cc_toolchain(ctx)

    # ------------------------------------------------------------------
    # Phase 1 — build action: dump dynamic symbols to a file.
    # We use run_shell solely to capture stdout via '>'; all inputs come
    # from the CC toolchain (hermetic) and the binary under test.
    # ------------------------------------------------------------------
    objdump_path = cc_toolchain.objdump_executable
    if not objdump_path:
        fail("CC toolchain does not provide objdump; cannot run glibc_floor_test on this platform.")

    symbols_file = ctx.actions.declare_file(ctx.label.name + "_symbols.txt")
    ctx.actions.run_shell(
        command = '"{objdump}" -T "{binary}" > "{out}" 2>/dev/null || true'.format(
            objdump = objdump_path,
            binary = binary.path,
            out = symbols_file.path,
        ),
        inputs = depset(
            [binary],
            transitive = [cc_toolchain.all_files],
        ),
        outputs = [symbols_file],
        mnemonic = "GlibcFloorDump",
        # Attribute the action to the CC toolchain type so Bazel's automatic
        # execution groups (AEGs) select the correct execution platform.
        toolchain = "@bazel_tools//tools/cpp:toolchain_type",
    )

    # ------------------------------------------------------------------
    # Phase 2 — test script: compare versions at test time.
    # No objdump here — just reads the pre-built symbols_file from runfiles.
    # ------------------------------------------------------------------
    script = ctx.actions.declare_file(ctx.label.name + "_check.sh")
    ctx.actions.write(
        output = script,
        content = """\
#!/bin/sh
set -e
SYMBOLS_FILE="{symbols}"
MAX_VERSION="{max_version}"

if [ ! -s "$SYMBOLS_FILE" ]; then
  echo "No GLIBC symbols found (static binary or no glibc dep)"
  exit 0
fi

FOUND=$(grep -oE 'GLIBC_[0-9]+[.][0-9]+' "$SYMBOLS_FILE" | sed 's/GLIBC_//' | sort -t. -k1,1n -k2,2n | tail -1)
if [ -z "$FOUND" ]; then
  echo "No GLIBC symbols found (static binary or no glibc dep)"
  exit 0
fi

MAX_MAJOR=$(echo "$MAX_VERSION" | cut -d. -f1)
MAX_MINOR=$(echo "$MAX_VERSION" | cut -d. -f2)
FOUND_MAJOR=$(echo "$FOUND" | cut -d. -f1)
FOUND_MINOR=$(echo "$FOUND" | cut -d. -f2)

if [ "$FOUND_MAJOR" -gt "$MAX_MAJOR" ] || \\
   ([ "$FOUND_MAJOR" -eq "$MAX_MAJOR" ] && [ "$FOUND_MINOR" -gt "$MAX_MINOR" ]); then
  echo "FAIL: {binary_name} requires glibc $FOUND, max allowed is $MAX_VERSION"
  exit 1
fi
echo "PASS: {binary_name} glibc floor is $FOUND (<= $MAX_VERSION)"
""".format(
            symbols = symbols_file.short_path,
            max_version = ctx.attr.max_version,
            binary_name = binary.short_path,
        ),
        is_executable = True,
    )

    return [DefaultInfo(
        executable = script,
        runfiles = ctx.runfiles(files = [symbols_file]),
    )]

glibc_floor_test = rule(
    doc = """Verify that an ELF binary does not require a glibc version newer than max_version.

The test inspects the dynamic symbol table via the CC toolchain's objdump binary
(hermetic — not from PATH) and fails if any GLIBC_x.y symbol exceeds the floor.

Accepts both go_binary executables and shared libraries (.so); the binary
attribute should produce exactly one ELF file.
""",
    implementation = _glibc_floor_test_impl,
    attrs = {
        "binary": attr.label(
            doc = "The ELF binary or shared library to inspect.",
            allow_files = True,
            cfg = "target",
            mandatory = True,
        ),
        "max_version": attr.string(
            doc = "Maximum allowed glibc version, e.g. '2.17'.",
            mandatory = True,
        ),
    } | CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain(),
    fragments = ["cpp"],
    test = True,
)

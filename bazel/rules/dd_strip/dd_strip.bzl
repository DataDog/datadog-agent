"""dd_strip_symbols: split a binary/library into stripped + debug-only outputs.

dd_strip_symbols wraps any single label that produces one default output file (a go_binary,
cc_binary, cc_shared_library, or rust_binary) to provide 3 potential outputs.
- the original target (forwarding DefaultInfo files from the target)
- a stripped version of the taget
- the debug symbols only version of the target.

The rule's default output is the unstripped original unless //:install_stripped_binaries
is enabled, in which case it is the stripped twin. The split (and the debug-only
companion) is produced either way when requested, so a debug-symbol tarball (see
get_debug_symbols) can be built from the same configuration in which packages install
the unstripped files for downstream packagers (e.g. the omnibus build) that perform
their own stripping and symbol capture.

The debug output is only materialized for
consumers that ask for it via OutputGroupInfo (`--output_groups=+debug` or)
or by reading DdStripInfo directly.
"""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load(":dd_strip_info.bzl", "DdStripInfo")

_STRIPPER_TOOLCHAIN_TYPE = "//bazel/toolchains/dd_strip:dd_strip_toolchain_type"

def _dd_strip_symbols_impl(ctx):
    original = ctx.file.src

    toolchain = ctx.toolchains[_STRIPPER_TOOLCHAIN_TYPE]
    if toolchain == None:
        # No strip/split driver for this platform: pass the original through
        # rather than failing the build.    Consumers looking for DdStripInfo
        # will silently see the original target.
        return [
            DefaultInfo(files = depset([original])),
        ]

    basename = original.basename
    subdir = "strip_symbols/" + ctx.label.name
    stripped = ctx.actions.declare_file("{}/{}".format(subdir, basename))
    if toolchain.debug_is_directory:
        debug_name = basename + ".dSYM"
        debug = ctx.actions.declare_directory("{}/{}".format(subdir, debug_name))
    else:
        # TODO: We might need to use a different extension for windows.
        debug_name = basename + ".dbg"
        debug = ctx.actions.declare_file("{}/{}".format(subdir, debug_name))

    args = ctx.actions.args()
    args.add(original.path)
    args.add(stripped.path)
    args.add(debug.path)
    ctx.actions.run(
        mnemonic = "DdStripDebug",
        progress_message = "Splitting debug info for %s" % original.short_path,
        executable = toolchain.driver,
        arguments = [args],
        inputs = [original],
        outputs = [stripped, debug],
        toolchain = _STRIPPER_TOOLCHAIN_TYPE,
    )

    # Decide on whether we want to expose the originally unstripped binary or the
    # stripped one based on the //:install_stripped_binaries flag.
    install_stripped = ctx.attr._install_stripped[BuildSettingInfo].value
    default_file = stripped if install_stripped else original

    # Build a clone of the orignal DefaultInfo.  We can not simply return
    # the original because executable must also be built by this rule.
    default_info = ctx.attr.src[DefaultInfo]
    runfiles = getattr(default_info, "runfiles", None)
    data_runfiles = getattr(default_info, "data_runfiles", None)
    default_runfiles = getattr(default_info, "default_runfiles", None)

    # Preserve the executable-ness from the original object.
    was_executable = bool(hasattr(default_info, "files_to_run") and getattr(default_info.files_to_run, "executable", False))
    executable = stripped if (was_executable and install_stripped) else None

    return [
        DefaultInfo(files = depset([default_file]), executable = executable, runfiles = runfiles, data_runfiles = data_runfiles, default_runfiles = default_runfiles),
        DdStripInfo(
            original = original.owner,
            packaged_file = default_file,
            debug_file = debug,
        ),
        OutputGroupInfo(stripped = depset([stripped]), debug = depset([debug])),
    ]

dd_strip_symbols = rule(
    implementation = _dd_strip_symbols_impl,
    doc = """Wraps a single-output binary/library target, exposing:

    - DefaultInfo: The file packaging installs: the stripped version of the
      input when //:install_stripped_binaries is enabled, the unstripped
      original otherwise (the default).
    - DdStripInfo: Label of the wrapped target, the packaged file (see
      DefaultInfo), and the debug twin.
    - OutputGroupInfo(stripped = [...]): the stripped file
    - OutputGroupInfo(debug = [...]): the debug-only file/dSYM directory.

    Tree walking aspects can use DdStripInfo.original to identify objects that
    can be replaced with their stripped versions.  The two output groups are
    a conveniece for rules that must test either of the stripped or symbol outputs
    for various properties.
    """,
    attrs = {
        "src": attr.label(
            doc = "Label producing a single default output file to strip (a binary or shared library).",
            mandatory = True,
            allow_single_file = True,
        ),
        "_install_stripped": attr.label(
            doc = "Private label of the //:install_stripped_binaries build setting selecting the installed file",
            default = "@@//:install_stripped_binaries",
        ),
    },
    toolchains = [config_common.toolchain_type(_STRIPPER_TOOLCHAIN_TYPE, mandatory = False)],
)

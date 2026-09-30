"""dd_strip_symbols: split a binary/library into stripped + debug-only outputs.

dd_strip_symbols wraps any single label that produces one default output file (a go_binary,
cc_binary, cc_shared_library, or rust_binary) to provide 3 potential outputs.
- the original target (forwarding DefaultInfo files from the target)
- a stripped version of the taget
- the debug symbols only version of the target.

The debug output is only materialized for
consumers that ask for it via OutputGroupInfo (`--output_groups=+debug` or
) or by reading DdStripInfo directly

"""

load(":dd_strip_info.bzl", "DdStripInfo")

_STRIPPER_TOOLCHAIN_TYPE = "//bazel/toolchains/dd_strip:dd_strip_toolchain_type"

def _dd_strip_symbols_impl(ctx):
    original = ctx.file.input

    toolchain = ctx.toolchains[_STRIPPER_TOOLCHAIN_TYPE]
    if toolchain == None:
        # No strip/split driver for this platform: pass the original through
        # rather than failing the build.    Consumers looking for DdStripInfo
        # will silently see the original target.
        return [
            DefaultInfo(files = depset([original])),
        ]

    stripped_name = ctx.attr.stripped_file_name or (ctx.label.name + ".stripped")
    stripped_name = ctx.attr.stripped_file_name or ctx.label.name
    stripped = ctx.actions.declare_file(stripped_name)
    if toolchain.debug_is_directory:
        debug_name = ctx.attr.debug_file_name or (ctx.label.name + ".dSYM")
        debug = ctx.actions.declare_directory(debug_name)
    else:
        debug_name = ctx.attr.debug_file_name or (ctx.label.name + ".dbg")
        debug = ctx.actions.declare_file(debug_name)

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

    # Build a clone of the orignal DefaultInfo.  We can not simply return
    # the original because executable must also be built by this rule.
    default_info = ctx.attr.input[DefaultInfo]
    runfiles = getattr(default_info, "runfiles", None)
    data_runfiles = getattr(default_info, "data_runfiles", None)
    default_runfiles = getattr(default_info, "default_runfiles", None)
    # Preserve the executable-ness from the original object.
    was_executable = bool(hasattr(default_info, "files_to_run") and getattr(default_info.files_to_run, "executable", False))
    executable = stripped if was_executable else None

    return [
        DefaultInfo(files = depset([original, stripped]), executable = executable, runfiles = runfiles, data_runfiles = data_runfiles, default_runfiles = default_runfiles),
        DdStripInfo(
            original = original.owner,
            original_file = original,
            stripped_file = stripped,
            debug_file = debug,
        ),
        OutputGroupInfo(stripped = depset([stripped]), debug = depset([debug])),
    ]

dd_strip_symbols = rule(
    implementation = _dd_strip_symbols_impl,
    doc = """Wraps a single-output binary/library target, exposing:

    - DefaultInfo: The strippped version of input.
    - DdStripInfo: Provider of the original, stripped, and debug outputs.
    - OutputGroupInfo(stripped = [...]): the stripped file
    - OutputGroupInfo(debug = [...]): the debug-only file/dSYM directory.

    Tree walking aspects can use DdStripInfo.original to identify objects that
    can be replaced with their stripped versions.  The two output groups are
    a conveniece for rules that must test either of the stripped or symbol outputs
    for various properties.
    """,
    attrs = {
        # The name "input" instead of the typical "src" or "target", is used for
        # alignment with collect_dependencies, which only walks a small set of
        # attributes.
        "input": attr.label(
            doc = "Label producing a single default output file to strip (a binary or shared library).",
            mandatory = True,
            allow_single_file = True,
        ),
        "debug_file_name": attr.string(doc = "name for stripped file. Defaults to name+'.dbg'"),
        "stripped_file_name": attr.string(doc = "name for stripped file. Defaults to name+'.stripped'"),
    },
    toolchains = [config_common.toolchain_type(_STRIPPER_TOOLCHAIN_TYPE, mandatory = False)],
)

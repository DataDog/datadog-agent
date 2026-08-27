"""dd_strip_debug: split a binary/library into stripped + debug-only outputs.

dd_strip_debug wraps any single label that produces one default output file (a go_binary,
cc_binary, cc_shared_library, or rust_binary) to provide 3 potential outputs.
- the original target (forwarding DefaultInfo files from the target)
- a stripped version of the taget
- the debug symbols only version of the target.

The wrapped rule's DefaultInfo is untouched and stays unstripped, so the wrapped
target may be used in place of the orignal as a dependency.
The split outputs are only materialized for
consumers that ask for them via OutputGroupInfo (`--output_groups=+debug` or
`+stripped`) or by reading DdStripInfo directly

"""

load("@rules_cc//cc:defs.bzl", "cc_binary")
load("@rules_go//go:def.bzl", "go_binary")
load(":dd_strip_info.bzl", "DdStripInfo")

_STRIPPER_TOOLCHAIN_TYPE = "//bazel/toolchains/dd_strip:dd_strip_toolchain_type"

def _dd_strip_debug_impl(ctx):
    original = ctx.file.src

    toolchain = ctx.toolchains[_STRIPPER_TOOLCHAIN_TYPE]
    if toolchain == None:
        # No strip/split driver for this platform: pass the original through
        # rather than failing the build.    Consumers looking for DdStripInfo
        # will silently see the original target.
        return [
            DefaultInfo(files = depset(original)),
        ]

    stripped = ctx.actions.declare_file(ctx.label.name + ".stripped")
    if toolchain.debug_is_directory:
        debug = ctx.actions.declare_directory(ctx.label.name + ".debug")
    else:
        debug = ctx.actions.declare_file(ctx.label.name + ".debug")

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
    # Transfer the executable-ness from the original file.
    default_info = ctx.attr.src[DefaultInfo]
    was_executable = bool(hasattr(default_info, "executable") and default_info.executable)
    executable = stripped if was_executable else None

    return [
        DefaultInfo(files = depset([stripped]), executable = executable),
        DdStripInfo(
            original = original.owner,
            original_file = original,
            stripped_file = stripped,
            debug_file = debug,
        ),
        OutputGroupInfo(stripped = depset([stripped]), debug = depset([debug])),
    ]

dd_strip_symbols = rule(
    implementation = _dd_strip_debug_impl,
    doc = """Wraps a single-output binary/library target, exposing:

    - DefaultInfo: The strippped version of src.
    - DdStripInfo: Provider of the original, stripped, and debug outputs.
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
    },
    toolchains = [config_common.toolchain_type(_STRIPPER_TOOLCHAIN_TYPE, mandatory = False)],
)

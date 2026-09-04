"""dd_strip_debug: split a binary/library into stripped + debug-only outputs.

Wraps any single label that produces one default output file (a go_binary,
cc_binary, cc_shared_library, or rust_binary). The wrapped rule's DEFAULT
output is untouched and stays unstripped, so `bazel build //some:target`
behavior never changes; the split outputs are only materialized for
consumers that ask for them via OutputGroupInfo (`--output_groups=+debug` or
`+stripped`) or by reading DdStripInfo directly (see dd_pkg_files_stripped.bzl).

dd_go_binary_with_debug / dd_cc_binary_with_debug / dd_rust_binary_with_debug
are convenience macros that create the wrapped binary/library under a private
`<name>_unstripped` target and wrap it with dd_strip_debug under the
requested `name`, so callers just swap the rule name and keep every other
kwarg unchanged.
"""

load("@rules_cc//cc:defs.bzl", "cc_binary")
load("@rules_go//go:def.bzl", "go_binary")
load(":dd_strip_info.bzl", "DdStripInfo")

_TOOLCHAIN_TYPE = "//bazel/toolchains/dd_strip:dd_strip_toolchain_type"

def _passthrough(original, excluded):
    return [
        DefaultInfo(files = depset([original])),
        DdStripInfo(
            stripped_file = original,
            debug_file = None,
            original_file = original,
            excluded = excluded,
        ),
        OutputGroupInfo(stripped = depset([original]), debug = depset([])),
    ]

def _dd_strip_debug_impl(ctx):
    original = ctx.file.input

    if ctx.attr.exclude:
        return _passthrough(original, excluded = True)

    toolchain = ctx.toolchains[_TOOLCHAIN_TYPE]
    if toolchain == None or not toolchain.available:
        # No strip/split driver for this platform: pass the original through
        # rather than failing the build (parity with DdStripInfo.excluded).
        return _passthrough(original, excluded = True)

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
        executable = toolchain.driver,
        arguments = [args],
        inputs = [original],
        outputs = [stripped, debug],
        toolchain = _TOOLCHAIN_TYPE,
        mnemonic = "DdStripDebug",
        progress_message = "Splitting debug info for %s" % original.short_path,
    )

    return [
        # Default output stays unstripped: bazel build //x:x never changes.
        DefaultInfo(files = depset([original])),
        DdStripInfo(
            stripped_file = stripped,
            debug_file = debug,
            original_file = original,
            excluded = False,
        ),
        OutputGroupInfo(stripped = depset([stripped]), debug = depset([debug])),
    ]

dd_strip_debug = rule(
    implementation = _dd_strip_debug_impl,
    doc = """Wraps a single-output binary/library target, exposing:

    - DefaultInfo: the original, unstripped file (unchanged build behavior).
    - OutputGroupInfo(stripped = [...]): the stripped file, for packaging.
    - OutputGroupInfo(debug = [...]): the debug-only file/dSYM directory.
    - DdStripInfo: the same three files plus whether stripping was skipped.

    When exclude = True (parity with omnibus's strip_exclude, e.g. for eBPF
    .o objects handled separately by _stripped_ebpf), or when no dd_strip
    toolchain is registered for the current platform, stripped_file equals
    original_file and debug_file is None -- no stripping occurs and
    DdStripInfo.excluded is True.""",
    attrs = {
        "input": attr.label(
            doc = "Label producing a single default output file to strip (a binary or shared library).",
            mandatory = True,
            allow_single_file = True,
        ),
        "exclude": attr.bool(
            doc = "Skip stripping and expose the original file unchanged, parity with omnibus's strip_exclude.",
            default = False,
        ),
    },
    toolchains = [config_common.toolchain_type(_TOOLCHAIN_TYPE, mandatory = False)],
)

def _wrap(binary_rule, name, strip_exclude, visibility, kwargs):
    unstripped_name = name + "_unstripped"
    binary_rule(
        name = unstripped_name,
        visibility = ["//visibility:private"],
        tags = (kwargs.pop("tags", None) or []) + ["manual"],
        **kwargs
    )
    dd_strip_debug(
        name = name,
        input = ":" + unstripped_name,
        exclude = strip_exclude,
        visibility = visibility,
    )

def dd_go_binary_with_debug(name, strip_exclude = False, visibility = None, **kwargs):
    """A go_binary wrapped with dd_strip_debug.

    Accepts all go_binary attributes via **kwargs. See dd_strip_debug for the
    behavior of the wrapping (default output stays unstripped; use
    --output_groups=+stripped/+debug or DdStripInfo to get the split).

    Args:
      name: target name. The underlying go_binary is created as
            "<name>_unstripped" (private, tags = ["manual"]).
      strip_exclude: parity with omnibus's strip_exclude -- skip stripping.
      visibility: visibility of the resulting dd_strip_debug target.
      **kwargs: forwarded to go_binary.
    """
    _wrap(go_binary, name, strip_exclude, visibility, kwargs)

def dd_cc_binary_with_debug(name, strip_exclude = False, visibility = None, **kwargs):
    """A cc_binary wrapped with dd_strip_debug. See dd_go_binary_with_debug."""
    _wrap(cc_binary, name, strip_exclude, visibility, kwargs)

def dd_rust_binary_with_debug(name, strip_exclude = False, visibility = None, **kwargs):
    """A rust_binary wrapped with dd_strip_debug. See dd_go_binary_with_debug."""

    # Deferred import: rules_rust is not a dependency of every consumer of
    # this file (e.g. dd_go_binary_with_debug-only callers), and loading it
    # unconditionally would pull rules_rust into packages that never use it.
    _wrap(_rust_binary(), name, strip_exclude, visibility, kwargs)

def _rust_binary():
    return Label("@rules_rust//rust:defs.bzl%rust_binary")

"""dd_pkg_strip_transform — packaging-time strip/debug-split filter (ABLD-464 Plan B).

Consumes a PackageFilesInfo (the same `dest_src_map: {dest_path: File}`
structure dd_cc_packaged.bzl and dd_collect_dependencies.bzl already operate
on) and emits a new PackageFilesInfo pointing at transformed files:

  - mode = "stripped" (default): object files have symbol tables/debug info
    removed; everything else (configs, licenses, scripts, directories, ...)
    passes through byte-for-byte unchanged.
  - mode = "debug_only": only the split-off debug artifacts remain --
    non-object-file entries are dropped from the dest_src_map entirely, since
    e.g. a config file has no debug info and should not appear in the
    debug-only sibling package.

Because this operates on the already-flattened, merged dest_src_map (after
pkg_files has resolved srcs to Files), it needs no cooperation from the
binary's own build rule -- it works against `prebuilt_file`-backed filegroups
like `@agent_binary//:agent` today, which is this design's main advantage
over the provider/rule-based alternative (see the ABLD-464 design doc).

IMPLEMENTATION NOTE on avoiding double work: the "stripped" and "debug_only"
modes must not each independently run strip/objcopy/dsymutil on the same
file -- that would double the work for every packaging build that produces
both a package and its debug sibling. To guarantee this, the actual
strip/split actions live in the private `_dd_strip_split` rule (one action
per file, declaring BOTH outputs at once); `dd_pkg_strip_transform` itself
creates no actions of its own -- it only *selects* which half of
`_dd_strip_split`'s already-declared outputs to expose. As long as both the
"stripped" and "debug_only" `dd_pkg_strip_transform` instances point at the
same `_dd_strip_split` target (which `dd_pkg_files_stripped` guarantees by
construction), Bazel analyzes that shared target once, so the action runs at
most once per file no matter how many of its consumers are built.

File-type detection (ELF vs Mach-O vs PE vs "not an object file at all") is
necessarily a runtime concern -- Starlark cannot inspect file contents during
the analysis phase -- so it happens in dd_strip_driver.py, not here. This
file's `_looks_like_object_file` is only a cheap, best-effort Starlark-side
heuristic (extension + the pkg_files group's executable-bit attribute) used
to skip spawning an action at all for files that are obviously not object
code; the driver script is the authoritative fallback for anything else.
"""

load("@rules_pkg//pkg:mappings.bzl", "pkg_files")
load("@rules_pkg//pkg:providers.bzl", "PackageFilesInfo")

_DD_STRIP_TOOLCHAIN = "//bazel/toolchains/dd_strip:toolchain_type"

# Extensions that are unambiguously not machine code. Skipping these avoids
# spawning a strip action (and, in debug_only mode, a spurious dest_src_map
# entry) for the configs/licenses/docs/scripts that make up most of a
# package tree. This is only a fast-path optimization -- see the module
# docstring; dd_strip_driver.py's magic-byte sniffing is authoritative.
_NEVER_BINARY_EXTENSIONS = (
    ".yaml",
    ".yml",
    ".json",
    ".txt",
    ".md",
    ".cfg",
    ".conf",
    ".ini",
    ".py",
    ".sh",
    ".rb",
    ".pem",
    ".crt",
    ".key",
    ".png",
    ".svg",
    ".html",
    ".example",
    ".license",
)

_ALWAYS_BINARY_SUFFIXES = (".so", ".dylib", ".dll", ".exe")

def _looks_like_object_file(dest, executable):
    """Best-effort guess at whether `dest` is worth spawning a strip action for.

    Errs toward "maybe" -- a false positive here just costs a wasted action
    that the driver script turns into a no-op passthrough. A false negative
    would silently skip stripping a real binary, so extensionless files
    (typical for Go/Rust/C binaries shipped as e.g. "bin/agent/agent") are
    treated as plausible whenever the enclosing pkg_files group is marked
    executable.
    """
    lower = dest.lower()
    if lower.endswith(_ALWAYS_BINARY_SUFFIXES) or ".so." in lower:
        return True
    for ext in _NEVER_BINARY_EXTENSIONS:
        if lower.endswith(ext):
            return False
    basename = lower.rsplit("/", 1)[-1]
    if "." not in basename:
        return True
    return executable

_DdStripSplitInfo = provider(
    doc = "Internal: the per-mode dest_src_maps produced by one shared strip/split pass.",
    fields = {
        "stripped_dest_src_map": "dict: dest path -> stripped File, for every entry in the source PackageFilesInfo.",
        "debug_dest_src_map": "dict: dest path -> debug File/directory, for entries that were actually strippable object files.",
        "attributes": "the source PackageFilesInfo.attributes, forwarded as-is.",
        "debug_attributes": "attributes to apply to the debug_only dest_src_map (mode forced non-executable).",
    },
)

def _debug_attributes(attributes):
    # Debug artifacts (.debug files / .dSYM bundles / unstripped originals)
    # are inspected by debuggers, not executed -- ship them non-executable
    # regardless of what mode the shipped binary itself uses.
    result = dict(attributes)
    result["mode"] = "0644"
    return result

def _dd_strip_split_impl(ctx):
    toolchain = ctx.toolchains[_DD_STRIP_TOOLCHAIN]
    strip_info = toolchain.dd_strip_info if toolchain else None
    src_info = ctx.attr.src[PackageFilesInfo]
    executable = "x" in src_info.attributes.get("mode", "")

    # macOS debug output is a dsymutil .dSYM bundle (a directory); Linux's
    # objcopy --only-keep-debug and Windows' "copy of the unstripped
    # original" are both single files.
    debug_is_dir = bool(strip_info and strip_info.dsymutil_path)
    debug_suffix = ".dSYM" if debug_is_dir else (".debug" if strip_info and strip_info.objcopy_path else "")

    stripped_dest_src_map = {}
    debug_dest_src_map = {}
    all_outputs = []

    for dest, file in src_info.dest_src_map.items():
        # Directories (TreeArtifacts) aren't split element-by-element by this
        # rule; ship them unstripped and exclude them from the debug sibling.
        # This is a known limitation of the packaging-time-filter approach --
        # see the ABLD-464 design doc.
        if file.is_directory or not _looks_like_object_file(dest, executable) or strip_info == None:
            stripped_dest_src_map[dest] = file
            continue

        stripped_out = ctx.actions.declare_file("dd_strip/stripped/" + dest)
        if debug_is_dir:
            debug_out = ctx.actions.declare_directory("dd_strip/debug/" + dest + debug_suffix)
        else:
            debug_out = ctx.actions.declare_file("dd_strip/debug/" + dest + debug_suffix)

        args = ctx.actions.args()
        args.add("--input", file)
        args.add("--stripped-out", stripped_out)
        args.add("--debug-out", debug_out.path)
        if debug_is_dir:
            args.add("--debug-out-is-dir")
        if strip_info.strip_path:
            args.add("--strip", strip_info.strip_path)
        if strip_info.objcopy_path:
            args.add("--objcopy", strip_info.objcopy_path)
        if strip_info.dsymutil_path:
            args.add("--dsymutil", strip_info.dsymutil_path)

        ctx.actions.run(
            executable = ctx.executable._driver,
            arguments = [args],
            inputs = depset([file], transitive = [strip_info.tool_files]),
            outputs = [stripped_out, debug_out],
            mnemonic = "DdStripSplit",
            progress_message = "Splitting debug symbols from %s" % dest,
            toolchain = _DD_STRIP_TOOLCHAIN,
        )

        stripped_dest_src_map[dest] = stripped_out
        debug_dest_src_map[dest] = debug_out
        all_outputs.extend([stripped_out, debug_out])

    return [
        _DdStripSplitInfo(
            stripped_dest_src_map = stripped_dest_src_map,
            debug_dest_src_map = debug_dest_src_map,
            attributes = src_info.attributes,
            debug_attributes = _debug_attributes(src_info.attributes),
        ),
        DefaultInfo(files = depset(all_outputs)),
    ]

_dd_strip_split = rule(
    implementation = _dd_strip_split_impl,
    doc = """Internal: runs the actual strip/split action once per (strippable) file.

    Not meant to be used directly -- use dd_pkg_files_stripped. Kept separate
    from dd_pkg_strip_transform so that "stripped" and "debug_only" mode
    instances can share a single set of actions; see the module docstring.
    """,
    attrs = {
        "src": attr.label(
            mandatory = True,
            providers = [PackageFilesInfo],
        ),
        "_driver": attr.label(
            default = Label("//bazel/rules/dd_packaging:dd_strip_driver"),
            executable = True,
            cfg = "exec",
        ),
    },
    toolchains = [config_common.toolchain_type(_DD_STRIP_TOOLCHAIN, mandatory = False)],
)

def _dd_pkg_strip_transform_impl(ctx):
    split_info = ctx.attr.split[_DdStripSplitInfo]
    if ctx.attr.mode == "debug_only":
        dest_src_map = split_info.debug_dest_src_map
        attributes = split_info.debug_attributes
    else:
        dest_src_map = dict(split_info.stripped_dest_src_map)
        attributes = split_info.attributes

    return [
        PackageFilesInfo(
            dest_src_map = dest_src_map,
            attributes = attributes,
        ),
        # Without an explicit DefaultInfo, `bazel build`/`cquery --output=files`
        # against this target directly would request the (empty) implicit
        # default output group and never actually force the underlying
        # _dd_strip_split action to run. Packaging rules only look at
        # PackageFilesInfo, but this makes `bazel build` on the label alone
        # (as in local iteration, or a debug-package pkg_filegroup) do
        # something observable.
        DefaultInfo(files = depset(dest_src_map.values())),
    ]

dd_pkg_strip_transform = rule(
    implementation = _dd_pkg_strip_transform_impl,
    doc = """Projects one mode's worth of a shared _dd_strip_split's outputs into a PackageFilesInfo.

    Not meant to be used directly -- use dd_pkg_files_stripped.
    """,
    attrs = {
        "split": attr.label(
            mandatory = True,
            providers = [_DdStripSplitInfo],
        ),
        "mode": attr.string(
            mandatory = True,
            values = ["stripped", "debug_only"],
        ),
    },
    provides = [PackageFilesInfo],
)

def dd_pkg_files_stripped(name, srcs, prefix = "", mode = "stripped", **kwargs):
    """A pkg_files-alike whose binaries are stripped at packaging time.

    Behaves like `pkg_files(name, srcs, prefix, **kwargs)`, except object
    files (recognized as ELF/Mach-O/PE by dd_strip_driver.py) have their
    debug info split off instead of shipping unmodified. `name` carries the
    requested `mode`'s worth of the result (default "stripped", i.e. the
    normal packaging behavior with debug info removed); a `name + "_debug"`
    sibling target is always created as well (mode="debug_only"), containing
    only the split-off debug artifacts -- reference that label from a debug
    package's pkg_filegroup. Both labels are backed by the same underlying
    strip/split actions, so building both never runs strip/objcopy/dsymutil
    twice on the same file (see dd_pkg_strip_transform.bzl for why that
    matters).

    Args:
        name: name of the "stripped"-mode target (or whatever `mode` requests).
        srcs: same as pkg_files' srcs.
        prefix: same as pkg_files' prefix.
        mode: "stripped" (default) or "debug_only". Only pass "debug_only"
            directly if you don't need the normal stripped-package variant
            at all -- in that case no "_debug" sibling is created, since
            `name` already is the debug-only variant.
        **kwargs: forwarded to the underlying pkg_files call (e.g. attributes).
    """
    files_name = name + "_pkg_files"
    split_name = name + "_split"

    pkg_files(
        name = files_name,
        srcs = srcs,
        prefix = prefix,
        tags = ["manual"],
        visibility = ["//visibility:private"],
        **kwargs
    )

    _dd_strip_split(
        name = split_name,
        src = ":" + files_name,
        tags = ["manual"],
        visibility = ["//visibility:private"],
    )

    dd_pkg_strip_transform(
        name = name,
        split = ":" + split_name,
        mode = mode,
    )

    if mode != "debug_only":
        dd_pkg_strip_transform(
            name = name + "_debug",
            split = ":" + split_name,
            mode = "debug_only",
        )

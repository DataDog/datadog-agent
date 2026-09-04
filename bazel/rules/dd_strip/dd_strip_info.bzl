"""Provider reporting the stripped/debug split produced by dd_strip_debug."""

DdStripInfo = provider(
    doc = """Reports the outputs of splitting a single binary/library into a
    stripped artifact (for shipping) and a debug-only artifact (for a
    companion "dbg" package), mirroring omnibus's Stripper.

    Consumers such as dd_pkg_files_stripped read this provider off a wrapped
    target to decide which file to place in the shipped package vs. the
    debug-only package.""",
    fields = {
        "stripped_file": """File. The binary/library with debug info removed,
            ready for the shipped package. Equal to original_file when
            excluded is True or no strip toolchain is available.""",
        "debug_file": """File or None. The extracted debug information.
            A regular File on Linux (objcopy --only-keep-debug output) and
            Windows (the unstripped original), a Directory for macOS .dSYM
            bundles. None when excluded is True or no strip toolchain is
            available for the target platform.""",
        "original_file": "File. The unstripped input as originally built.",
        "excluded": """bool. True when this target opted out of stripping
            (parity with omnibus's strip_exclude, used e.g. for eBPF .o
            objects) or when no dd_strip toolchain is available for the
            current platform. stripped_file is the unmodified original_file
            in that case, and debug_file is None.""",
    },
)

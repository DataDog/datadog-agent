"""Provider reporting the stripped/debug split produced by dd_strip_debug."""

DdStripInfo = provider(
    doc = """Provides the outputs of splitting a single binary/library into a
    stripped artifact (for the main distribution package) and a debug-only
    artifact (for a companion "dbg" package).

    Consumers (such as dd_pkg_*) could read this provider off a wrapped
    target to decide which file to place in the shipped package vs. the
    debug-only package. The "original" member exists to specify a target
    from the overall dependency graph that needs to be replace with either
    stripped or debug.
    """,
    fields = {
        "original": "Label of the orignal target.",
        "original_file": "File. The unstripped input as originally built.",
        "stripped_file": """File. The binary/library with debug symbols removed.
            Equal to original_file when excluded is True or no strip toolchain
            is available.""",
        "debug_file": """File or None. The extracted debug information.
            A regular File on Linux (objcopy --only-keep-debug output) and
            Windows (the unstripped original), a Directory for macOS .dSYM
            bundles.""",
    },
)

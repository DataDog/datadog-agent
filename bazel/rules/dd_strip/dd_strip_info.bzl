"""Provider reporting the packaged/debug split produced by dd_strip_symbols."""

DdStripInfo = provider(
    doc = """Provides the packaging-relevant outputs of dd_strip_symbols: the
    file packages install and its debug-only twin for the companion
    "dbg" package.
    """,
    fields = {
        "original": "Label of the wrapped target.",
        "packaged_file": """File. The file packages install: the unstripped
            original by default, the stripped twin when
            //:install_stripped_binaries is enabled.""",
        "debug_file": """File or Directory. The extracted debug information.
            A regular File on Linux (objcopy --only-keep-debug output) and
            Windows (the unstripped original), a Directory for macOS .dSYM
            bundles.""",
    },
)

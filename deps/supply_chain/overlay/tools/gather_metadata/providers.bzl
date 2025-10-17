"""Providers for transitively gathering all license and package_info targets.

Warning: This is private to the aspect that walks the tree. The API is subject
to change at any release.
"""

TargetWithMetadataInfo = provider(
    doc = """A target and the assocated metadata for it.""",
    fields = {
        "target": "Label: A target which will be associated with some metadata.",
        "metadata": "depset(): [list] of my direct collected leaf providers",
    },
)

TransitiveMetadataInfo = provider(
    doc = """The transitive set of TargetWithMetadataInfo objects.""",
    fields = {
        "trans": "depset(): transitive collection of TWMI",
        "top_level_target": "Label: The top level target label we are examining.",
        "traces": "list(string) - diagnostic for tracing a dependency relationship to a target.",
    },
)

null_transitive_metadata_info = TransitiveMetadataInfo()

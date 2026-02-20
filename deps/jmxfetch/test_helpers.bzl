"""Test helpers for jmxfetch module_utils."""

load(":module_utils.bzl", "parse_jmxfetch_version")

# Define a provider to expose version parsing results
VersionParseInfo = provider(
    doc = "Information about a parsed jmxfetch version",
    fields = {
        "version": "The input version string",
        "is_snapshot": "Whether the version is a snapshot",
        "url": "The constructed download URL",
    },
)

version_parse_test_rule_attrs = {
    "version": attr.string(doc = "Version string to parse"),
    "out": attr.output(doc = "Output file"),
}

def _version_parse_test_rule_impl(ctx):
    """Test rule that parses a version and outputs the result."""

    # Call the actual parsing function
    version = ctx.attr.version
    version_info = parse_jmxfetch_version(version)

    # Write the result to the output file for debugging
    output_content = """Version: {version}
is_snapshot: {is_snapshot}
url: {url}
""".format(
        version = version,
        is_snapshot = version_info["is_snapshot"],
        url = version_info["url"],
    )

    ctx.actions.write(
        output = ctx.outputs.out,
        content = output_content,
    )

    # Return both the file and a provider with the parsed info
    return [
        DefaultInfo(files = depset([ctx.outputs.out])),
        VersionParseInfo(
            version = version,
            is_snapshot = version_info["is_snapshot"],
            url = version_info["url"],
        ),
    ]

version_parse_test_rule = rule(
    implementation = _version_parse_test_rule_impl,
    attrs = version_parse_test_rule_attrs,
)

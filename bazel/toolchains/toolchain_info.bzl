"""Common provider for tools we wrap in a toolchain."""

ToolInfo = provider(
    doc = """Agent toolchain""",
    fields = {
        "name": "The name of the toolchain",
        "valid": "Is this toolchain valid and usable?",
        "version": "The version string of the tool",
        "label": "The path to a target I will build. If we are building the tool from source",
        "path": "The path to a pre-built instance of the tool",
    },
)

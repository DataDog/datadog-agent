PathFixedThingInfo = provider(
    doc = """A file and it's rpath fixed version.""",
    fields = {
        "original": "File object. Usually a shared library",
        "fixed": "File object.",
        "label": "Label which produced it",
    },
)

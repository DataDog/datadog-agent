load("@package_metadata//rules:package_metadata.bzl", "package_metadata")
load("//compliance/rules:purl.bzl", "purl_for_golang")

def _impl(name, attributes, module, visibility):
    package_metadata(
        name = name,
        attributes = attributes,
        purl = purl_for_golang(module),
        # also reachable from targets declared by macros defined elsewhere
        visibility = ["//visibility:public"],
    )

go_package_metadata = macro(
    implementation = _impl,
    attrs = {
        "attributes": attr.label_list(configurable = False),
        "module": attr.string(mandatory = True, configurable = False),
    },
)

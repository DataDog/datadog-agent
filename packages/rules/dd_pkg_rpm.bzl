"""dd_pkg_rpm - wrapper for pkg_rpm adding agent specific defaults."""

load("@rules_pkg//pkg:rpm_pfg.bzl", "pkg_rpm_impl")
load("//packages/rules:dd_pkg_owned_dirs.bzl", "dd_pkg_owned_dirs")
load("//packages/rules:description.bzl", "rpm_description")

_DEFAULT_DEFINES = {
    "__provides_exclude": ".*",
    "__requires_exclude": ".*",
    "_build_id_links": "none",
}

# rpm only removes directories listed in the package on uninstall. Declare the
# implicit parent directories of srcs so they don't get left behind.
def _dd_pkg_rpm_impl(name, visibility, description, srcs, defines = None, **kwargs):
    owned_dirs = "%s_owned_dirs_" % name
    dd_pkg_owned_dirs(
        name = owned_dirs,
        srcs = srcs,
        tags = ["manual"],
        visibility = ["//visibility:private"],
    )

    merged_defines = dict(_DEFAULT_DEFINES)
    merged_defines.update(defines or {})
    pkg_rpm_impl(
        name = name,
        srcs = srcs + [":" + owned_dirs],
        binary_payload_compression = select({
            "//:is_release": "w9T8.xzdio",
            "//conditions:default": "w5T8.xzdio",
        }),
        defines = merged_defines,
        description = rpm_description(description),
        visibility = visibility,
        **kwargs
    )

dd_pkg_rpm = macro(
    doc = "Build an RPM package",
    inherit_attrs = pkg_rpm_impl,
    implementation = _dd_pkg_rpm_impl,
    attrs = {
        "binary_payload_compression": None,
        "defines": attr.string_dict(configurable = False),
        "description": attr.string(configurable = False),
        "private_stamp_detect": None,
        "stamp": None,
    },
)

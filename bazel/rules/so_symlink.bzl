load("@rules_pkg//pkg:mappings.bzl", "pkg_mklink")

def _so_symlink_impl(name, visibility, lib, major, minor, patch, prefix):
    pkg_mklink(
        name = name + "_major",
        link_name = "%s%s.%d" % (prefix, lib, major),
        target = lib,
        visibility = visibility,
    )
    if minor >= 0:
        if patch >= 0:
            pkg_mklink(
                name = name + "_patch",
                link_name = "%s%s.%d.%d.%d" % (prefix, lib, major, minor, patch),
                target = lib,
                visibility = visibility,
            )
        else:
            pkg_mklink(
                name = name + "_minor",
                link_name = "%s%s.%d.%d" % (prefix, lib, major, minor),
                target = lib,
                visibility = visibility,
            )

so_symlink = macro(
    attrs = {
        "lib": attr.label(mandatory = True, configurable = False),
        "major": attr.int(mandatory = True, configurable = False),
        "minor": attr.int(default = -1, configurable = False),
        "patch": attr.int(default = -1, configurable = False),
        "prefix": attr.string(default = "lib/", configurable = False),
    },
    implementation = _so_symlink_impl,
)

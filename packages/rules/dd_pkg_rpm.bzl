"""dd_pkg_rpm - wrapper for pkg_rpm adding agent specific defaults."""

load("@rules_pkg//pkg:rpm_pfg.bzl", "pkg_rpm_impl")
load("//packages/rules:dd_pkg_owned_dirs.bzl", "dd_pkg_owned_dirs")

# rpm only removes directories listed in the package on uninstall. Declare the
# implicit parent directories of srcs so they don't get left behind.
def _dd_pkg_rpm_impl(name, visibility, srcs, **kwargs):
    owned_dirs = "%s_owned_dirs_" % name
    dd_pkg_owned_dirs(
        name = owned_dirs,
        srcs = srcs,
        tags = ["manual"],
        visibility = ["//visibility:private"],
    )

    pkg_rpm_impl(
        name = name,
        srcs = srcs + [":" + owned_dirs],
        visibility = visibility,
        **kwargs
    )

dd_pkg_rpm = macro(
    doc = "Build an RPM package",
    inherit_attrs = pkg_rpm_impl,
    implementation = _dd_pkg_rpm_impl,
    attrs = {
        "private_stamp_detect": None,
        "stamp": None,
    },
)

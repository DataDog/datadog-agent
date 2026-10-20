"""dd_pkg_rpm - wrapper for pkg_rpm adding agent specific defaults."""

load("@rules_pkg//pkg:rpm_pfg.bzl", "pkg_rpm_impl")

def _dd_pkg_rpm_impl(name, visibility, **kwargs):
    pkg_rpm_impl(
        name = name,
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

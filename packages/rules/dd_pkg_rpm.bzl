"""dd_pkg_rpm - wrapper for pkg_rpm adding agent specific defaults."""

load("@rules_pkg//pkg:rpm_pfg.bzl", "pkg_rpm_impl")
load("//packages/rules:dd_pkg_owned_dirs.bzl", "dd_pkg_owned_dirs")
load("//packages/rules:description.bzl", "rpm_description")

_DEFAULT_DEFINES = {
    # Skip the steps rpmbuild runs after %install (brp-strip, brp-compress,
    # ...), like omnibus does. Binaries are already stripped by dd_strip.
    "__spec_install_post": "%{nil}",
    "_build_id_links": "none",
}

# rpm only removes directories listed in the package on uninstall. Declare the
# implicit parent directories of srcs so they don't get left behind.
def _rpm_package(name, srcs, defines = None, **kwargs):
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
        autoreqprov = False,
        binary_payload_compression = select({
            "//:is_release": "w9T8.xzdio",
            "//conditions:default": "w5T8.xzdio",
        }),
        defines = merged_defines,
        **kwargs
    )

_DBG_UNSET_ATTRS = {
    "post_scriptlet": None,
    "post_scriptlet_file": None,
    "posttrans_scriptlet": None,
    "posttrans_scriptlet_file": None,
    "postun_scriptlet": None,
    "postun_scriptlet_file": None,
    "pre_scriptlet": None,
    "pre_scriptlet_file": None,
    "preun_scriptlet": None,
    "preun_scriptlet_file": None,
    "provides": None,
    "requires_contextual": None,
}

def _dd_pkg_rpm_impl(name, visibility, debug_symbols, description, package_file_name, package_name, srcs, **kwargs):
    common = dict(
        description = rpm_description(description),
        packager = "Datadog, Inc <package@datadoghq.com>",
        vendor = "Datadog <package@datadoghq.com>",
        visibility = visibility,
    )

    _rpm_package(
        name = name,
        srcs = srcs,
        package_file_name = package_file_name,
        package_name = package_name,
        **(common | kwargs)
    )

    if not debug_symbols:
        return

    if not package_name or not package_file_name.startswith(package_name):
        fail("debug_symbols requires package_file_name to start with package_name, got: %s and %s" % (package_file_name, package_name))

    _rpm_package(
        name = "%s_dbg" % name,
        srcs = [debug_symbols],
        package_file_name = package_name + "-dbg" + package_file_name.removeprefix(package_name),
        package_name = package_name + "-dbg",
        **(common | kwargs | _DBG_UNSET_ATTRS | dict(
            # Require the exact same version of the main package. These are RPM
            # macros, expanded by rpmbuild from the epoch, version and release
            # of the -dbg package, which are the same as the main package's.
            requires = [package_name + " = %{{?epoch:%{{epoch}}:}}%{{version}}-%{{release}}"],
        ))
    )

dd_pkg_rpm = macro(
    doc = """Build an RPM package.

With debug_symbols, also build <name>_dbg: a <package_name>-dbg package holding
debug_symbols, typically a get_debug_symbols target.
""",
    inherit_attrs = pkg_rpm_impl,
    implementation = _dd_pkg_rpm_impl,
    attrs = {
        "autoreqprov": None,
        "binary_payload_compression": None,
        "debug_symbols": attr.label(
            doc = "Debug symbols to put in a <name>_dbg package.",
            configurable = False,
        ),
        "defines": attr.string_dict(configurable = False),
        "description": attr.string(configurable = False),
        "package_file_name": attr.string(configurable = False),
        "package_name": attr.string(configurable = False),
        "packager": None,
        "private_stamp_detect": None,
        "stamp": None,
        "vendor": None,
    },
)

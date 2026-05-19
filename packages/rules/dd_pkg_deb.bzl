"""dd__pkg_deb - wrapper for pkg_deb adding agent specific defaults."""

# We need to peek at the actual rule instead of the wrapper because you
# can't put a macro on a legacy macro.
# buildifier: disable=bzl-visibility
load("@rules_pkg//pkg/private/deb:deb.bzl", "pkg_deb_impl")
load("//packages/rules:package_naming.bzl", "package_name_variables")
load("//tools/tar_checksums:tar_md5sums.bzl", "tar_md5sums")

# kwargs is mandatory for macros, even if you don't use it.
# buildifier: disable=unused-variable
def _dd_pkg_deb_impl(name, visibility, conflicts, data, depends, description, homepage, license, maintainer, out, package, package_file_name, postinst, postrm, preinst, prerm, priority, recommends, section, version, **kwargs):
    variables_name = "%s_vars_" % name
    package_name_variables(
        name = variables_name,
        product_name = package,
    )

    sums_out = "%s_md5sums_out_" % name
    tar_md5sums(
        name = "%s_md5sums_" % name,
        src = data,
        md5sums = sums_out,
    )

    pkg_deb_impl(
        name = name,
        conflicts = conflicts,
        data = data,
        depends = depends,
        description = description,
        homepage = homepage or "http://www.datadoghq.com",
        license = license or "Apache License Version 2.0",
        maintainer = maintainer or "Datadog Packages <package@datadoghq.com>",
        md5sums = ":" + sums_out,
        out = out,
        package = package,
        package_file_name = package_file_name,
        package_variables = ":" + variables_name,
        postinst = postinst,
        postrm = postrm,
        preinst = preinst,
        prerm = prerm,
        priority = priority or "extra",
        recommends = recommends,
        section = section or "utils",
        version = version,
        visibility = visibility,
    )

dd_pkg_deb = macro(
    doc = "Build a debian package",
    inherit_attrs = pkg_deb_impl,
    implementation = _dd_pkg_deb_impl,
    attrs = {
        "md5sums": None,
        "package_variables": None,
    },
)

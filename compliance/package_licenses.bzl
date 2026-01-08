"""Collect licenses for a product package in a way we can ship them."""

load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@rules_pkg//pkg:mappings.bzl", "filter_directory", "pkg_files")
load("//compliance:license_csv.bzl", "license_csv")

visibility("public")

def package_licenses(name = None, src = None):
    """Collects all the licenses for a source target and makes them available for packaging.

    This result of this rule is a pkg_filegorup suitable for use in pkg_* rules.
    The pkg_install target '{name}_install' is provided as a convenience.

    Args:
        name: name
        src: target to search. This is typically a filegroup of the top level
             elements that will be bundled into a distribution artifact.
    """

    # Collect everything
    license_csv(
        name = "%s_licenses_" % name,
        target = src,
        csv_out = "%s_licenses.csv" % name,
        licenses_dir = "%s_licenses_dir_" % name,
        offers_dir = "%s_sources_dir_" % name,
        tags = ["manual"],
    )

    # Turn the copied licenses into a target we can feed to other rules.
    native.filegroup(
        name = "%s_copied_license_dir_" % name,
        srcs = [":%s_licenses_" % name],
        output_group = "licenses",
    )

    # This silliness strips the directory name of the license TreeArtifact.
    filter_directory(
        name = "%s_license_dir_stripped_" % name,
        src = ":%s_copied_license_dir_" % name,
        outdir_name = "LICENSES",
        strip_prefix = ".",
    )

    # Turn the copied offers into a target we can feed to other rules.
    native.filegroup(
        name = "%s_copied_offer_dir_" % name,
        srcs = [":%s_licenses_" % name],
        output_group = "offers",
    )

    # This silliness strips the directory name of the offer TreeArtifact.
    filter_directory(
        name = "%s_offer_dir_stripped_" % name,
        src = ":%s_copied_offer_dir_" % name,
        outdir_name = "sources",
        strip_prefix = ".",
    )

    # You would expect that filter_directory returns something usuable to pkg_* rules.
    # It does not, so we have to wrap it.
    pkg_files(
        name = name,
        srcs = [
            ":%s_license_dir_stripped_" % name,
            ":%s_offer_dir_stripped_" % name,
        ],
        visibility = ["//visibility:public"],
    )

    pkg_install(
        name = "%s_install" % name,
        srcs = [name],
        tags = ["manual"],  # Do not catch in ... expansion.
    )

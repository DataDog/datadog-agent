"""Collect licenses for a product package in a way we can ship them."""

load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@rules_pkg//pkg:mappings.bzl", "filter_directory", "pkg_files")
load("//compliance:license_csv.bzl", "license_csv")

visibility("public")

def package_licenses(name = None, src = None, visibility = None, **kwargs):
    """Collects all the licenses for a source target and makes them available for packaging.

    This result of this rule is a pkg_filegorup suitable for use in pkg_* rules.
    The pkg_install target '{name}_install' is provided as a convenience.
    The '{name}_manifest_info' target exposes a PackageLicenseManifestInfo provider
    (see //compliance:license_csv.bzl) pointing at the structured license data, for
    rules like version_manifest that need it without re-running license gathering.

    Args:
        name: name
        src: target to search. This is typically a filegroup of the top level
             elements that will be bundled into a distribution artifact.
        visibility: visisibility
        **kwargs: other args
    """
    # All targets are explicitly "manual" to not get caught in ... expansion.
    # The license collection runs an aspect over the entire build graph and
    # can be expensive. We should test license collection for each product
    # packaging explicitly.

    # Collect everything
    license_csv(
        name = "%s_licenses_" % name,
        target = src,
        csv_out = "%s_licenses.csv" % name,
        manifest_out = "%s_licenses_manifest.json" % name,
        licenses_dir = "%s_licenses_dir_" % name,
        offers_dir = "%s_sources_dir_" % name,
        tags = ["manual"],
    )

    # Public handle to the PackageLicenseManifestInfo provider, for rules (e.g.
    # version_manifest) that need the structured license list without re-running
    # the license-gathering aspect themselves. Aliases forward providers of "actual".
    native.alias(
        name = "%s_manifest_info" % name,
        actual = ":%s_licenses_" % name,
        tags = ["manual"],
        visibility = visibility or ["//visibility:public"],
    )

    # Turn the copied licenses into a target we can feed to other rules.
    native.filegroup(
        name = "%s_copied_license_dir_" % name,
        srcs = [":%s_licenses_" % name],
        output_group = "licenses",
        tags = ["manual"],
    )

    # This silliness strips the directory name of the license TreeArtifact.
    filter_directory(
        name = "%s_license_dir_stripped_" % name,
        src = ":%s_copied_license_dir_" % name,
        outdir_name = "LICENSES",
        strip_prefix = ".",
        tags = ["manual"],
    )

    # Turn the copied offers into a target we can feed to other rules.
    native.filegroup(
        name = "%s_copied_offer_dir_" % name,
        srcs = [":%s_licenses_" % name],
        output_group = "offers",
        tags = ["manual"],
    )

    # This silliness strips the directory name of the offer TreeArtifact.
    filter_directory(
        name = "%s_offer_dir_stripped_" % name,
        src = ":%s_copied_offer_dir_" % name,
        outdir_name = "sources",
        strip_prefix = ".",
        tags = ["manual"],
    )

    # You would expect that filter_directory returns something usuable to pkg_* rules.
    # It does not, so we have to wrap it.
    pkg_files(
        name = name,
        srcs = [
            ":%s_license_dir_stripped_" % name,
        ] + select({
            # None of the deps that declare ship_source_offer (freetds, openscap, attr, etc.) are included in the
            # Windows build; offers_dir is therefore always empty there. rules_python 1.9.0 makes `bazel run` copy
            # runfiles to a fresh temp dir and skips empty dirs, causing pkg_install to fail with FileNotFoundError
            # (Linux/macOS use symlinks so empty TreeArtifacts are fine); remove this select when Windows does.
            "@platforms//os:windows": [],  #TODO(ABLD-351): deal with products that don't have source offers
            "//conditions:default": [":%s_offer_dir_stripped_" % name],
        }),
        tags = ["manual"],
        visibility = visibility or ["//visibility:public"],
        **kwargs
    )

    # Temporary target while we are still doing a hybrid omnibus build.
    pkg_install(
        name = "%s_install" % name,
        srcs = [name],
        tags = ["manual"],
    )

"""Declares rule `version_manifest`.

Produces a JSON file approximating the omnibus `<artifact>.metadata.json`
sidecar for a built package: artifact facts (basename, sha256/sha512 of the
package file), project facts (name, version, license), and the nested
"software" list of everything whose license was gathered by a
//compliance:package_licenses target.

This is a first cut aimed at capturing the overall structure (see
lib/omnibus/manifest.rb and lib/omnibus/metadata.rb in omnibus-ruby); it does
not yet match the legacy format field-for-field.
"""

load("@rules_pkg//pkg:providers.bzl", "PackageVariablesInfo")
load("//compliance:license_csv.bzl", "PackageLicenseManifestInfo")

visibility("public")

def _version_manifest_impl(ctx):
    package_files = ctx.attr.package[DefaultInfo].files.to_list()
    if len(package_files) != 1:
        fail("version_manifest's 'package' must produce exactly one file, got: %s" % package_files)
    package_file = package_files[0]

    manifest_info = ctx.attr.licenses[PackageLicenseManifestInfo]
    variables = ctx.attr.package_variables[PackageVariablesInfo].values

    out = ctx.outputs.out
    inputs = [package_file, manifest_info.manifest_file]

    args = ctx.actions.args()
    args.add("--package", package_file.path)
    args.add("--licenses", manifest_info.manifest_file.path)
    args.add("--out", out.path)
    args.add("--name", ctx.attr.product_name or variables["product_name"])
    args.add("--friendly_name", ctx.attr.friendly_name)
    args.add("--homepage", ctx.attr.homepage)
    args.add("--version", variables["version"])
    args.add("--license", ctx.attr.license)
    args.add("--arch", variables[ctx.attr.arch_key])
    if ctx.attr.build_git_revision:
        args.add("--build_git_revision", ctx.attr.build_git_revision)
    if ctx.file.license_file:
        args.add("--license_file", ctx.file.license_file.path)
        inputs.append(ctx.file.license_file)

    ctx.actions.run(
        mnemonic = "VersionManifest",
        progress_message = "Writing version manifest for %s" % str(ctx.attr.package.label),
        executable = ctx.executable._processor,
        arguments = [args],
        inputs = inputs,
        outputs = [out],
    )

    return [DefaultInfo(files = depset([out]))]

_version_manifest = rule(
    implementation = _version_manifest_impl,
    doc = """Writes a version-manifest style JSON file describing a built package.""",
    attrs = {
        "package": attr.label(
            doc = "The pkg_deb/pkg_rpm (or similar) target whose output file this manifest describes.",
            mandatory = True,
        ),
        "licenses": attr.label(
            doc = "A //compliance:package_licenses '{name}_manifest_info' target.",
            providers = [PackageLicenseManifestInfo],
            mandatory = True,
        ),
        "package_variables": attr.label(
            doc = "A package_name_variables target, for version/arch/product_name.",
            providers = [PackageVariablesInfo],
            mandatory = True,
        ),
        "arch_key": attr.string(
            doc = "Which package_variables key to use for 'arch' (e.g. arch_deb, arch_rpm).",
            default = "arch_deb",
        ),
        "product_name": attr.string(
            doc = "Overrides package_variables' product_name if set.",
        ),
        "friendly_name": attr.string(default = ""),
        "homepage": attr.string(default = "http://www.datadoghq.com"),
        "license": attr.string(default = "Apache-2.0"),
        "build_git_revision": attr.string(),
        "license_file": attr.label(
            doc = "The project's own top-level LICENSE file, embedded verbatim as license_content.",
            allow_single_file = True,
        ),
        "out": attr.output(mandatory = True),
        "_processor": attr.label(
            default = Label("//compliance:write_version_manifest"),
            cfg = "exec",
            executable = True,
        ),
    },
)

def version_manifest(
        *,  # Disallow unnamed attributes.
        name,
        package,
        licenses,
        package_variables,
        out = None,
        visibility = None,
        **kwargs):
    """Writes a version-manifest style JSON file describing a built package.

    Args:
        name: name
        package: the pkg_deb/pkg_rpm (or similar) target this manifest describes.
        licenses: a //compliance:package_licenses '{name}_manifest_info' target.
        package_variables: a package_name_variables target.
        out: output file name. Defaults to '{name}.json'.
        visibility: visibility
        **kwargs: other args, forwarded to the underlying rule (e.g. arch_key, license, homepage).
    """
    _version_manifest(
        name = name,
        package = package,
        licenses = licenses,
        package_variables = package_variables,
        out = out or ("%s.json" % name),
        visibility = visibility,
        **kwargs
    )

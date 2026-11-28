"""Gather the third-party Go packages used by a target and map them to their licenses.

The licenses come from LICENSE-3rdparty.csv, whose Origin column is the Go import path of each
package, so each go_library is matched on its importpath. This is the Bazel replacement for
omnibus `third_party_licenses`, which uses the whole CSV regardless of what a product links.

The aspect is a trimmed copy of `gather_metadata_info_common` from
@supply_chain_tools//gather_metadata:core.bzl. It only records Go packages from external
repositories; packages in this repository are covered by the Agent's own license.
"""

load("@bazel_features//:features.bzl", "bazel_features")
load("@rules_go//go:def.bzl", "GoInfo")
load("@rules_license//rules:providers.bzl", "PackageInfo")
load("@supply_chain_tools//gather_metadata:core.bzl", "should_traverse")

GoPackagesInfo = provider(
    doc = "Third-party Go packages found in the transitive dependencies of a target.",
    fields = {
        "packages": "depset of struct(importpath, label, purl)",
    },
)

_null_go_packages_info = GoPackagesInfo(packages = depset())

def _is_exec_config(ctx):
    if bazel_features.rules.is_tool_configuration_public and ctx.configuration.is_tool_configuration():
        return True
    return ctx.bin_dir.path.endswith("-exec/bin") or "-exec-" in ctx.bin_dir.path

def _attr_targets(attr_value):
    if type(attr_value) == "Target":
        return [attr_value]
    if type(attr_value) == type([]):
        return [item for item in attr_value if type(item) == "Target"]
    if type(attr_value) == type({}):
        targets = []
        for key, value in attr_value.items():
            for entry in (key, value):
                if type(entry) == "Target":
                    targets.append(entry)
                elif type(entry) == type([]):
                    targets.extend([item for item in entry if type(item) == "Target"])
        return targets
    return []

def _purl(ctx):
    """Returns the purl of the package_info attached to the rule, or ""."""
    if hasattr(ctx.rule.attr, "package_metadata"):
        metadata = ctx.rule.attr.package_metadata
    elif hasattr(ctx.rule.attr, "applicable_licenses"):
        metadata = ctx.rule.attr.applicable_licenses
    else:
        metadata = []
    for dep in metadata:
        if PackageInfo in dep and dep[PackageInfo].purl:
            return dep[PackageInfo].purl
    return ""

def _go_packages_aspect_impl(target, ctx):
    # Build tools are not shipped, so their packages do not need licenses.
    if _is_exec_config(ctx):
        return [_null_go_packages_info]

    direct = []
    if GoInfo in target and target.label.workspace_name:
        importpath = getattr(target[GoInfo], "importpath", None) or getattr(ctx.rule.attr, "importpath", None)
        if importpath:
            direct.append(struct(
                importpath = importpath,
                label = str(target.label),
                purl = _purl(ctx),
            ))

    transitive = []
    for name in dir(ctx.rule.attr):
        if not should_traverse(ctx, name):
            continue
        for dep in _attr_targets(getattr(ctx.rule.attr, name)):
            if GoPackagesInfo in dep:
                info = dep[GoPackagesInfo]
                if info != _null_go_packages_info:
                    transitive.append(info.packages)

    if not direct and not transitive:
        return [_null_go_packages_info]
    if not direct and len(transitive) == 1:
        return [GoPackagesInfo(packages = transitive[0])]
    return [GoPackagesInfo(packages = depset(direct, transitive = transitive))]

go_packages_aspect = aspect(
    doc = "Collects the third-party Go packages a target depends on into GoPackagesInfo.",
    implementation = _go_packages_aspect_impl,
    attr_aspects = ["*"],
    provides = [GoPackagesInfo],
    apply_to_generating_rules = True,
)

def _go_third_party_licenses_impl(ctx):
    packages = []
    if GoPackagesInfo in ctx.attr.src:
        packages = [
            {"importpath": p.importpath, "label": p.label, "purl": p.purl}
            for p in ctx.attr.src[GoPackagesInfo].packages.to_list()
        ]
    packages_json = ctx.actions.declare_file(ctx.label.name + ".go_packages.json")
    ctx.actions.write(packages_json, json.encode_indent(packages))

    args = ctx.actions.args()
    args.add("--packages", packages_json)
    args.add("--csv", ctx.file.csv)
    args.add("--wwhrd", ctx.file.wwhrd)
    args.add("--license_ids_out", ctx.outputs.license_ids_out)
    args.add("--packages_out", ctx.outputs.packages_out)
    ctx.actions.run(
        executable = ctx.executable._processor,
        arguments = [args],
        inputs = [packages_json, ctx.file.csv, ctx.file.wwhrd],
        outputs = [ctx.outputs.license_ids_out, ctx.outputs.packages_out],
        mnemonic = "GoThirdPartyLicenses",
        progress_message = "Matching Go packages of %{label} to LICENSE-3rdparty.csv",
    )
    return [DefaultInfo(files = depset([ctx.outputs.license_ids_out, ctx.outputs.packages_out]))]

_go_third_party_licenses = rule(
    implementation = _go_third_party_licenses_impl,
    attrs = {
        "src": attr.label(
            doc = "Target whose transitive Go dependencies are gathered.",
            mandatory = True,
            aspects = [go_packages_aspect],
        ),
        "csv": attr.label(
            doc = "LICENSE-3rdparty.csv style file: Component,Origin,License,Copyright.",
            default = Label("//:license_3rdparty_csv"),
            allow_single_file = True,
        ),
        "wwhrd": attr.label(
            doc = "wwhrd config; packages under its `exceptions` are allowed to be absent from csv.",
            default = Label("//:wwhrd_config"),
            allow_single_file = True,
        ),
        "license_ids_out": attr.output(
            doc = "Sorted, distinct license ids, one per line.",
            mandatory = True,
        ),
        "packages_out": attr.output(
            doc = "CSV of importpath,purl,license for every matched package.",
            mandatory = True,
        ),
        "_processor": attr.label(
            default = Label("//compliance:go_licenses"),
            cfg = "exec",
            executable = True,
        ),
    },
)

def go_third_party_licenses(name, src, **kwargs):
    """Lists the licenses of the third-party Go packages that `src` depends on.

    Outputs `<name>.license_ids.txt` (distinct license ids) and `<name>.go_packages.csv`
    (importpath,purl,license per package). The build fails if a package is neither in
    LICENSE-3rdparty.csv nor excepted by .wwhrd.yml.

    Args:
      name: target name.
      src: target to gather Go packages from, e.g. a pkg_filegroup of the product.
      **kwargs: passed to the rule (e.g. csv, wwhrd, target_compatible_with, tags).
    """
    _go_third_party_licenses(
        name = name,
        src = src,
        license_ids_out = name + ".license_ids.txt",
        packages_out = name + ".go_packages.csv",
        **kwargs
    )

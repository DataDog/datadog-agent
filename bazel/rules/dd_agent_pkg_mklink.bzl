"""dd_agent_pkg_mklink. Expand a template, splicing in agent specific flags."""

load("@rules_pkg//pkg:providers.bzl", "PackageSymlinkInfo")
load("//bazel/rules/variables:variables.bzl", "DdBuildTimeInfo")

def _dd_agent_pkg_mklink_impl(ctx):
    common = ctx.attr._variables[DdBuildTimeInfo].values

    subs = {}
    subs["install_dir"] = common["install_dir"]
    subs["build_version"] = common["build_version"]

    # Default our link mode to 755
    out_attributes = json.decode(ctx.attr.attributes)
    out_attributes.setdefault("mode", "0755")
    return [
        PackageSymlinkInfo(
            destination = ctx.attr.link_name.format(**subs),
            target = ctx.attr.target.format(**subs),
            attributes = out_attributes,
        ),
    ]

dd_agent_pkg_mklink = rule(
    implementation = _dd_agent_pkg_mklink_impl,
    doc = """pkg_mklink with substitution from select Agent flags.

This performs a simple search over the template file for the keys in
substitutions, and replaces them with the corresponding values.  There are
some default substitutions which are computed from the build environment.

Default substitutions:
  {install_dir}:  The value of the flag //:install_dir
  {build_version}: The pipeline build version.
""",
    attrs = {
        "target": attr.string(
            doc = """See @rules_pkg for documentation.""",
            mandatory = True,
        ),
        "link_name": attr.string(
            doc = """See @rules_pkg for documentation.""",
            mandatory = True,
        ),
        "attributes": attr.string(
            doc = """See @rules_pkg for documentation.""",
            default = "{}",  # Empty JSON
        ),
        "_variables": attr.label(default = "//bazel/rules/variables"),
    },
)

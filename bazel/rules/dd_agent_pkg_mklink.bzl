"""dd_agent_pkg_mklink. Expand a template, splicing in agent specific flags."""

load("@agent_volatile//:env_vars.bzl", "env_vars")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@rules_pkg//pkg:providers.bzl", "PackageSymlinkInfo")

# The place we will install to if we run bazel pkg_install without a destdir
# We use /tmp for lack of a better safe space.
DEFAULT_OUTPUT_CONFIG_DIR = "/tmp"

# The location where the product should be installed on a user system.
DEFAULT_PRODUCT_DIR = "/opt/datadog-agent"

def _dd_agent_pkg_mklink_impl(ctx):
    # Set up a fallback default.
    subs = {}

    # TODO: Consider sharing common logic with dd_agent_expand_template IFF we find
    #       the alignment in variable names gets larger.
    # TODO: should this be different for windows? Or should we have different variables for windows?
    subs["output_config_dir"] = DEFAULT_OUTPUT_CONFIG_DIR
    if ctx.attr._output_config_dir and BuildSettingInfo in ctx.attr._output_config_dir:
        output_config_dir = ctx.attr._output_config_dir[BuildSettingInfo].value.rstrip("/")
        subs["output_config_dir"] = output_config_dir
    subs["install_dir"] = DEFAULT_PRODUCT_DIR
    if ctx.attr._install_dir and BuildSettingInfo in ctx.attr._install_dir:
        install_dir = ctx.attr._install_dir[BuildSettingInfo].value
        subs["install_dir"] = install_dir

    # The environment variable is PACKAGE_VERSION but the omnibus scripts
    # use build_version. Let's unify that in the future. For now, it is not
    # clear which direction we should move towards.
    subs["build_version"] = env_vars.PACKAGE_VERSION or "_build_version_unset_"

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
  {output_config_dir}:  The value of the flag //:output_config_dir
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
        "_install_dir": attr.label(default = "@@//:install_dir"),
        "_output_config_dir": attr.label(default = "@@//:output_config_dir"),
    },
)

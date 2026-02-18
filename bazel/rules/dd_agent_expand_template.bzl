"""dd_agent_expand_template. Expand a template, splicing in agent specific flags."""

load("@bazel_skylib//lib:paths.bzl", "paths")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@rules_pkg//pkg:providers.bzl", "PackageFilesInfo")

# The place we will install to if we run bazel pkg_install without a destdir
# We use /tmp for lack of a better safe space.
DEFAULT_OUTPUT_CONFIG_DIR = "/tmp"

# The location where the product should be installed on a user system.
DEFAULT_PRODUCT_DIR = "/opt/datadog-agent"

def _dd_agent_expand_template_impl(ctx):
    # Set up a fallback default.
    subs = {}

    # TODO: should this be different for windows? Or should we have different variables for windows?
    subs["{output_config_dir}"] = DEFAULT_OUTPUT_CONFIG_DIR
    if ctx.attr._output_config_dir:
        if BuildSettingInfo in ctx.attr._output_config_dir:
            output_config_dir = ctx.attr._output_config_dir[BuildSettingInfo].value.rstrip("/")
            subs["{output_config_dir}"] = output_config_dir
    subs["{install_dir}"] = DEFAULT_PRODUCT_DIR
    if ctx.attr._install_dir:
        if BuildSettingInfo in ctx.attr._install_dir:
            install_dir = ctx.attr._install_dir[BuildSettingInfo].value
            subs["{install_dir}"] = install_dir

    # TODO: decide if we should default etc to the output base or relative to install_dir.
    # There are use cases for either. For now, we are relative to the output base.
    # Note that the choice of /etc/datadog-agent seems odd, in that /etc is usually /etc,
    # but this matches what is done in the current product packaging.
    subs["{etc_dir}"] = subs["{output_config_dir}"] + "/etc/datadog-agent"

    # Now add local substitutions.
    # For extra oomph, we apply the flag substitutions first. That allows us to
    # reuse tmplates which might have <% install_dir %> by declaring a local
    # substitution   "<% install_dir %>": "{install_dir}".
    # That is an affordance during migration.
    computed_subs = {}
    for key, value in ctx.attr.substitutions.items():
        for flag, flag_value in subs.items():
            value = value.replace(flag, flag_value)
        computed_subs[key] = value
    subs.update(computed_subs)

    # let's add a little flavor. We don't need this today but it will be
    # needed if we expand things like the file name of an artifact and we want
    # to show compliation mode. This mostly shows the technique.
    subs["{TARGET_CPU}"] = ctx.var["TARGET_CPU"]
    subs["{COMPILATION_MODE}"] = ctx.var["COMPILATION_MODE"]

    ctx.actions.expand_template(
        template = ctx.file.template,
        output = ctx.outputs.out,
        substitutions = subs,
    )

    # Now build an output path relative to this package.
    workspace_root = paths.join("..", ctx.label.workspace_name) if ctx.label.workspace_name else ""
    package_path = paths.join(workspace_root, ctx.outputs.out.owner.package)
    dest_path = paths.relativize(ctx.outputs.out.short_path, package_path)
    destination = paths.join(ctx.attr.prefix, dest_path)
    return [
        DefaultInfo(files = depset([ctx.outputs.out])),
        PackageFilesInfo(
            dest_src_map = {destination: ctx.outputs.out},
            attributes = json.decode(ctx.attr.attributes),
        ),
    ]

OUT_DOC = """The destination of the expanded file.

When this target is consummed by pkg_* rules, the output destinaion
of the file is computed relative to the package. This is effectively
the equivalent to using `strip_prefix=strip_prefix.from_pkg()` of a
`pkg_files` rule.

If `prefix` is also specified, that is applied after computing the
package relative path.
"""

dd_agent_expand_template = rule(
    implementation = _dd_agent_expand_template_impl,
    doc = """Template expansion

This performs a simple search over the template file for the keys in
substitutions, and replaces them with the corresponding values.  There are
some default substitutions which are computed from the build environment.

Default substitutions:
  {install_dir}:  The value of the flag //:install_dir
  {output_config_dir}:  The value of the flag //:output_config_dir
  {etc_dir}: The output_config_dir + "/etc"
  {TARGET_CPU}: Target CPU arch
  {COMPILATION_MODE}: usually fastbuild, opt, or release.

There is no special syntax for the keys from substitutions. To avoid conflicts, you would need to
explicitly add delimiters to the key strings, for example "{KEY}" or "@KEY@".""",
    attrs = {
        "template": attr.label(
            mandatory = True,
            allow_single_file = True,
            doc = "The template file to expand.",
        ),
        "substitutions": attr.string_dict(
            doc = "A dictionary mapping strings to their substitutions. These take precedence over flags.",
        ),
        "out": attr.output(mandatory = True, doc = OUT_DOC),
        "attributes": attr.string(
            doc = """See @rules_pkg for documentation.""",
            default = "{}",  # Empty JSON
        ),
        "prefix": attr.string(doc = """See @rules_pkg for documentation."""),
        "_install_dir": attr.label(default = "@@//:install_dir"),
        "_output_config_dir": attr.label(default = "@@//:output_config_dir"),
    },
)

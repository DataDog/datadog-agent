"""dd_agent_expand_template. Expand a template, splicing in agent specific flags."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

# The place we will install to if we run bazel pkg_install without a destdir
# We use /tmp for lack of a better safe space.
DEFAULT_OUTPUT_CONFIG_DIR = "/tmp"

# The location where the product should be installed on a user system.
DEFAULT_PRODUCT_DIR = "/opt/datadog-agent"

def _dd_agent_expand_template_impl(ctx):
    # Set up a fallback default.
    # TODO: should this be different for windows? Or should we have different variables for windows?
    subs = {}
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
    subs["{etc_dir}"] = subs["{output_config_dir}"] + "/etc"

    # Now let local substitutions override the flags.
    subs.update(ctx.attr.substitutions)

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
    return [DefaultInfo(files = depset([ctx.outputs.out]))]

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
        "out": attr.output(
            mandatory = True,
            doc = "The destination of the expanded file.",
        ),
        "_install_dir": attr.label(default = "@@//:install_dir"),
        "_output_config_dir": attr.label(default = "@@//:output_config_dir"),
    },
)

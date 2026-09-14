"""variables. A single, shared source of build-time substitution values.

Several rules and macros (dd_agent_pkg_mklink, dd_agent_expand_template,
package_name_variables, dd_agent_go_binary) each independently derive values
like the agent version or the install directory from environment variables,
release.json, and build settings. This file centralizes that computation so
there is exactly one place that knows how to compute each value.

Most consumers are rules and can depend on the `variables` target below and
read its DdBuildTimeVariables provider. dd_agent_go_binary is a legacy macro
(no ctx, runs at loading time) and cannot read a provider off a target, so the
version-related subset of the computation is also exposed as a plain function,
compute_version_variables(), that does not require ctx.
"""

load("@agent_volatile//:env_vars.bzl", "env_vars")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@dd_release_json//:release_json.bzl", "release_json")
load("//tasks:agent_payload_version.bzl", "AGENT_PAYLOAD_VERSION")

# The location where the product should be installed on a user system.
DEFAULT_PRODUCT_DIR = "/opt/datadog-agent"

# The place we will install to if we run bazel pkg_install without a destdir.
# We use /tmp for lack of a better safe space.
DEFAULT_OUTPUT_CONFIG_DIR = "/tmp"

DdBuildTimeVariables = provider(
    doc = "A dict of common build-time substitution values, computed once from " +
          "the build environment (env vars, release.json, build settings).",
    fields = ["values"],
)

def url_safe_to_standard(url_safe):
    """Convert a URL-safe agent version string to the standard SemVer form.

    PACKAGE_VERSION is produced by `dda inv agent.version --url-safe`, which
    replaces the SemVer '+' build-metadata separator with '.'. The standard
    form uses '+'.

    Examples:
      "7.81.0-devel.git.635.e3326d4.pipeline.1" -> "7.81.0-devel+git.635.e3326d4.pipeline.1"
      "7.81.0-rc.1.git.635.e3326d4"             -> "7.81.0-rc.1+git.635.e3326d4"
      "7.81.0"                                   -> "7.81.0"  (clean release, no change)
    """
    idx = url_safe.find(".git.")
    if idx < 0:
        return url_safe
    return url_safe[:idx] + "+git." + url_safe[idx + 5:]

def standard_to_url_safe(standard):
    """Convert a standard SemVer agent version string to the URL-safe form.

    Mirrors url_safe_to_standard(): only the SemVer '+' build-metadata
    separator is replaced with '.', matching the convention used by
    `dda inv agent.version --url-safe`. No other character is touched.

    Examples:
      "7.81.0-devel+git.635.e3326d4.pipeline.1" -> "7.81.0-devel.git.635.e3326d4.pipeline.1"
      "7.81.0-rc.1+git.635.e3326d4"             -> "7.81.0-rc.1.git.635.e3326d4"
      "7.81.0"                                   -> "7.81.0"  (clean release, no change)
    """
    idx = standard.find("+git.")
    if idx < 0:
        return standard
    return standard[:idx] + ".git." + standard[idx + 5:]

def compute_version_variables():
    """Compute the subset of common variables derived purely from env vars and release.json.

    These values need no ctx (no build settings are involved), so this can be
    called both from rule implementations (which have ctx) and from legacy
    macros like dd_agent_go_binary (which only run at loading time).

    Returns:
      dict with keys: build_version, agent_version, agent_version_url_safe,
      base_branch, milestone, agent_payload_version.
    """

    # The environment variable is PACKAGE_VERSION but the omnibus scripts use
    # build_version. Let's unify that in the future. For now, it is not clear
    # which direction we should move towards.
    build_version = env_vars.PACKAGE_VERSION or "_build_version_unset_"

    # In CI: PACKAGE_VERSION env var is set by `dda inv agent.version --url-safe`,
    # producing the URL-safe dotted form e.g.
    # "7.81.0-devel.git.635.e3326d4.pipeline.102267660".
    # Locally: fall back to release_json current_milestone + "-localbuild".
    if env_vars.PACKAGE_VERSION:
        agent_version_url_safe = env_vars.PACKAGE_VERSION
    else:
        agent_version_url_safe = release_json.get("current_milestone") + "-localbuild"
    agent_version = url_safe_to_standard(agent_version_url_safe)

    return {
        "build_version": build_version,
        "agent_version": agent_version,
        "agent_version_url_safe": agent_version_url_safe,
        "base_branch": release_json.get("base_branch"),
        "milestone": release_json.get("current_milestone"),
        "agent_payload_version": AGENT_PAYLOAD_VERSION,
    }

def _variables_impl(ctx):
    values = {}
    values.update(compute_version_variables())

    install_dir = DEFAULT_PRODUCT_DIR
    if ctx.attr._install_dir and BuildSettingInfo in ctx.attr._install_dir:
        install_dir = ctx.attr._install_dir[BuildSettingInfo].value
    values["install_dir"] = install_dir

    output_config_dir = DEFAULT_OUTPUT_CONFIG_DIR
    if ctx.attr._output_config_dir and BuildSettingInfo in ctx.attr._output_config_dir:
        output_config_dir = ctx.attr._output_config_dir[BuildSettingInfo].value.rstrip("/")
    values["output_config_dir"] = output_config_dir

    # TODO: decide if we should default etc to the output base or relative to install_dir.
    # There are use cases for either. For now, we are relative to the output base.
    values["etc_dir"] = output_config_dir + "/etc/datadog-agent"

    return [DdBuildTimeVariables(values = values)]

variables = rule(
    implementation = _variables_impl,
    doc = """Computes the common build-time substitution values shared across packaging
and version-stamping rules, and returns them as a DdBuildTimeVariables provider.

Values provided:
  install_dir:  The value of the flag //:install_dir
  output_config_dir: The value of the flag //:output_config_dir
  etc_dir: output_config_dir + "/etc/datadog-agent"
  build_version: The pipeline build version (PACKAGE_VERSION), raw.
  base_branch: Agent base branch from release.json.
  milestone: Next product milestone version from release.json.
  agent_version: The agent version in standard SemVer form.
  agent_version_url_safe: The agent version in URL-safe form.
  agent_payload_version: The agent payload version.
""",
    attrs = {
        "_install_dir": attr.label(default = "//:install_dir"),
        "_output_config_dir": attr.label(default = "//:output_config_dir"),
    },
)

"""package_naming - tools for handling package file names.

Tools to create file names like datadog-fips-agent_7.78.0~devel.git.538.46fb587.pipeline.102267660-1_arm64.deb
from things we have:

  PACKAGE_VERSION
    - From the omnibus environment
    - created with tasks.libs.releasing.version%get_version from
      - CI_* vars in the gitlab environment
      - release.json for fallbacks
      - example: 7.78.0-devel.git.538.46fb587.pipeline.102267660
  release.json: provides base_branch, and current_milestone
  pkg_* rules that take the name of the product as input.

"""

load("@agent_volatile//:env_vars.bzl", "env_vars")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@dd_release_json//:release_json.bzl", "release_json")
load("@rules_cc//cc:find_cc_toolchain.bzl", "find_cc_toolchain", "use_cc_toolchain")
load("@rules_pkg//pkg:providers.bzl", "PackageVariablesInfo")

# Map of architecture names we might see into those we need to use in package names
_arch_names = {
    # debs use amd64.  E.g. datadog-agent-dbg-7.78.0~devel.git.684.d9f682a.pipeline.103327127-1.x86_64.rpm
    "deb": {
        "aarch64": "arm64",
        "arm64": "arm64",
        "k8": "amd64",
        "x86_64": "amd64",
        "x86": "amd64",
        "64": "amd64",  # Macos amd comes back with just "64"
    },
    # rpms use x86_64.  E.g. datadog-agent-dbg-7.78.0~devel.git.684.d9f682a.pipeline.103327127-1.x86_64.rpm
    "rpm": {
        "aarch64": "aarch64",
        "arm64": "aarch64",
        "amd64": "x86_64",
        "k8": "x86_64",
        "x86": "x86_64",
        "64": "x86_64",
    },
}

_DOCSTRING = """
Provide a PackageVariablesInfo which can be used for substitutions into output file names for pkg_* rules.

The three things you are likely to use:
- version: A suitable version that includes the pipeline id.
- arch: A deb/rpm conformant architecture string based on the target platform.
- product_name = ctx.attr.product_name

And other thing which might be useful:
- base_branch: Agent base branch from release.json.
- compilation_mode: bazel compilation mode (e.g. fastbuild, release).
- compiler: raw compiler name from CC toolchain.
- cpu: raw cpu name from CC toolchain.
- libc: raw libc name from CC toolchain.
- milestone: Next product milestone version.
"""

def _make_version():
    """Make the version component of the file name.

    Use PACKAGE_VERSION, if it is availble, otherwise guess from release.json

    Returns:
       version (str)
    """
    if env_vars.PACKAGE_VERSION:
        return env_vars.PACKAGE_VERSION.replace("-", "~")

    milestone = release_json.get("current_milestone")

    # 'localbuild' is just a placeholder choice for now.
    return milestone + "-localbuild"

def _extract_arch(ctx, cpu, style):
    """Extract the arch part from a os/arch pair."""

    # The current linux toolchain that we use does not return the target CPU.
    # It returns "local", probably because we are building without --platforms.
    target_cpu = ctx.var.get("TARGET_CPU")

    if cpu == "local" and target_cpu:
        cpu = target_cpu

    # darwin has the strange habit of calling a cpu "darwin_arm64".
    arch = cpu
    if "_" in cpu:
        arch = cpu.split("_")[-1]
    arch_name = _arch_names[style].get(arch)
    if arch_name:
        return arch_name

    # Sometimes the arch from the toolchain is just strange. Bazel's fallback
    # of target CPU is often better.
    return _arch_names[style].get(target_cpu) or target_cpu

def _inject_flavor(name, flavor):
    """Forms a canonical name from the base product name and the flavor"""
    if not flavor or flavor == "base":
        return name
    if not "-" in name:
        return "%s-%s" % (name, flavor)
    words = name.split("-")
    return "%s-%s-%s" % (words[0], flavor, "-".join(words[1:]))

def _package_name_variables_impl(ctx):
    values = {}

    flavor = ctx.attr._flavor[BuildSettingInfo].value
    values["product_name"] = _inject_flavor(ctx.attr.product_name, flavor)
    values["version"] = _make_version()
    values["base_branch"] = release_json.get("base_branch")
    values["milestone"] = release_json.get("current_milestone")

    # Package names often like to know what they are compiled for
    cc_toolchain = find_cc_toolchain(ctx)
    values["arch_deb"] = _extract_arch(ctx, cc_toolchain.cpu, "deb")
    values["arch_rpm"] = _extract_arch(ctx, cc_toolchain.cpu, "rpm")
    values["cpu"] = cc_toolchain.cpu
    values["compiler"] = cc_toolchain.compiler
    values["libc"] = cc_toolchain.libc
    values["compilation_mode"] = ctx.var.get("COMPILATION_MODE")

    # For initial testing: buildifier: disable=print
    # print(json.encode_indent(values))
    return PackageVariablesInfo(values = values)

package_name_variables = rule(
    implementation = _package_name_variables_impl,
    doc = _DOCSTRING,
    # Note that the default value comes from the rule instantiation.
    attrs = {
        "product_name": attr.string(
            doc = "Placeholder for our final product name.",
            default = "datadog-agent",
        ),
        "_flavor": attr.label(default = "//packages/agent:flavor"),
    },
    toolchains = use_cc_toolchain(),
)

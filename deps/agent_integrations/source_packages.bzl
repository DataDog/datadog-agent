"""Repository rule to retrieve source packages from integrations-core."""

load("@toml.bzl", "toml")
load("//bazel/repo:release_json.bzl", "read_effective_release_json")

ARM_EXCLUSIONS = ["ibm_ace", "ibm_mq"]
EXCLUSIONS = [
    "tokumx",  # py2-only, unsupported by current Agent
]

def _integration_source_packages_impl(rctx):
    release_info = read_effective_release_json(rctx, rctx.attr._release_info)
    commit = release_info["dependencies"]["INTEGRATIONS_CORE_VERSION"]

    # https://docs.github.com/en/repositories/working-with-files/using-files/downloading-source-code-archives#source-code-archive-urls
    # Disallowing `/` prevents retrieval of mutable references via the archive URLs
    if "/" in commit:
        fail("INTEGRATIONS_CORE_VERSION must be a commit hash and not a mutable reference (got {}, which includes a `/`)".format(commit))

    # Note: this requires INTEGRATIONS_CORE_VERSION to be a full commit hash.
    # This relies on omnibus-wrapping code for resolution in cases where the original INTEGRATIONS_CORE_VERSION
    # is set to a mutable reference.
    rctx.download_and_extract(
        url = "{base_url}/archive/{commit}.tar.gz".format(
            base_url = rctx.attr.base_url,
            commit = commit,
        ),
        stripPrefix = "integrations-core-{}".format(commit),
    )

    base_packages = ["datadog_checks_base", "datadog_checks_downloader"]
    integrations_by_platform = classify_integrations(rctx, arm_incompatible_integrations = ARM_EXCLUSIONS)
    integration_packages = set()
    integration_packages.update(*integrations_by_platform.values())

    packages = base_packages + list(integration_packages)

    # Individual packages that need to be built get their own BUILD file for building them as wheels
    for pkg in packages:
        rctx.file(
            "{}/BUILD.bazel".format(pkg),
            """
load("@@//deps/agent_integrations:defs.bzl", "pyproject_wheel")

package(default_visibility = ["//visibility:public"])

pyproject_wheel(
    name = "wheel",
    srcs = glob(["**"]),
    pyproject = "pyproject.toml",
)
""",
        )

    # Top-level BUILD file contains references to groups of wheels that are meaningful as a unit
    base_wheel_srcs = ", ".join(['"//{}:wheel"'.format(pkg) for pkg in base_packages])

    # Integrations are put under a filegroup with the srcs gated by a `select`,
    # such that the right subset of integrations are installed based on the platform
    integrations_select = """select({{
    {}
}})""".format(
        "\n".join([
            '"{}": [{}],'.format(platform, ", ".join(['"//{}:wheel"'.format(pkg) for pkg in sorted(integrations)]))
            for platform, integrations in integrations_by_platform.items()
        ]),
    )
    rctx.file(
        "BUILD.bazel",
        """
load("@package_metadata//rules:package_metadata.bzl", "package_metadata")
load("@rules_license//rules:license.bzl", "license")

package(default_visibility = ["//visibility:public"])

package_metadata(
    name = "package_metadata",
    attributes = [":license"],
    purl = "pkg:github/DataDog/integrations-core@{commit}",
)

license(
    name = "license",
    license_kinds = ["@rules_license//licenses/spdx:BSD-3-Clause"],
    license_text = "LICENSE",
)

exports_files(["requirements-agent-release.txt"])

filegroup(
    name = "all_wheels",
    srcs = ["base_wheels", "integrations_wheels"],
    package_metadata = [":package_metadata"],
)

filegroup(
    name = "base_wheels",
    srcs = [{base_wheel_srcs}],
)

filegroup(
    name = "integrations_wheels",
    srcs = {integrations_select},
)
""".format(
            base_wheel_srcs = base_wheel_srcs,
            integrations_select = integrations_select,
            commit = commit,
        ),
    )

    return rctx.repo_metadata(reproducible = True)

def classify_integrations(rctx, *, arm_incompatible_integrations = []):
    """Classifies toplevel integration packages by Bazel target platform.

    Args:
      rctx: The repository_ctx to use.
      arm_incompatible_integrations: Integrations that can't be shipped for ARM systems.

    Returns:
      A dictionary mapping Bazel platform labels to sets of integration names.
    """
    integrations_by_platform = {
        "@@//:linux_x86_64": set(),
        "@@//:linux_arm64": set(),
        "@@//:macos_x86_64": set(),
        "@@//:macos_arm64": set(),
        "@@//:windows_x86_64": set(),
    }
    manifest_platform_overrides = _load_manifest_platform_overrides(rctx)

    for entry in rctx.path(".").readdir():
        integration_name = entry.basename
        if not entry.is_dir or integration_name in EXCLUSIONS:
            continue

        # Skip folders that are not Python packages
        if not entry.get_child("pyproject.toml").exists:
            continue

        supported_platforms = _supported_platforms(rctx, entry, manifest_platform_overrides)

        if "linux" in supported_platforms:
            integrations_by_platform["@@//:linux_x86_64"].add(integration_name)
            if integration_name not in arm_incompatible_integrations:
                integrations_by_platform["@@//:linux_arm64"].add(integration_name)
        if "mac_os" in supported_platforms:
            integrations_by_platform["@@//:macos_x86_64"].add(integration_name)
            if integration_name not in arm_incompatible_integrations:
                integrations_by_platform["@@//:macos_arm64"].add(integration_name)
        if "windows" in supported_platforms:
            integrations_by_platform["@@//:windows_x86_64"].add(integration_name)

    return integrations_by_platform

def _load_manifest_platform_overrides(rctx):
    """Reads .ddev/config.toml manifest platform overrides for integrations without manifest.json."""
    config_path = rctx.path(".ddev/config.toml")
    if not config_path.exists:
        return {}

    config = toml.decode(rctx.read(config_path))
    if config == None:
        fail("Failed to parse {} as TOML".format(config_path))

    return config.get("overrides", {}).get("manifest", {}).get("platforms", {})

def _supported_platforms(rctx, entry, manifest_platform_overrides):
    """Returns the supported platforms for a package, using manifest.json or .ddev/config.toml overrides."""
    integration_name = entry.basename
    manifest_path = entry.get_child("manifest.json")
    if not manifest_path.exists:
        return manifest_platform_overrides.get(integration_name, [])

    manifest = json.decode(rctx.read(manifest_path))
    classifier_tags = manifest["tile"]["classifier_tags"]

    supported_platforms = []
    if "Supported OS::Linux" in classifier_tags:
        supported_platforms.append("linux")
    if "Supported OS::macOS" in classifier_tags:
        supported_platforms.append("mac_os")
    if "Supported OS::Windows" in classifier_tags:
        supported_platforms.append("windows")

    return supported_platforms

integration_source_packages = repository_rule(
    implementation = _integration_source_packages_impl,
    attrs = {
        "base_url": attr.string(
            default = "https://github.com/DataDog/integrations-core",
            doc = "Base URL of the repository",
        ),
        "_release_info": attr.label(default = "//:release.json", allow_single_file = True),
    },
    doc = "Retrieves integrations-core source packages used to build Agent integration wheels.",
)
